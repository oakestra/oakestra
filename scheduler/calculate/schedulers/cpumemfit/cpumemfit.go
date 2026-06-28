// Package cpumemfit implements a best-fit scheduling algorithm that places
// jobs on the candidate with the most available CPU headroom and memory.
package cpumemfit

import (
	"cmp"
	"encoding/json"
	"os"
	"scheduler/calculate/schedulers/placement"
	"scheduler/logger"
	"slices"
)

var plane = os.Getenv("ORCHESTRATION_PLANE")

// Constraints holds the placement directives embedded in a job descriptor.
type Constraints struct {
	placement.GenericConstraints
}

// VolumeSpec mirrors the "volumes" entry in the Oakestra deployment descriptor.
// Only the csi_driver field is used for scheduling purposes.
type VolumeSpec struct {
	VolumeID  string            `json:"volume_id"`
	CSIDriver string            `json:"csi_driver"`
	MountPath string            `json:"mount_path"`
	Config    map[string]string `json:"config"`
}

// Resources is a placement candidate or job descriptor for the CPU+memory scheduler.
// It implements placement.ResourceList.
type Resources struct {
	placement.BaseResources
	Constraints []Constraints `json:"constraints"`
	// CSIDrivers lists the CSI plugin driver names available on this cluster node.
	// A deployment that requests a specific CSI driver will only be scheduled on
	// nodes/clusters that advertise that driver.
	CSIDrivers []string     `json:"csi_drivers"`
	Volumes    []VolumeSpec `json:"volumes"`
}

// ID returns the candidate or job identifier.
func (r Resources) ID() string { return r.BaseResources.ID }

// ResourceConstraints returns the scheduling constraints used to query the
// resource abstractor.
func (r Resources) ResourceConstraints() map[string]string {
	constraints := make(map[string]string)
	for _, c := range r.Constraints {
		logger.DebugLogger().Printf("Constraint: %+v", c)
		if c.Type == "direct" {
			var name string
			if plane == "cluster" {
				name = c.Node
			} else {
				name = c.Cluster
			}
			constraints["candidate_name"] = name
		}
	}
	return constraints
}

func (r *Resources) UnmarshalJSON(data []byte) error {
	// Shadow struct: intercept polymorphic fields; let the Alias absorb the rest
	// (including Volumes []VolumeSpec which auto-deserialises from "volumes").
	type Alias Resources
	aux := &struct {
		Virtualization any `json:"virtualization"`
		// csi_drivers can arrive as []string (root-level, already aggregated)
		// or as []object {csi_driver_name, csi_driver_endpoint} (cluster-level,
		// sent verbatim by the Node Engine during worker registration).
		CSIDrivers any `json:"csi_drivers"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	virt, err := placement.NormalizeVirtualization(aux.Virtualization)
	if err != nil {
		return err
	}
	r.BaseResources.Virtualization = virt

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
func normaliseCsiDrivers(raw any) []string {
	if raw == nil {
		return nil
	}
	items, ok := raw.([]any)
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
		case map[string]any:
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

// Scheduler picks the candidate with the most available CPU headroom and memory.
// It implements placement.Algorithm[Resources].
type Scheduler struct{}

func (Scheduler) JobData() Resources { return Resources{} }

func (Scheduler) Calculate(job Resources, candidates []Resources) (Resources, error) {
	if len(candidates) == 0 {
		return Resources{}, placement.SchedulingError{NegativeSchedulingStatus: placement.TargetClusterNotActive}
	}
	filtered := filterRequirements(job, candidates)
	if len(filtered) == 0 {
		return Resources{}, placement.SchedulingError{NegativeSchedulingStatus: placement.NoActiveClusterWithCapacity}
	}
	slices.SortFunc(filtered, cmpMemCPU)
	return filtered[len(filtered)-1], nil
}

// filterRequirements returns the candidates that meet the job's resource and
// constraint requirements.
func filterRequirements(job Resources, candidates []Resources) []Resources {
	return placement.Filter(candidates, func(c Resources) bool {
		logger.DebugLogger().Printf("Filtering candidate: %v", c)
		return placement.MeetsBasicRequirements(job.BaseResources, c.BaseResources) && hasRequiredCSIDrivers(job, c)
	})
}

// hasRequiredCSIDrivers reports whether candidate advertises every CSI driver
// referenced in the job's volume list.
func hasRequiredCSIDrivers(job, candidate Resources) bool {
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

// cmpMemCPU orders Resources by a combined score of free CPU headroom and
// available memory, ascending (highest score last, returned by Calculate).
func cmpMemCPU(a, b Resources) int {
	scoreA := (100.0 - a.CPUPercent) + a.AvailableMem
	scoreB := (100.0 - b.CPUPercent) + b.AvailableMem
	return cmp.Compare(scoreA, scoreB)
}
