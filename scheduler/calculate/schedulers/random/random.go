// Package random implements a scheduling algorithm that picks a qualifying
// placement candidate at random.
package random

import (
	"encoding/json"
	"math/rand/v2"
	"scheduler/calculate/schedulers/placement"
	"scheduler/logger"
)

// Constraints holds the placement directives embedded in a job descriptor.
type Constraints struct {
	placement.GenericConstraints
}

// Resources serves as both the job descriptor (J) and the placement candidate (C)
// in Algorithm[Resources, Resources]. The two roles share one type because the
// ResourceAbstractor returns candidates with the same JSON schema as the incoming
// job payload — they were a single type before the Algorithm interface was
// generalised to two type parameters.
type Resources struct {
	placement.BaseResources
	Constraints []Constraints `json:"constraints"`
}

// ID returns the candidate or job identifier.
func (r Resources) ID() string { return r.BaseResources.ID }

// ResourceConstraints returns the scheduling constraints used to query the
// resource abstractor.
func (r Resources) ResourceConstraints() map[string]string {
	constraints := make(map[string]string)
	for _, c := range r.Constraints {
		logger.DebugLogger().Printf("Constraint: %+v", c)
		if c.Type == "direct" {
			constraints["cluster_name"] = c.Cluster
			constraints["node_name"] = c.Node
		}
	}
	return constraints
}

func (r *Resources) UnmarshalJSON(data []byte) error {
	type Alias Resources
	aux := &struct {
		Virtualization any `json:"virtualization"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	virt, err := placement.NormalizeVirtualization(aux.Virtualization)
	if err != nil {
		return err
	}
	r.BaseResources.Virtualization = virt

	return nil
}

// Scheduler picks a qualifying placement candidate at random.
// It implements placement.Algorithm[Resources].
type Scheduler struct{}

func (Scheduler) JobData() Resources { return Resources{} }

func (Scheduler) Calculate(job Resources, candidates []Resources) (Resources, error) {
	if len(candidates) == 0 {
		return Resources{}, placement.SchedulingError{NegativeSchedulingStatus: placement.TargetClusterNotActive}
	}
	filtered := filterRequirements(job, candidates)
	if len(filtered) == 0 {
		status := placement.NoCapacityStatus(job.BaseResources, candidates, func(c Resources) bool {
			return meetsAllButGPU(job, c)
		})
		return Resources{}, placement.SchedulingError{NegativeSchedulingStatus: status}
	}
	return filtered[rand.IntN(len(filtered))], nil
}

func filterRequirements(job Resources, candidates []Resources) []Resources {
	return placement.Filter(candidates, func(c Resources) bool {
		logger.DebugLogger().Printf("Filtering candidate: %v", c)
		return meetsAllButGPU(job, c) && placement.MeetsGPURequirements(job.BaseResources, c.BaseResources)
	})
}

// meetsAllButGPU is every filter check except the GPU one. Calculate also uses
// it on its own to tell a GPU shortage apart from other misses, so new filter
// checks belong here rather than in filterRequirements.
func meetsAllButGPU(job, c Resources) bool {
	return placement.MeetsNonGPURequirements(job.BaseResources, c.BaseResources)
}
