package rootRandom

import (
	"math/rand/v2"
	"scheduler/calculate/schedulers/interfaces"
)

type CpuMemConstraints struct {
	interfaces.GenericConstraints
}

type CpuMemResources struct {
	Constraints    CpuMemConstraints `json:"constraints"`
	Id             string            `json:"_id"`
	Virtualization bool              `json:"virtualization"`
	AvailableMem   float64           `json:"memory_in_mb"`
	AvailableCPU   float64           `json:"total_cpu_cores"`
}

func (r CpuMemResources) GetId() string {
	return r.Id
}

func (r CpuMemResources) ResourceConstraints() map[string]string {
	var constraints map[string]string
	if r.Constraints.Type == "direct" {
		constraints["cluster_name"] = r.Constraints.Cluster
		constraints["node_name"] = r.Constraints.Node
	}
	return constraints
}

type BestCpuMemFit struct{}

func (a BestCpuMemFit) ResourceList() []CpuMemResources {
	var data []CpuMemResources
	return data
}

func (a BestCpuMemFit) JobData() CpuMemResources {
	var data CpuMemResources
	return data
}

func (a BestCpuMemFit) Calculate(job CpuMemResources, candidates []CpuMemResources) CpuMemResources {
	filteredCandidates := filterRequirements(job, candidates)

	idx := rand.IntN(len(filteredCandidates))

	return filteredCandidates[idx]
}

// filterRequirements returns a slice of PlacementCandidates which meet the job requirements
func filterRequirements(job CpuMemResources, candidates []CpuMemResources) []CpuMemResources {
	filteredCandidates := make([]CpuMemResources, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Virtualization == job.Virtualization {
			if candidate.AvailableCPU >= job.AvailableCPU {
				if candidate.AvailableMem >= job.AvailableMem {
					filteredCandidates = append(filteredCandidates, candidate)
				}
			}
		}
	}
	return filteredCandidates
}
