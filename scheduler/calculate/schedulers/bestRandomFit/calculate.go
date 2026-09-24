// Package bestRandomFit implements a placement algorithm that filters candidates by
// requirements, then picks UNIFORMLY AT RANDOM among the best-scoring band.
//
// It is the middle ground between bestCpuMemFit (always the single top) and rootRandom
// (random among ALL qualified): random among the best. This spreads load without ignoring
// fit.
package bestRandomFit

import (
	"math/rand/v2"
	"scheduler/calculate/schedulers/cpumem"
	"scheduler/calculate/schedulers/interfaces"
	"slices"
)

// DefaultTolerance is the score epsilon defining the "best band": candidates whose
// score is within Tolerance of the max are all considered top and one is picked at random.
const DefaultTolerance = 1.0

// BestRandomFit implements interfaces.Algorithm[cpumem.Resources].
type BestRandomFit struct {
	// Tolerance widens the best band. 0 means only exact-max-score candidates.
	Tolerance float64
	// rng is optional; when nil, math/rand/v2 global source is used. Injectable for tests.
	rng func(n int) int
}

func (a BestRandomFit) ResourceList() []cpumem.Resources {
	var data []cpumem.Resources
	return data
}

func (a BestRandomFit) JobData() cpumem.Resources {
	var data cpumem.Resources
	return data
}

func (a BestRandomFit) Calculate(job cpumem.Resources, candidates []cpumem.Resources) (cpumem.Resources, error) {
	if len(candidates) == 0 {
		return cpumem.Resources{}, interfaces.SchedulingError{NegativeSchedulingStatus: interfaces.TargetClusterNotActive}
	}
	filtered := cpumem.FilterRequirements(job, candidates)
	if len(filtered) == 0 {
		return cpumem.Resources{}, interfaces.SchedulingError{NegativeSchedulingStatus: interfaces.NoActiveClusterWithCapacity}
	}

	// Sort ascending by score; the last element is the max.
	slices.SortFunc(filtered, cpumem.CmpMemCpu)
	top := cpumem.Score(filtered[len(filtered)-1])

	// Collect the best band: everything within Tolerance of the top score.
	best := make([]cpumem.Resources, 0, len(filtered))
	for i := len(filtered) - 1; i >= 0; i-- {
		if top-cpumem.Score(filtered[i]) <= a.Tolerance {
			best = append(best, filtered[i])
		} else {
			break
		}
	}

	return best[a.pick(len(best))], nil
}

func (a BestRandomFit) pick(n int) int {
	if n <= 1 {
		return 0
	}
	if a.rng != nil {
		return a.rng(n)
	}
	return rand.IntN(n)
}
