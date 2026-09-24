// Package scheduling adapts the shared scheduler's bestrandomfit algorithm for use by
// the node engine's p2p bid round (plan §1.5 step 6, §1.6). The node engine collects
// positive bids from peers and calls PickHost to select one at random among the best.
package scheduling

import (
	"scheduler/calculate/schedulers/bestrandomfit"
	"scheduler/calculate/schedulers/placement"
)

// Requirements are the placement constraints distilled from an SLA service.
type Requirements struct {
	Runtime string
	Memory  float64 // MB required
	Vcpus   float64
}

// Bid is a peer's self-reported capacity, collected during the bid round.
type Bid struct {
	NodeUUID string
	Mem      float64 // available memory
	Vcpus    float64 // available vcpus
	CpuPct   float64 // current cpu utilisation %
}

// PickHost selects a host among positive bids using bestrandomfit — random among the
// best-scoring band. Returns the chosen node UUID; ok=false if none qualify.
func PickHost(req Requirements, bids []Bid, tolerance float64) (nodeUUID string, ok bool) {
	if len(bids) == 0 {
		return "", false
	}
	job := bestrandomfit.Resources{BaseResources: placement.BaseResources{
		Virtualization: []string{req.Runtime},
		AvailableMem:   req.Memory,
		AvailableCPU:   req.Vcpus,
	}}
	candidates := make([]bestrandomfit.Resources, len(bids))
	for i, b := range bids {
		candidates[i] = bestrandomfit.Resources{BaseResources: placement.BaseResources{
			ID:             b.NodeUUID,
			Virtualization: []string{req.Runtime},
			AvailableMem:   b.Mem,
			AvailableCPU:   b.Vcpus,
			CPUPercent:     b.CpuPct,
		}}
	}
	if tolerance <= 0 {
		tolerance = bestrandomfit.DefaultTolerance
	}
	chosen, err := bestrandomfit.Scheduler{Tolerance: tolerance}.Calculate(job, candidates)
	if err != nil {
		return "", false
	}
	return chosen.ID(), true
}
