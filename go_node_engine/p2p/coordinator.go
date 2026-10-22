package p2p

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go_node_engine/config"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/p2p/scheduling"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"time"
)

// ErrUnschedulable is returned when no capable node (including self) exists right now.
// The caller parks the job as PENDING instead of dropping it.
var ErrUnschedulable = errors.New("UNSCHEDULABLE: no capable node in the network")

// unschedulable wraps ErrUnschedulable with why each node could not take the job.
func unschedulable(details []string) error {
	if len(details) == 0 {
		return ErrUnschedulable
	}
	return fmt.Errorf("%w: %s", ErrUnschedulable, strings.Join(details, " | "))
}

// Placement wire types. A probe sends a BidRequest to each sampled member's control API
// and gets a BidResponse; the chosen host then receives an AssignRequest. All of it is
// direct member-to-member HTTPS on the mTLS control plane.
type BidRequest struct {
	ReqID   string `json:"req_id"`
	JobName string `json:"job_name"`
	Runtime string `json:"runtime"`
	Memory  int    `json:"memory"`
	Vcpus   int    `json:"vcpus"`
	Vgpus   int    `json:"vgpus,omitempty"`
	// CSIDrivers lists the CSI driver of every volume the job mounts.
	CSIDrivers []string `json:"csi_drivers,omitempty"`
}

type BidResponse struct {
	ReqID    string  `json:"req_id"`
	NodeUUID string  `json:"node_uuid"`
	Ok       bool    `json:"ok"`
	FreeMem  float64 `json:"free_mem_mb"`
	Vcpus    float64 `json:"vcpus"`
	CpuPct   float64 `json:"cpu_pct"`
	Vgpus    int     `json:"vgpus"`
	// CSIDrivers lists the CSI drivers this node has probed successfully.
	CSIDrivers []string `json:"csi_drivers,omitempty"`
	// Reason explains a negative bid; empty when Ok.
	Reason string `json:"reason,omitempty"`
}

type AssignRequest struct {
	Job        DeployableService `json:"job"`
	Generation int               `json:"generation"`
}

type AssignResponse struct {
	Ok     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

// newBidRequest builds the bid request describing job's placement requirements.
func newBidRequest(job DeployableService) BidRequest {
	req := BidRequest{
		JobName: job.JobName,
		Runtime: job.Service.Runtime,
		Memory:  job.Service.Memory,
		Vcpus:   job.Service.Vcpus,
		Vgpus:   job.Service.Vgpus,
	}
	for _, vol := range job.Service.Volumes {
		if vol.CSIDriver != "" {
			req.CSIDrivers = append(req.CSIDrivers, vol.CSIDriver)
		}
	}
	return req
}

// evaluateBid answers "can this node host the job right now?" — capability match
// (runtime and CSI drivers) plus currently-free resources.
func (c *Controller) evaluateBid(req BidRequest) BidResponse {
	node := model.GetNodeInfo()
	freeMem := float64(node.MemoryMB) * (1 - node.MemoryUsed/100)

	// Accept SLA virtualization names ("container") as well as runtime ids ("docker").
	req.Runtime = runtimeFor(req.Runtime)
	var reasons []string
	if !slices.Contains(node.Technology, config.RuntimeType(req.Runtime)) {
		reasons = append(reasons, fmt.Sprintf("runtime %q not supported (have %v)", req.Runtime, node.Technology))
	}
	for _, driver := range req.CSIDrivers {
		if !node.HasCSIDriver(driver) {
			reasons = append(reasons, fmt.Sprintf("missing CSI driver %q", driver))
		}
	}
	if freeMem < float64(req.Memory) {
		reasons = append(reasons, fmt.Sprintf("needs %d MB memory, %.0f MB free", req.Memory, freeMem))
	}
	if node.CpuCores < req.Vcpus {
		reasons = append(reasons, fmt.Sprintf("needs %d vcpus, node has %d", req.Vcpus, node.CpuCores))
	}
	if node.GpuCores < req.Vgpus {
		reasons = append(reasons, fmt.Sprintf("needs %d vgpus, node has %d", req.Vgpus, node.GpuCores))
	}

	csiDrivers := make([]string, 0, len(node.CSIDrivers))
	for _, d := range node.GetCSIDrivers() {
		csiDrivers = append(csiDrivers, d.Name)
	}
	return BidResponse{
		ReqID:      req.ReqID,
		NodeUUID:   c.NodeUUID,
		Ok:         len(reasons) == 0,
		FreeMem:    freeMem,
		Vcpus:      float64(node.CpuCores),
		CpuPct:     node.CpuUsage,
		Vgpus:      node.GpuCores,
		CSIDrivers: csiDrivers,
		Reason:     strings.Join(reasons, "; "),
	}
}

// Placement defaults, used when the config leaves them unset.
const (
	defaultSchedSample       = 3
	defaultSchedProbeTimeout = 5 * time.Second
)

// schedSample is how many random shortlisted members the first probe asks.
func (c *Controller) schedSample() int {
	if c.Conf.P2P.SchedSample > 0 {
		return c.Conf.P2P.SchedSample
	}
	return defaultSchedSample
}

// schedProbeTimeout bounds how long a probe waits for its members' answers.
func (c *Controller) schedProbeTimeout() time.Duration {
	if c.Conf.P2P.SchedProbeTimeoutSec > 0 {
		return time.Duration(c.Conf.P2P.SchedProbeTimeoutSec) * time.Second
	}
	return defaultSchedProbeTimeout
}

// ScheduleJob places one service and assigns it to the chosen host.
//
// Placement samples instead of broadcasting, like picking the shortest of a few
// checkout lines: shortlist the members whose gossiped capabilities match (local, no
// traffic), ask a few random ones how much room they have right now, and pick among
// those with bestRandomFit. Only if none of the sample has room are the remaining
// shortlisted members asked, so a job is never parked while some node could take it.
//
// Returns the winning host UUID, or an error wrapping ErrUnschedulable that lists
// why each node declined.
func (c *Controller) ScheduleJob(job DeployableService, localOnly bool, generation int) (string, error) {
	members, err := c.nm.Members()
	if err != nil || len(members) == 0 {
		// NetManager not in p2p yet or single node without membership: local fast path.
		members = []MemberInfo{{UUID: c.NodeUUID}}
	}
	bidReq := newBidRequest(job)
	bidReq.ReqID = fmt.Sprintf("%s-g%d-%d", job.JobName, generation, time.Now().UnixNano())

	// Standalone fast path: a single node or --local.
	if localOnly || len(members) <= 1 {
		self := c.evaluateBid(bidReq)
		if self.Ok {
			return c.NodeUUID, c.assignTo(MemberInfo{UUID: c.NodeUUID}, job, generation)
		}
		logger.InfoLogger().Printf("p2p sched: %s declined %s: %s", self.NodeUUID, job.JobName, self.Reason)
		return "", unschedulable([]string{fmt.Sprintf("%s: %s", self.NodeUUID, self.Reason)})
	}

	candidates, declined := c.shortlist(bidReq, members)
	if len(candidates) == 0 {
		return "", unschedulable(declined)
	}
	rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	sample := min(c.schedSample(), len(candidates))

	req := scheduling.Requirements{Runtime: job.Service.Runtime, Memory: float64(job.Service.Memory), Vcpus: float64(job.Service.Vcpus),
		Vgpus: job.Service.Vgpus, CSIDrivers: bidReq.CSIDrivers}
	for i, batch := range [][]MemberInfo{candidates[:sample], candidates[sample:]} {
		if len(batch) == 0 {
			continue
		}
		if i == 1 {
			logger.InfoLogger().Printf("p2p sched: no room for %s among %d sampled member(s), asking the other %d", job.JobName, sample, len(batch))
		}
		positives, reasons := c.probe(batch, bidReq)
		declined = append(declined, reasons...)
		host, reasons := c.placeAmong(req, positives, batch, job, generation)
		declined = append(declined, reasons...)
		if host != "" {
			return host, nil
		}
	}
	return "", unschedulable(declined)
}

// probe asks each member for a live bid in parallel (this node answers in-process) and
// waits for every answer, bounded by the probe timeout. Waiting for all of them matters:
// stopping at the first positive would always pick the coordinator, whose in-process
// answer arrives before any peer's.
func (c *Controller) probe(batch []MemberInfo, req BidRequest) (positives []scheduling.Bid, declined []string) {
	responses := make(chan BidResponse, len(batch))
	for _, m := range batch {
		go func(m MemberInfo) {
			var resp BidResponse
			if m.UUID == c.NodeUUID {
				resp = c.evaluateBid(req)
			} else if r, err := c.requestBid(m, req); err != nil {
				resp = BidResponse{Reason: fmt.Sprintf("no bid: %v", err)}
			} else {
				resp = r
			}
			// Attribute the answer to the member we asked, not to what it claims.
			resp.NodeUUID = m.UUID
			responses <- resp
		}(m)
	}

	deadline := time.After(c.schedProbeTimeout())
	for replied := 0; replied < len(batch); replied++ {
		select {
		case resp := <-responses:
			if !resp.Ok {
				logger.InfoLogger().Printf("p2p sched: %s declined %s: %s", resp.NodeUUID, req.JobName, resp.Reason)
				declined = append(declined, fmt.Sprintf("%s: %s", resp.NodeUUID, resp.Reason))
				continue
			}
			positives = append(positives, scheduling.Bid{
				NodeUUID: resp.NodeUUID, Mem: resp.FreeMem, Vcpus: resp.Vcpus, CpuPct: resp.CpuPct, Vgpus: resp.Vgpus,
				CSIDrivers: resp.CSIDrivers,
			})
		case <-deadline:
			declined = append(declined, fmt.Sprintf("probe timeout after %s: %d of %d member(s) replied", c.schedProbeTimeout(), replied, len(batch)))
			return positives, declined
		}
	}
	return positives, declined
}

// placeAmong picks a host among positive bids with bestRandomFit and assigns the job.
// If the winner refuses (its capacity went between bid and assign) or is unreachable,
// it is dropped and the next pick is tried. Returns "" when no positive accepted.
func (c *Controller) placeAmong(req scheduling.Requirements, positives []scheduling.Bid, batch []MemberInfo, job DeployableService, generation int) (string, []string) {
	var declined []string
	memberByUUID := make(map[string]MemberInfo, len(batch))
	for _, m := range batch {
		memberByUUID[m.UUID] = m
	}
	for len(positives) > 0 {
		winnerUUID, ok := scheduling.PickHost(req, positives, 0)
		if !ok {
			declined = append(declined, fmt.Sprintf("%d positive bid(s) rejected by the placement filter", len(positives)))
			break
		}
		err := c.assignTo(memberByUUID[winnerUUID], job, generation)
		if err == nil {
			return winnerUUID, declined
		}
		logger.InfoLogger().Printf("p2p sched: %s refused assign for %s, trying next candidate", winnerUUID, job.JobName)
		declined = append(declined, fmt.Sprintf("%s: assign failed: %v", winnerUUID, err))
		positives = slices.DeleteFunc(positives, func(b scheduling.Bid) bool { return b.NodeUUID == winnerUUID })
	}
	return "", declined
}

// requestBid asks one peer for a bid over its control API.
func (c *Controller) requestBid(m MemberInfo, req BidRequest) (BidResponse, error) {
	var resp BidResponse
	err := c.postPeer(m, "/ctrl/bid", req, &resp)
	return resp, err
}

// assignTo hands the job to the winner. The winner re-checks resources (TOCTOU),
// deploys, persists, advertises, and replicates custody.
func (c *Controller) assignTo(m MemberInfo, job DeployableService, generation int) error {
	assign := AssignRequest{Job: job, Generation: generation}
	if m.UUID == c.NodeUUID {
		resp := c.handleAssign(assign)
		if !resp.Ok {
			return fmt.Errorf("local assign refused: %s", resp.Reason)
		}
		return nil
	}
	var resp AssignResponse
	if err := c.postPeer(m, "/ctrl/assign", assign, &resp); err != nil {
		return err
	}
	if !resp.Ok {
		return fmt.Errorf("assign refused by %s: %s", m.UUID, resp.Reason)
	}
	return nil
}

// postPeer POSTs a JSON payload to another member's control API and decodes the reply.
func (c *Controller) postPeer(m MemberInfo, path string, payload, out interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s://%s:%d%s", c.scheme, m.IP(), c.ControlPort, path)
	resp, err := c.peers.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %s: status %d", m.UUID, path, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
