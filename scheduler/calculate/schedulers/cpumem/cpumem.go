package cpumem

import (
	"encoding/json"
	"errors"
	"os"
	"scheduler/calculate/schedulers/interfaces"
	"scheduler/logger"
	"slices"
)

// Plane is "cluster" at the cluster orchestrator and empty/"root" elsewhere. It selects
// whether a "direct" placement constraint targets a node or a cluster.
var Plane = os.Getenv("ORCHESTRATION_PLANE")

type Constraints struct {
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

// Resources implements interfaces.ResourceList for CPU/memory-based placement.
type Resources struct {
	Constraints    []Constraints `json:"constraints"`
	Id             string        `json:"_id"`
	Virtualization []string      `json:"virtualization"`
	CSIDrivers     []string      `json:"csi_drivers"`
	Volumes        []VolumeSpec  `json:"volumes"`
	AvailableMem   float64       `json:"memory"`
	AvailableCPU   float64       `json:"vcpus"`
	CPUPercent     float64       `json:"cpu_percent"`
}

func (r Resources) GetId() string {
	return r.Id
}

func (r Resources) ResourceConstraints() map[string]string {
	var constraints = make(map[string]string)
	for _, constraint := range r.Constraints {
		logger.DebugLogger().Printf("Constraint: %+v", constraint)
		if constraint.Type == "direct" {
			var c string
			if Plane == "cluster" {
				c = constraint.Node
			} else {
				c = constraint.Cluster
			}
			constraints["candidate_name"] = c
		}
	}
	return constraints
}

func (r *Resources) UnmarshalJSON(data []byte) error {
	// Shadow struct: intercept polymorphic fields, let the Alias absorb the rest
	// (including Volumes []VolumeSpec which auto-deserialises from "volumes").
	type Alias Resources
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
	r.CSIDrivers = normaliseCsiDrivers(aux.CSIDrivers)

	return nil
}

// normaliseCsiDrivers accepts the raw JSON value of the "csi_drivers" key and
// returns a deduplicated slice of driver-name strings.
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

// FilterRequirements returns the candidates that satisfy the job requirements
// (virtualization, cpu, mem, CSI drivers).
func FilterRequirements(job Resources, candidates []Resources) []Resources {
	filtered := make([]Resources, 0, len(candidates))
	for _, candidate := range candidates {
		logger.DebugLogger().Printf("Filtering candidate: %v", candidate)
		if len(job.Virtualization) == 0 || !slices.Contains(candidate.Virtualization, job.Virtualization[0]) {
			continue
		}
		if candidate.AvailableCPU < job.AvailableCPU {
			continue
		}
		if candidate.AvailableMem < job.AvailableMem {
			continue
		}
		if !HasRequiredCSIDrivers(job, candidate) {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

// HasRequiredCSIDrivers checks that the candidate advertises every CSI driver
// referenced in the job's volume list.
func HasRequiredCSIDrivers(job Resources, candidate Resources) bool {
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

// Score is the higher-is-better placement score (free cpu headroom + available mem).
func Score(r Resources) float64 {
	return (100.00 - r.CPUPercent) + r.AvailableMem
}

// CmpMemCpu orders two candidates ascending by Score (for slices.SortFunc).
func CmpMemCpu(a Resources, b Resources) int {
	sa, sb := Score(a), Score(b)
	if sa > sb {
		return 1
	}
	if sa < sb {
		return -1
	}
	return 0
}
