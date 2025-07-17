package rootBestCpuMemFit

import (
	"encoding/json"
	"errors"
	"scheduler/calculate/schedulers/interfaces"
	"scheduler/logger"
	"slices"
)

type CpuMemConstraints struct {
	interfaces.GenericConstraints
}

// CpuMemResources implements ResourceList
type CpuMemResources struct {
	Constraints    []CpuMemConstraints `json:"constraints"`
	Id             string              `json:"_id"`
	Virtualization []string            `json:"virtualization"`
	AvailableMem   float64             `json:"memory"`
	AvailableCPU   float64             `json:"vcpus"`
	CPUPercent     float64             `json:"cpu_percent"`
}

func (r CpuMemResources) GetId() string {
	return r.Id
}

func (r CpuMemResources) ResourceConstraints() map[string]string {
	var constraints = make(map[string]string)
	for _, constraint := range r.Constraints {
		logger.DebugLogger().Printf("Constraint: %+v", constraint)
		if constraint.Type == "direct" {
			constraints["cluster_name"] = constraint.Cluster
			constraints["node_name"] = constraint.Node
		}
	}
	return constraints
}

func (r *CpuMemResources) UnmarshalJSON(data []byte) error {
	// Create a shadow struct with the same fields,
	// but use `interface{}` for fields you want to custom-parse
	type Alias CpuMemResources
	aux := &struct {
		Virtualization interface{} `json:"virtualization"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Handle the virtualization field manually
	switch v := aux.Virtualization.(type) {
	case string:
		r.Virtualization = []string{v}
	case []interface{}:
		var result []string
		for _, item := range v {
			if str, ok := item.(string); ok {
				result = append(result, str)
			} else {
				return errors.New("invalid type in virtualization array")
			}
		}
		r.Virtualization = result
	case nil:
		r.Virtualization = nil
	default:
		return errors.New("unexpected type for virtualization")
	}

	return nil
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

func (a BestCpuMemFit) Calculate(job CpuMemResources, candidates []CpuMemResources) (CpuMemResources, error) {
	if len(candidates) == 0 {
		return CpuMemResources{}, interfaces.SchedulingError{NegativeSchedulingStatus: interfaces.TargetClusterNotActive}
	}
	filteredCandidates := filterRequirements(job, candidates)

	if len(filteredCandidates) == 0 {
		return CpuMemResources{}, interfaces.SchedulingError{NegativeSchedulingStatus: interfaces.TargetClusterNoCapacity}
	}

	slices.SortFunc(filteredCandidates, cmpMemCpu)

	return filteredCandidates[0], nil
}

// filterRequirements returns a slice of PlacementCandidates which meet the job requirements
func filterRequirements(job CpuMemResources, candidates []CpuMemResources) []CpuMemResources {
	filteredCandidates := make([]CpuMemResources, 0, len(candidates))
	for _, candidate := range candidates {
		logger.DebugLogger().Printf("Filtering candidate: %v", candidate)
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
