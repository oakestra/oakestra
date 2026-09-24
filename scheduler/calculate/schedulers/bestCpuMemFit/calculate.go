package bestCpuMemFit

import (
	"scheduler/calculate/schedulers/cpumem"
	"scheduler/calculate/schedulers/interfaces"
	"slices"
)

// BestCpuMemFit picks the single best-scoring candidate that satisfies the job.
// It implements interfaces.Algorithm[cpumem.Resources].
type BestCpuMemFit struct{}

func (a BestCpuMemFit) ResourceList() []cpumem.Resources {
	var data []cpumem.Resources
	return data
}

func (a BestCpuMemFit) JobData() cpumem.Resources {
	var data cpumem.Resources
	return data
}

func (a BestCpuMemFit) Calculate(job cpumem.Resources, candidates []cpumem.Resources) (cpumem.Resources, error) {
	if len(candidates) == 0 {
		return cpumem.Resources{}, interfaces.SchedulingError{NegativeSchedulingStatus: interfaces.TargetClusterNotActive}
	}
	filtered := cpumem.FilterRequirements(job, candidates)
	if len(filtered) == 0 {
		return cpumem.Resources{}, interfaces.SchedulingError{NegativeSchedulingStatus: interfaces.NoActiveClusterWithCapacity}
	}

	slices.SortFunc(filtered, cpumem.CmpMemCpu)
	return filtered[len(filtered)-1], nil
}
