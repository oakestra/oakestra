package p2p

import (
	"encoding/json"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"slices"
	"strings"
	"time"
)

// CapabilitiesJobName is the reserved registry job name carrying each member's static
// capabilities. Only what changes rarely is gossiped — never live load — so a
// coordinator can rule out nodes that could never host a job without asking them.
const CapabilitiesJobName = "__caps__"

// capabilitiesRefresh is how often a node checks whether its capabilities changed
// (e.g. a CSI driver came up) and re-advertises them if so.
const capabilitiesRefresh = time.Minute

// Capabilities are a node's static hosting properties: what it could run if empty.
type Capabilities struct {
	Arch       string   `json:"arch"`
	Runtimes   []string `json:"runtimes"`
	CSIDrivers []string `json:"csi_drivers"`
	Vcpus      int      `json:"vcpus"`
	MemoryMB   int      `json:"memory_mb"`
	Vgpus      int      `json:"vgpus"`
}

// localCapabilities reads this node's capabilities from the node-info singleton.
func localCapabilities() Capabilities {
	node := model.GetNodeInfo()
	caps := Capabilities{
		Arch:       node.CpuArch,
		Runtimes:   make([]string, 0, len(node.Technology)),
		CSIDrivers: make([]string, 0, len(node.CSIDrivers)),
		Vcpus:      node.CpuCores,
		MemoryMB:   node.MemoryMB,
		Vgpus:      node.GpuCores,
	}
	for _, t := range node.Technology {
		caps.Runtimes = append(caps.Runtimes, string(t))
	}
	for _, d := range node.GetCSIDrivers() {
		caps.CSIDrivers = append(caps.CSIDrivers, d.Name)
	}
	return caps
}

// Match reports whether a node with these capabilities could ever host the job, and
// why not. Free resources are deliberately not considered: that is the live bid's job.
func (caps Capabilities) Match(req BidRequest) (bool, string) {
	var reasons []string
	runtime := runtimeFor(req.Runtime)
	if !slices.Contains(caps.Runtimes, runtime) {
		reasons = append(reasons, fmt.Sprintf("runtime %q not supported (have %v)", runtime, caps.Runtimes))
	}
	for _, driver := range req.CSIDrivers {
		if !slices.Contains(caps.CSIDrivers, driver) {
			reasons = append(reasons, fmt.Sprintf("missing CSI driver %q", driver))
		}
	}
	if caps.MemoryMB < req.Memory {
		reasons = append(reasons, fmt.Sprintf("needs %d MB memory, node has %d MB in total", req.Memory, caps.MemoryMB))
	}
	if caps.Vcpus < req.Vcpus {
		reasons = append(reasons, fmt.Sprintf("needs %d vcpus, node has %d", req.Vcpus, caps.Vcpus))
	}
	if caps.Vgpus < req.Vgpus {
		reasons = append(reasons, fmt.Sprintf("needs %d vgpus, node has %d", req.Vgpus, caps.Vgpus))
	}
	return len(reasons) == 0, strings.Join(reasons, "; ")
}

// AnnounceCapabilities gossips this node's capabilities as a __caps__ registry entry,
// then re-advertises whenever they change. Every advertisement bumps the entry's
// version, so unchanged capabilities are not re-sent. Runs for the daemon's lifetime.
func (c *Controller) AnnounceCapabilities() {
	announced := ""
	for {
		data, err := json.Marshal(localCapabilities())
		if err == nil && string(data) != announced {
			entry := RegistryEntry{JobName: CapabilitiesJobName, NodeUUID: c.NodeUUID, Meta: string(data)}
			if err := c.nm.Advertise(entry); err == nil {
				announced = string(data)
				logger.InfoLogger().Printf("p2p: capabilities announced %s", announced)
			}
		}
		if announced == "" {
			// NetManager's p2p layer not up yet: retry soon.
			time.Sleep(2 * time.Second)
			continue
		}
		time.Sleep(capabilitiesRefresh)
	}
}

// shortlist returns the members whose capabilities match the job, without any network
// traffic beyond reading the local registry replica. A member whose __caps__ entry has
// not converged yet stays on the list — the live bid decides for it. ruledOut explains
// every member that was excluded.
func (c *Controller) shortlist(req BidRequest, members []MemberInfo) (candidates []MemberInfo, ruledOut []string) {
	known := map[string]Capabilities{c.NodeUUID: localCapabilities()}
	if entries, err := c.nm.Services(); err == nil {
		for _, e := range entries {
			if e.JobName != CapabilitiesJobName || e.NodeUUID == c.NodeUUID {
				continue
			}
			var caps Capabilities
			if json.Unmarshal([]byte(e.Meta), &caps) == nil {
				known[e.NodeUUID] = caps
			}
		}
	}
	for _, m := range members {
		caps, ok := known[m.UUID]
		if !ok {
			candidates = append(candidates, m)
			continue
		}
		if match, reason := caps.Match(req); match {
			candidates = append(candidates, m)
		} else {
			ruledOut = append(ruledOut, fmt.Sprintf("%s: %s", m.UUID, reason))
		}
	}
	return candidates, ruledOut
}
