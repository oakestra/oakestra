package rootBestCpuMemFit

import (
	"scheduler/calculate/schedulers/interfaces"
	"slices"
)

type CpuMemConstraints struct {
	interfaces.GenericConstraints
}

// CpuMemResources implements ResourceList
type CpuMemResources struct {
	Constraints    CpuMemConstraints `json:"constraints"`
	Id             string            `json:"_id"`
	Virtualization []string          `json:"virtualization"`
	AvailableMem   float64           `json:"memory_in_mb"`
	AvailableCPU   float64           `json:"total_cpu_cores"`
	CPUPercent     float64           `json:"aggregated_cpu_percent"`
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

// BestCpuMemFit implements Algorithm
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

	slices.SortFunc(filteredCandidates, cmpMemCpu)

	return filteredCandidates[0]
}

// filterRequirements returns a slice of PlacementCandidates which meet the job requirements
func filterRequirements(job CpuMemResources, candidates []CpuMemResources) []CpuMemResources {
	filteredCandidates := make([]CpuMemResources, 0, len(candidates))
	for _, candidate := range candidates {
		if slices.Contains(candidate.Virtualization, job.Virtualization[0]) {
			if candidate.AvailableCPU >= job.AvailableCPU {
				if candidate.AvailableMem >= job.AvailableMem {
					filteredCandidates = append(filteredCandidates, candidate)
				}
			}
		}
	}
	return filteredCandidates
}

// cmpMemCpu compares two PlacementCandidates with respect to available memory + cpu
func cmpMemCpu(a CpuMemResources, b CpuMemResources) int {
	scoreA := (100.00 - a.CPUPercent) + b.AvailableMem
	scoreB := (100.00 - b.CPUPercent) + b.AvailableMem

	if scoreA > scoreB {
		return 1
	}
	if scoreA < scoreB {
		return -1
	}
	return 0
}
