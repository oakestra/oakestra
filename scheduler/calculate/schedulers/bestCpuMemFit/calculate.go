package bestCpuMemFit

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

// VolumeSpec mirrors the "volumes" entry in the Oakestra deployment descriptor.
// Only the csi_driver field is used for scheduling purposes.
type VolumeSpec struct {
	VolumeID  string            `json:"volume_id"`
	CSIDriver string            `json:"csi_driver"`
	MountPath string            `json:"mount_path"`
	Config    map[string]string `json:"config"`
}

// CpuMemResources implements ResourceList
type CpuMemResources struct {
	Constraints    []CpuMemConstraints `json:"constraints"`
	Id             string              `json:"_id"`
	Virtualization []string            `json:"virtualization"`
	// CSIDrivers lists the CSI plugin driver names available on this cluster node.
	// A deployment that requests a specific CSI driver will only be scheduled on
	// nodes/clusters that advertise that driver.
	CSIDrivers   []string     `json:"csi_drivers"`
	Volumes      []VolumeSpec `json:"volumes"`
	AvailableMem float64      `json:"memory"`
	AvailableCPU float64      `json:"vcpus"`
	CPUPercent   float64      `json:"cpu_percent"`
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
	// Shadow struct: intercept polymorphic fields, let the Alias absorb the rest
	// (including Volumes []VolumeSpec which auto-deserialises from "volumes").
	type Alias CpuMemResources
	aux := &struct {
		Virtualization interface{} `json:"virtualization"`
		// csi_drivers can arrive as []string (root-level, already aggregated)
		// or as []object {csi_driver_name, csi_driver_endpoint} (cluster-level,
		// sent verbatim by the Node Engine during worker registration).
		CSIDrivers interface{} `json:"csi_drivers"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// --- virtualization (string | []string) ---
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

	// --- csi_drivers: []string | []{csi_driver_name, ...} | nil ---
	// Normalise to a deduplicated flat list of driver-name strings regardless
	// of whether we received pre-aggregated strings (root level) or raw
	// CSIDriverType objects from the Node Engine (cluster level).
	r.CSIDrivers = normaliseCsiDrivers(aux.CSIDrivers)

	return nil
}

// normaliseCsiDrivers accepts the raw JSON value of the "csi_drivers" key and
// returns a deduplicated slice of driver-name strings. It handles:
//
//	nil                                     → nil
//	["nfs.csi.k8s.io", ...]                → same (root-level aggregated)
//	[{"csi_driver_name":"nfs.csi.k8s.io"}] → extracted names (worker-level raw)
func normaliseCsiDrivers(raw interface{}) []string {
	if raw == nil {
		return nil
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		var name string
		switch v := item.(type) {
		case string:
			name = v
		case map[string]interface{}:
			// Node Engine wire format: {csi_driver_name: "...", csi_driver_endpoint: "..."}
			if n, ok := v["csi_driver_name"].(string); ok {
				name = n
			}
		}
		if name == "" {
			continue
		}
		if _, dup := seen[name]; !dup {
			seen[name] = struct{}{}
			result = append(result, name)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
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
		return CpuMemResources{}, interfaces.SchedulingError{NegativeSchedulingStatus: interfaces.NoActiveClusterWithCapacity}
	}

	slices.SortFunc(filteredCandidates, cmpMemCpu)

	return filteredCandidates[len(filteredCandidates)-1], nil
}

// filterRequirements returns a slice of PlacementCandidates which meet the job requirements
func filterRequirements(job CpuMemResources, candidates []CpuMemResources) []CpuMemResources {
	filteredCandidates := make([]CpuMemResources, 0, len(candidates))
	for _, candidate := range candidates {
		logger.DebugLogger().Printf("Filtering candidate: %v", candidate)
		if slices.Contains(candidate.Virtualization, job.Virtualization[0]) {
			if candidate.AvailableCPU >= job.AvailableCPU {
				if candidate.AvailableMem >= job.AvailableMem {
					if hasRequiredCSIDrivers(job, candidate) {
						filteredCandidates = append(filteredCandidates, candidate)
					}
				}
			}
		}
	}
	return filteredCandidates
}

// hasRequiredCSIDrivers checks that the candidate advertises every CSI driver
// referenced in the job's volume list.
func hasRequiredCSIDrivers(job CpuMemResources, candidate CpuMemResources) bool {
	for _, vol := range job.Volumes {
		if vol.CSIDriver == "" {
			continue
		}
		if !slices.Contains(candidate.CSIDrivers, vol.CSIDriver) {
			logger.DebugLogger().Printf(
				"Candidate %s does not have required CSI driver %s (available: %v)",
				candidate.Id, vol.CSIDriver, candidate.CSIDrivers,
			)
			return false
		}
	}
	return true
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
