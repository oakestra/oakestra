// Package scheduling adapts the shared scheduler's bestrandomfit algorithm for use by
// the node engine's p2p placement. The node engine collects positive bids from the
// members it sampled and calls PickHost to select one at random among the best.
package scheduling

import (
	"scheduler/calculate/schedulers/bestrandomfit"
	"scheduler/calculate/schedulers/cpumemfit"
	"scheduler/calculate/schedulers/placement"
	"slices"
)

// Requirements are the placement constraints distilled from an SLA service.
type Requirements struct {
	Runtime string
	Memory  float64 // MB required
	Vcpus   float64
	Vgpus   int
	// CSIDrivers lists the CSI driver of every volume the service mounts.
	CSIDrivers []string
}

// Bid is a peer's self-reported free capacity, collected by a placement probe.
type Bid struct {
	NodeUUID string
	Mem      float64 // available memory
	Vcpus    float64 // available vcpus
	CpuPct   float64 // current cpu utilisation %
	Vgpus    int     // GPUs on the peer
	// CSIDrivers lists the CSI drivers the peer has probed successfully.
	CSIDrivers []string
}

// PickHost selects a host among positive bids using bestrandomfit — random among the
// best-scoring band. Returns the chosen node UUID; ok=false if none qualify.
func PickHost(req Requirements, bids []Bid, tolerance float64) (nodeUUID string, ok bool) {
	// The shared scheduler has no GPU dimension, so GPU fit is filtered here first.
	bids = slices.DeleteFunc(slices.Clone(bids), func(b Bid) bool { return b.Vgpus < req.Vgpus })
	if len(bids) == 0 {
		return "", false
	}
	job := cpumemfit.Resources{BaseResources: placement.BaseResources{
		Virtualization: []string{req.Runtime},
		AvailableMem:   req.Memory,
		AvailableCPU:   req.Vcpus,
	}}
	for _, driver := range req.CSIDrivers {
		job.Volumes = append(job.Volumes, cpumemfit.VolumeSpec{CSIDriver: driver})
	}
	candidates := make([]cpumemfit.Resources, len(bids))
	for i, b := range bids {
		candidates[i] = cpumemfit.Resources{
			BaseResources: placement.BaseResources{
				ID:             b.NodeUUID,
				Virtualization: []string{req.Runtime},
				AvailableMem:   b.Mem,
				AvailableCPU:   b.Vcpus,
				CPUPercent:     b.CpuPct,
			},
			CSIDrivers: b.CSIDrivers,
		}
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
