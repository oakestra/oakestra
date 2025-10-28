package bestMemoryFitLatencyAware

import (
	"encoding/json"
	"errors"
	"scheduler/calculate/schedulers/interfaces"
	"scheduler/logger"
	"slices"
)

type LatencyAwareConstraints struct {
	interfaces.GenericConstraints
}

type LatencyAwareResources struct {
	Constraints    []LatencyAwareConstraints `json:"constraints"`
	Id             string                    `json:"_id"`
	Virtualization []string                  `json:"virtualization"`
	AvailableMem   float64                   `json:"memory"`
	AvailableCPU   float64                   `json:"vcpus"`
	CPUPercent     float64                   `json:"cpu_percent"`
	JobName        string                    `json:"job_name"`
	Latency        map[string]int            `json:"latency"` // map candidate/job->latency
}

func (r LatencyAwareResources) GetId() string {
	return r.Id
}

func (r LatencyAwareResources) ResourceConstraints() map[string]string {
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

func (r *LatencyAwareResources) UnmarshalJSON(data []byte) error {
	// Create a shadow struct with the same fields,
	// but use `interface{}` for fields you want to custom-parse
	type Alias LatencyAwareResources
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

type BestMemoryFitLatencyAware struct {
	Deployments map[string]map[string]bool // maps names of jobs to set of candidate _ids (instance of a job could be spread across multiple candidates
}

func (a BestMemoryFitLatencyAware) ResourceList() []LatencyAwareResources {
	var data []LatencyAwareResources
	return data
}

func (a BestMemoryFitLatencyAware) JobData() LatencyAwareResources {
	var data LatencyAwareResources
	return data
}

func (a *BestMemoryFitLatencyAware) Calculate(job LatencyAwareResources, candidates []LatencyAwareResources) (LatencyAwareResources, error) {
	if len(candidates) == 0 {
		return LatencyAwareResources{}, interfaces.SchedulingError{NegativeSchedulingStatus: interfaces.TargetClusterNotActive}
	}

	filteredCandidates := a.filterRequirements(job, candidates)

	if len(filteredCandidates) == 0 {
		return LatencyAwareResources{}, interfaces.SchedulingError{NegativeSchedulingStatus: interfaces.NoActiveClusterWithCapacity}
	}

	// todo revert to non-stable, only useful for testing
	slices.SortStableFunc(filteredCandidates, cmpMemory)
	res := filteredCandidates[0]

	// update deployments
	if a.Deployments == nil {
		a.Deployments = make(map[string]map[string]bool)
	}

	if a.Deployments[job.JobName] == nil {
		a.Deployments[job.JobName] = make(map[string]bool)
	}

	a.Deployments[job.JobName][res.Id] = true
	return res, nil
}

// filterRequirements returns a slice of PlacementCandidates which meet the job requirements
func (a BestMemoryFitLatencyAware) filterRequirements(job LatencyAwareResources, candidates []LatencyAwareResources) []LatencyAwareResources {
	filteredCandidates := make([]LatencyAwareResources, 0, len(candidates))
	for _, candidate := range candidates {
		if slices.Contains(candidate.Virtualization, job.Virtualization[0]) {
			if candidate.AvailableCPU >= job.AvailableCPU {
				if candidate.AvailableMem >= job.AvailableMem {
					if a.checkLatencyRequirement(job, candidate) {
						filteredCandidates = append(filteredCandidates, candidate)
					}
				}
			}
		}
	}
	return filteredCandidates
}

// checkLatencyRequirements verifies if the latency to every dependency is below the given threshold
func (a BestMemoryFitLatencyAware) checkLatencyRequirement(job LatencyAwareResources, candidate LatencyAwareResources) bool {
	for dependency, latency := range job.Latency {
		if res, ok := a.Deployments[dependency]; ok {
			// if dependency is deployed on at least one candidate with threshold
			found := false
			for c := range res {
				if l, ok := candidate.Latency[c]; ok && l <= latency {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		} else {
			// dependency not deployed
		}
	}
	return true
}

// cmpMemory compares two candidates according to their available memory
func cmpMemory(a LatencyAwareResources, b LatencyAwareResources) int {
	if a.AvailableMem > b.AvailableMem {
		return 1
	}
	if a.AvailableMem < b.AvailableMem {
		return -1
	}
	return 0
}

// cmpMemCpu compares two PlacementCandidates with respect to available memory + cpu
func cmpMemCpu(a LatencyAwareResources, b LatencyAwareResources) int {
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
