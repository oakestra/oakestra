package rootBestCpuMemFit

import (
	"encoding/json"
	"errors"
	"math/rand/v2"
	"scheduler/calculate/schedulers/interfaces"
	"scheduler/logger"
	"slices"
)

type RandomConstraints struct {
	interfaces.GenericConstraints
}

// RandomResources implements ResourceList
type RandomResources struct {
	Constraints    []RandomConstraints `json:"constraints"`
	Id             string              `json:"_id"`
	Virtualization []string            `json:"virtualization"`
	AvailableMem   float64             `json:"memory"`
	AvailableCPU   float64             `json:"vcpus"`
	CPUPercent     float64             `json:"cpu_percent"`
}

func (r RandomResources) GetId() string {
	return r.Id
}

func (r RandomResources) ResourceConstraints() map[string]string {
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

func (r *RandomResources) UnmarshalJSON(data []byte) error {
	// Create a shadow struct with the same fields,
	// but use `interface{}` for fields you want to custom-parse
	type Alias RandomResources
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

func (a BestCpuMemFit) ResourceList() []RandomResources {
	var data []RandomResources
	return data
}

func (a BestCpuMemFit) JobData() RandomResources {
	var data RandomResources
	return data
}

func (a BestCpuMemFit) Calculate(job RandomResources, candidates []RandomResources) (RandomResources, error) {
	if len(candidates) == 0 {
		return RandomResources{}, interfaces.SchedulingError{NegativeSchedulingStatus: interfaces.TargetClusterNotActive}
	}
	filteredCandidates := filterRequirements(job, candidates)

	if len(filteredCandidates) == 0 {
		return RandomResources{}, interfaces.SchedulingError{NegativeSchedulingStatus: interfaces.TargetClusterNoCapacity}
	}

	idx := rand.IntN(len(filteredCandidates))

	return filteredCandidates[idx], nil
}

// filterRequirements returns a slice of PlacementCandidates which meet the job requirements
func filterRequirements(job RandomResources, candidates []RandomResources) []RandomResources {
	filteredCandidates := make([]RandomResources, 0, len(candidates))
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
