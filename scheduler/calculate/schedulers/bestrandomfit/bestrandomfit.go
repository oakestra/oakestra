// Package bestrandomfit implements a scheduling algorithm that filters candidates
// by requirements, then picks uniformly at random among the best-scoring band.
//
// It is the middle ground between cpumemfit (always the single top) and random
// (random among all qualified): random among the best. This spreads load without
// ignoring fit. The score is the same as cpumemfit's.
package bestrandomfit

import (
	"cmp"
	"encoding/json"
	"math/rand/v2"
	"scheduler/calculate/schedulers/placement"
	"scheduler/logger"
	"slices"
)

// DefaultTolerance is the score epsilon defining the "best band": candidates whose
// score is within Tolerance of the max are all considered top and one is picked at random.
const DefaultTolerance = 1.0

// Constraints holds the placement directives embedded in a job descriptor.
type Constraints struct {
	placement.GenericConstraints
}

// Resources serves as both the job descriptor (J) and the placement candidate (C)
// in Algorithm[Resources, Resources].
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

// Scheduler picks at random among the qualifying candidates whose score is within
// Tolerance of the best. It implements placement.Algorithm[Resources].
type Scheduler struct {
	// Tolerance widens the best band. 0 means only exact-max-score candidates.
	Tolerance float64
	// rng is optional; when nil, the math/rand/v2 global source is used. Injectable for tests.
	rng func(n int) int
}

func (Scheduler) JobData() Resources { return Resources{} }

func (s Scheduler) Calculate(job Resources, candidates []Resources) (Resources, error) {
	if len(candidates) == 0 {
		return Resources{}, placement.SchedulingError{NegativeSchedulingStatus: placement.TargetClusterNotActive}
	}
	filtered := filterRequirements(job, candidates)
	if len(filtered) == 0 {
		return Resources{}, placement.SchedulingError{NegativeSchedulingStatus: placement.NoActiveClusterWithCapacity}
	}

	// Sort ascending by score; the last element is the max.
	slices.SortFunc(filtered, cmpScore)
	top := score(filtered[len(filtered)-1])

	// Collect the best band: everything within Tolerance of the top score.
	best := make([]Resources, 0, len(filtered))
	for i := len(filtered) - 1; i >= 0; i-- {
		if top-score(filtered[i]) > s.Tolerance {
			break
		}
		best = append(best, filtered[i])
	}
	return best[s.pick(len(best))], nil
}

func (s Scheduler) pick(n int) int {
	if n <= 1 {
		return 0
	}
	if s.rng != nil {
		return s.rng(n)
	}
	return rand.IntN(n)
}

func filterRequirements(job Resources, candidates []Resources) []Resources {
	return placement.Filter(candidates, func(c Resources) bool {
		logger.DebugLogger().Printf("Filtering candidate: %v", c)
		return placement.MeetsBasicRequirements(job.BaseResources, c.BaseResources)
	})
}

// score combines free CPU headroom and available memory, as cpumemfit does.
func score(r Resources) float64 {
	return (100.0 - r.CPUPercent) + r.AvailableMem
}

func cmpScore(a, b Resources) int {
	return cmp.Compare(score(a), score(b))
}
