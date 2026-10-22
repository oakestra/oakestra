// Package bestrandomfit implements a scheduling algorithm that filters candidates
// by requirements, then picks uniformly at random among the best-scoring band.
//
// It is the middle ground between cpumemfit (always the single top) and random
// (random among all qualified): random among the best. This spreads load without
// ignoring fit. It shares cpumemfit's Resources schema, requirement filter (including
// CSI drivers) and score; only the final pick differs.
package bestrandomfit

import (
	"cmp"
	"math/rand/v2"
	"scheduler/calculate/schedulers/cpumemfit"
	"scheduler/calculate/schedulers/placement"
	"scheduler/logger"
	"slices"
)

// DefaultTolerance is the score epsilon defining the "best band": candidates whose
// score is within Tolerance of the max are all considered top and one is picked at random.
const DefaultTolerance = 1.0

// Scheduler picks at random among the qualifying candidates whose score is within
// Tolerance of the best. It implements placement.Algorithm[cpumemfit.Resources].
type Scheduler struct {
	// Tolerance widens the best band. 0 means only exact-max-score candidates.
	Tolerance float64
	// rng is optional; when nil, the math/rand/v2 global source is used. Injectable for tests.
	rng func(n int) int
}

func (Scheduler) JobData() cpumemfit.Resources { return cpumemfit.Resources{} }

func (s Scheduler) Calculate(job cpumemfit.Resources, candidates []cpumemfit.Resources) (cpumemfit.Resources, error) {
	if len(candidates) == 0 {
		return cpumemfit.Resources{}, placement.SchedulingError{NegativeSchedulingStatus: placement.TargetClusterNotActive}
	}
	filtered := filterRequirements(job, candidates)
	if len(filtered) == 0 {
		return cpumemfit.Resources{}, placement.SchedulingError{NegativeSchedulingStatus: placement.NoActiveClusterWithCapacity}
	}

	// Sort ascending by score; the last element is the max.
	slices.SortFunc(filtered, cmpScore)
	top := score(filtered[len(filtered)-1])

	// Collect the best band: everything within Tolerance of the top score.
	best := make([]cpumemfit.Resources, 0, len(filtered))
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

// filterRequirements returns the candidates that meet the job's resource and CSI
// driver requirements, mirroring cpumemfit.
func filterRequirements(job cpumemfit.Resources, candidates []cpumemfit.Resources) []cpumemfit.Resources {
	return placement.Filter(candidates, func(c cpumemfit.Resources) bool {
		logger.DebugLogger().Printf("Filtering candidate: %v", c)
		return placement.MeetsBasicRequirements(job.BaseResources, c.BaseResources) && hasRequiredCSIDrivers(job, c)
	})
}

// hasRequiredCSIDrivers reports whether candidate advertises every CSI driver
// referenced in the job's volume list.
func hasRequiredCSIDrivers(job, candidate cpumemfit.Resources) bool {
	for _, vol := range job.Volumes {
		if vol.CSIDriver == "" {
			continue
		}
		if !slices.Contains(candidate.CSIDrivers, vol.CSIDriver) {
			logger.DebugLogger().Printf(
				"Candidate %s does not have required CSI driver %s (available: %v)",
				candidate.ID(), vol.CSIDriver, candidate.CSIDrivers,
			)
			return false
		}
	}
	return true
}

// score combines free CPU headroom and available memory, as cpumemfit does.
func score(r cpumemfit.Resources) float64 {
	return (100.0 - r.CPUPercent) + r.AvailableMem
}

func cmpScore(a, b cpumemfit.Resources) int {
	return cmp.Compare(score(a), score(b))
}
