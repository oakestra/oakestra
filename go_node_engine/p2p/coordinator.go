package p2p

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/p2p/scheduling"
	"math"
	"net/http"
	"sync"
	"time"
)

// ErrUnschedulable is returned when no capable node (including self) exists right now.
// The caller parks the job as PENDING instead of dropping it (plan §7.6).
var ErrUnschedulable = errors.New("UNSCHEDULABLE: no capable node in the network")

// Bid round wire types (plan §1.5). BID_REQUEST fans out to every member's control API;
// BID/ASSIGN/ASSIGN_ACK are the responses — direct member-to-member HTTP on the
// members-only control plane (the plan allows unicast in place of gossip broadcast).
type BidRequest struct {
	ReqID   string `json:"req_id"`
	JobName string `json:"job_name"`
	Runtime string `json:"runtime"`
	Memory  int    `json:"memory"`
	Vcpus   int    `json:"vcpus"`
}

type BidResponse struct {
	ReqID    string  `json:"req_id"`
	NodeUUID string  `json:"node_uuid"`
	Ok       bool    `json:"ok"`
	FreeMem  float64 `json:"free_mem_mb"`
	Vcpus    float64 `json:"vcpus"`
	CpuPct   float64 `json:"cpu_pct"`
}

type AssignRequest struct {
	Job        DeployableService `json:"job"`
	Generation int               `json:"generation"`
}

type AssignResponse struct {
	Ok     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

// evaluateBid answers "can this node host the job right now?" — capability match plus
// currently-free resources (plan §1.5 step 4).
func (c *Controller) evaluateBid(req BidRequest) BidResponse {
	node := model.GetNodeInfo()
	freeMem := float64(node.MemoryMB) * (1 - node.MemoryUsed/100)

	supported := false
	for _, t := range node.Technology {
		if string(t) == req.Runtime {
			supported = true
			break
		}
	}
	ok := supported &&
		freeMem >= float64(req.Memory) &&
		float64(node.CpuCores) >= float64(req.Vcpus)

	return BidResponse{
		ReqID:    req.ReqID,
		NodeUUID: c.NodeUUID,
		Ok:       ok,
		FreeMem:  freeMem,
		Vcpus:    float64(node.CpuCores),
		CpuPct:   node.CpuUsage,
	}
}

// ScheduleJob runs the bid round for one service and assigns it to the chosen host
// (plan §1.5). Returns the winning host UUID, or ErrUnschedulable.
func (c *Controller) ScheduleJob(job DeployableService, localOnly bool, generation int) (string, error) {
	members, err := c.nm.Members()
	if err != nil || len(members) == 0 {
		// NetManager not in p2p yet or single node without membership: local fast path.
		members = []MemberInfo{{UUID: c.NodeUUID}}
	}
	n := len(members)

	bidReq := BidRequest{
		ReqID:   fmt.Sprintf("%s-g%d-%d", job.JobName, generation, time.Now().UnixNano()),
		JobName: job.JobName,
		Runtime: job.Service.Runtime,
		Memory:  job.Service.Memory,
		Vcpus:   job.Service.Vcpus,
	}

	// Standalone fast path (plan §1.5 step 2): N==1 or --local.
	if localOnly || n <= 1 {
		if self := c.evaluateBid(bidReq); self.Ok {
			return c.NodeUUID, c.assignTo(MemberInfo{UUID: c.NodeUUID}, job, generation)
		}
		return "", ErrUnschedulable
	}

	// Quorum = ceil(N / SIGMA), default SIGMA=4 (plan §1.5 step 1).
	sigma := c.Conf.P2P.SchedSigma
	if sigma <= 0 {
		sigma = 4
	}
	quorum := int(math.Ceil(float64(n) / float64(sigma)))
	if quorum < 1 {
		quorum = 1
	}
	timeout := time.Duration(c.Conf.P2P.SchedTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	// Fan out BID_REQUEST to every member (self evaluated locally, plan §1.5 step 3-4).
	responses := make(chan BidResponse, n)
	var wg sync.WaitGroup
	for _, m := range members {
		wg.Add(1)
		go func(m MemberInfo) {
			defer wg.Done()
			if m.UUID == c.NodeUUID {
				responses <- c.evaluateBid(bidReq)
				return
			}
			if resp, err := c.requestBid(m, bidReq); err == nil {
				responses <- resp
			}
		}(m)
	}
	go func() { wg.Wait(); close(responses) }()

	// Collect positives until quorum, timeout, or all replied (plan §1.5 step 5).
	positives := make([]scheduling.Bid, 0, n)
	memberByUUID := map[string]MemberInfo{}
	for _, m := range members {
		memberByUUID[m.UUID] = m
	}
	deadline := time.After(timeout)
	replied := 0
collect:
	for replied < n {
		select {
		case resp, open := <-responses:
			if !open {
				break collect
			}
			replied++
			if resp.Ok {
				positives = append(positives, scheduling.Bid{
					NodeUUID: resp.NodeUUID, Mem: resp.FreeMem, Vcpus: resp.Vcpus, CpuPct: resp.CpuPct,
				})
				if len(positives) >= quorum {
					break collect
				}
			}
		case <-deadline:
			logger.InfoLogger().Printf("p2p sched: bid round for %s hit the %s timeout with %d/%d positives — network may be approaching capacity", job.JobName, timeout, len(positives), quorum)
			break collect
		}
	}

	// Decide via bestRandomFit — random among the best band (plan §1.5 step 6, §1.6).
	req := scheduling.Requirements{Runtime: job.Service.Runtime, Memory: float64(job.Service.Memory), Vcpus: float64(job.Service.Vcpus)}
	for len(positives) > 0 {
		winnerUUID, ok := scheduling.PickHost(req, positives, 0)
		if !ok {
			break
		}
		winner := memberByUUID[winnerUUID]
		if err := c.assignTo(winner, job, generation); err == nil {
			return winnerUUID, nil
		}
		// TOCTOU: winner refused or died between bid and assign — drop it, pick the next
		// random positive (plan §1.5 "Assign + TOCTOU").
		logger.InfoLogger().Printf("p2p sched: %s refused assign for %s, trying next candidate", winnerUUID, job.JobName)
		kept := positives[:0]
		for _, b := range positives {
			if b.NodeUUID != winnerUUID {
				kept = append(kept, b)
			}
		}
		positives = kept
	}
	return "", ErrUnschedulable
}

// requestBid asks one peer for a bid over its control API.
func (c *Controller) requestBid(m MemberInfo, req BidRequest) (BidResponse, error) {
	var resp BidResponse
	err := c.postPeer(m, "/ctrl/bid", req, &resp)
	return resp, err
}

// assignTo hands the job to the winner. The winner re-checks resources (TOCTOU),
// deploys, persists, advertises, and replicates custody (plan §1.5, §7.1).
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
