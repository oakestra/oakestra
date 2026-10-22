package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"go_node_engine/internal/store"
	"go_node_engine/model"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// capacityPeerTimeout bounds each member's reply during a network capacity sweep.
const capacityPeerTimeout = 5 * time.Second

// Capacity is one node's view of its own schedulable resources: the inputs that
// evaluateBid decides on, plus how many services it already hosts.
type Capacity struct {
	NodeUUID      string   `json:"node_uuid"`
	Runtimes      []string `json:"runtimes"`
	Vcpus         int      `json:"vcpus"`
	CpuPct        float64  `json:"cpu_pct"`
	MemoryMB      int      `json:"memory_mb"`
	MemoryUsedPct float64  `json:"memory_used_pct"`
	FreeMemMB     float64  `json:"free_mem_mb"`
	Vgpus         int      `json:"vgpus"`
	GpuPct        float64  `json:"gpu_pct"`
	CSIDrivers    []string `json:"csi_drivers"`
	Services      int      `json:"services"`
	// Bid is this node's answer to the hypothetical job in the query, if one was given.
	Bid *BidResponse `json:"bid,omitempty"`
}

// MemberCapacity is one row of a network capacity sweep. Error is set when the
// member could not be queried; such a member also cannot answer a placement probe.
type MemberCapacity struct {
	NodeUUID string    `json:"node_uuid"`
	Addr     string    `json:"addr"`
	Capacity *Capacity `json:"capacity,omitempty"`
	Error    string    `json:"error,omitempty"`
}

// NetworkCapacity is the capacity of every member together with the placement
// parameters this node schedules with.
type NetworkCapacity struct {
	Members         []MemberCapacity `json:"members"`
	Sample          int              `json:"sample"`
	ProbeTimeoutSec int              `json:"probe_timeout_sec"`
}

// localCapacity reports this node's capacity; with a non-nil req it also evaluates
// the hypothetical job exactly as a real placement probe would.
func (c *Controller) localCapacity(req *BidRequest) Capacity {
	node := model.GetNodeInfo()
	capacity := Capacity{
		NodeUUID:      c.NodeUUID,
		Runtimes:      make([]string, 0, len(node.Technology)),
		Vcpus:         node.CpuCores,
		CpuPct:        node.CpuUsage,
		MemoryMB:      node.MemoryMB,
		MemoryUsedPct: node.MemoryUsed,
		FreeMemMB:     float64(node.MemoryMB) * (1 - node.MemoryUsed/100),
		Vgpus:         node.GpuCores,
		GpuPct:        node.GpuUsage,
		CSIDrivers:    make([]string, 0, len(node.CSIDrivers)),
	}
	for _, t := range node.Technology {
		capacity.Runtimes = append(capacity.Runtimes, string(t))
	}
	for _, d := range node.GetCSIDrivers() {
		capacity.CSIDrivers = append(capacity.CSIDrivers, d.Name)
	}
	if records, err := store.Load(); err == nil {
		capacity.Services = len(records)
	}
	if req != nil {
		bid := c.evaluateBid(*req)
		capacity.Bid = &bid
	}
	return capacity
}

// capacityHandler serves GET /ctrl/capacity. By default it reports this node only;
// with network=1 it queries every member concurrently. The optional runtime, memory,
// vcpus, vgpus and csi (repeatable) parameters describe a hypothetical job that each node
// evaluates with its real bid logic, reporting why it would refuse.
func (c *Controller) capacityHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	req, err := bidRequestFromQuery(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if query.Get("network") != "1" {
		writeJSON(w, c.localCapacity(req))
		return
	}

	members, err := c.nm.Members()
	if err != nil || len(members) == 0 {
		// Same fallback as ScheduleJob: without membership, placement is local only.
		members = []MemberInfo{{UUID: c.NodeUUID}}
	}
	query.Del("network")
	rows := make([]MemberCapacity, len(members))
	var wg sync.WaitGroup
	for i, m := range members {
		rows[i] = MemberCapacity{NodeUUID: m.UUID, Addr: m.Addr}
		if m.UUID == c.NodeUUID {
			local := c.localCapacity(req)
			rows[i].Capacity = &local
			continue
		}
		wg.Add(1)
		go func(row *MemberCapacity, m MemberInfo) {
			defer wg.Done()
			capacity, err := c.peerCapacity(r.Context(), m, query)
			if err != nil {
				row.Error = err.Error()
				return
			}
			row.Capacity = &capacity
		}(&rows[i], m)
	}
	wg.Wait()

	writeJSON(w, NetworkCapacity{Members: rows, Sample: c.schedSample(), ProbeTimeoutSec: int(c.schedProbeTimeout().Seconds())})
}

// peerCapacity fetches one member's local capacity over its control API.
func (c *Controller) peerCapacity(ctx context.Context, m MemberInfo, query url.Values) (Capacity, error) {
	ctx, cancel := context.WithTimeout(ctx, capacityPeerTimeout)
	defer cancel()
	target := fmt.Sprintf("%s://%s:%d/ctrl/capacity?%s", c.scheme, m.IP(), c.ControlPort, query.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Capacity{}, err
	}
	resp, err := c.peers.Do(req)
	if err != nil {
		return Capacity{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Capacity{}, fmt.Errorf("status %d", resp.StatusCode)
	}
	var capacity Capacity
	err = json.NewDecoder(resp.Body).Decode(&capacity)
	return capacity, err
}

// bidRequestFromQuery builds the hypothetical job from the query, or returns nil
// when the query describes none.
func bidRequestFromQuery(query url.Values) (*BidRequest, error) {
	if !query.Has("runtime") && !query.Has("memory") && !query.Has("vcpus") && !query.Has("vgpus") && !query.Has("csi") {
		return nil, nil
	}
	req := &BidRequest{Runtime: runtimeFor(query.Get("runtime")), CSIDrivers: query["csi"]}
	for name, field := range map[string]*int{"memory": &req.Memory, "vcpus": &req.Vcpus, "vgpus": &req.Vgpus} {
		v := query.Get(name)
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid %s %q: %w", name, v, err)
		}
		*field = n
	}
	return req, nil
}
