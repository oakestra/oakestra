// Package placement defines the core scheduling abstractions shared by all
// scheduler algorithm implementations.
package placement

import (
	"cmp"
	"errors"
	"slices"
)

// NegativeSchedulingStatus is a sentinel value indicating why scheduling failed.
type NegativeSchedulingStatus int

const (
	TargetClusterNotFound NegativeSchedulingStatus = iota
	TargetClusterNotActive
	NoActiveClusterWithCapacity
	NoWorkerCapacity
	NoQualifiedWorkerFound
	NoNodeFound
	// NoGpuCapacity is appended rather than inserted so the numeric values of
	// the statuses above stay stable.
	NoGpuCapacity
)

// NegativeSchedulingStatusName maps each status to the string sent on the wire
// to the manager. The string values are part of the external API contract and
// must not be changed.
var NegativeSchedulingStatusName = map[NegativeSchedulingStatus]string{
	TargetClusterNotFound:       "TargetClusterNotFound",
	TargetClusterNotActive:      "TargetClusterNotActive",
	NoActiveClusterWithCapacity: "NoActiveClusterWithCapacity",
	NoWorkerCapacity:            "NO_WORKER_CAPACITY",
	NoQualifiedWorkerFound:      "NO_QUALIFIED_WORKER_FOUND",
	NoNodeFound:                 "NO_NODE_FOUND",
	NoGpuCapacity:               "NoGpuCapacity",
}

// SchedulingError is returned by Algorithm.Calculate when no suitable placement
// candidate can be found.
type SchedulingError struct {
	NegativeSchedulingStatus NegativeSchedulingStatus
}

func (e SchedulingError) Error() string {
	return NegativeSchedulingStatusName[e.NegativeSchedulingStatus]
}

// GenericConstraints holds the placement-directive fields common to all schedulers.
type GenericConstraints struct {
	Type    string `json:"type"`
	Node    string `json:"node"`
	Cluster string `json:"cluster"`
}

// BaseResources holds the fields shared by all placement candidates and jobs.
// Embed it in concrete resource types to share common fields and helpers.
type BaseResources struct {
	// ID is the MongoDB document identifier for this candidate or job.
	ID string `json:"_id"`
	// Virtualization is handled by each concrete type's UnmarshalJSON
	// (normalised from string|[]string). The tag is kept so that
	// getInterestedResources includes "virtualization" in the resource-
	// abstractor projection query.
	Virtualization []string `json:"virtualization"`
	AvailableMem   float64  `json:"memory"`
	AvailableCPU   float64  `json:"vcpus"`
	CPUPercent     float64  `json:"cpu_percent"`
	// On a job, Vgpus and Vram are what the SLA asks for. On a candidate
	// they are the installed totals. GpuVramFree is candidate-only. The
	// matching rules are in the scheduler README.
	Vgpus       int       `json:"vgpus"`
	Vram        float64   `json:"vram"`
	GpuVramFree []float64 `json:"gpu_vram_free"`
}

// Candidate is a resource (cluster or worker node) that can be selected for a job.
type Candidate interface {
	// ID returns the identifier of this placement candidate.
	ID() string
}

// Job describes the resource requirements of a workload to be scheduled.
type Job interface {
	// ID returns the identifier of this job.
	ID() string
	// ResourceConstraints returns the placement constraints as a map used to
	// query the resource abstractor (e.g. direct-placement target name).
	ResourceConstraints() map[string]string
}

// Algorithm chooses the best Candidate for a Job.
//
// J and C are kept as separate type parameters to allow future implementations
// where the job descriptor and the placement candidate carry different fields.
// In practice all current implementations bind J and C to the same concrete
// type (e.g. cpumemfit.Resources for both). This is intentional: the
// ResourceAbstractor returns candidates using the same JSON schema as the
// incoming job payload, so the two roles have always shared one struct. The
// two-parameter form is forward-compatible without forcing that constraint on
// new algorithms.
type Algorithm[J Job, C Candidate] interface {
	// JobData returns an empty value used as a container for the incoming
	// job request.
	JobData() J
	// Calculate returns the best placement candidate for the given job.
	Calculate(job J, candidates []C) (C, error)
}

// MeetsBasicRequirements reports whether candidate satisfies the job's
// virtualization type, CPU, memory, and GPU requirements.
// It returns false when the job carries no virtualization entry, which is
// treated as a malformed request.
func MeetsBasicRequirements(job, candidate BaseResources) bool {
	return MeetsNonGPURequirements(job, candidate) && MeetsGPURequirements(job, candidate)
}

// MeetsNonGPURequirements is MeetsBasicRequirements without the GPU check.
// Schedulers use it to tell "no GPU capacity" apart from a generic capacity
// shortage when nothing fits.
func MeetsNonGPURequirements(job, candidate BaseResources) bool {
	if len(job.Virtualization) == 0 {
		return false
	}
	return slices.Contains(candidate.Virtualization, job.Virtualization[0]) &&
		candidate.AvailableCPU >= job.AvailableCPU &&
		candidate.AvailableMem >= job.AvailableMem
}

// Only containers get physical NVIDIA GPUs, which is what a candidate's vgpus
// and gpu_vram_free count. crosvm uses vgpus for virtio-gpu devices that run
// on any host GPU, and unikernels ignore it.
const gpuRuntime = "docker"

// RequiredGPUs returns how many NVIDIA GPUs the job needs. vram without vgpus
// implies one GPU. Non-container jobs need none.
func RequiredGPUs(job BaseResources) int {
	if len(job.Virtualization) == 0 || job.Virtualization[0] != gpuRuntime {
		return 0
	}
	if job.Vgpus <= 0 && job.Vram > 0 {
		return 1
	}
	return max(job.Vgpus, 0)
}

// MeetsGPURequirements reports whether candidate has at least as many GPUs as
// the job requests, each with at least the job's vram free.
//
// vram is per GPU rather than a total on purpose. Clusters report the
// element-wise max of their workers' sorted free-VRAM lists, and checking the
// n-th largest entry against that is exact: it passes only if a single worker
// has n GPUs that fit. A sum over that list would mix GPUs from different
// workers.
func MeetsGPURequirements(job, candidate BaseResources) bool {
	n := RequiredGPUs(job)
	if n == 0 {
		return true
	}
	free := freeVramPerGPU(candidate)
	return len(free) >= n && free[n-1] >= job.Vram
}

// freeVramPerGPU returns the candidate's free VRAM per GPU, largest first.
func freeVramPerGPU(candidate BaseResources) []float64 {
	if len(candidate.GpuVramFree) == 0 {
		// Older node engines and cluster managers do not report per-GPU free
		// VRAM. Assume the installed VRAM is spread evenly and entirely free,
		// which is optimistic but matches what they were scheduled on before.
		if candidate.Vgpus <= 0 {
			return nil
		}
		perGPU := candidate.Vram / float64(candidate.Vgpus)
		free := make([]float64, candidate.Vgpus)
		for i := range free {
			free[i] = perGPU
		}
		return free
	}
	// Workers don't guarantee an order. Sort a copy so free[n-1] is the n-th
	// best GPU without mutating the candidate.
	free := slices.Clone(candidate.GpuVramFree)
	slices.SortFunc(free, func(a, b float64) int { return cmp.Compare(b, a) })
	return free
}

// NoCapacityStatus picks the status to report when no candidate passed the
// full filter. meetsAllButGPU must apply every check of the scheduler's filter
// except MeetsGPURequirements. If the job needs a GPU and some candidate fails
// only on GPUs, the more specific NoGpuCapacity is returned so the user knows
// what to change.
func NoCapacityStatus[C any](
	job BaseResources, candidates []C, meetsAllButGPU func(C) bool,
) NegativeSchedulingStatus {
	if RequiredGPUs(job) > 0 && slices.ContainsFunc(candidates, meetsAllButGPU) {
		return NoGpuCapacity
	}
	return NoActiveClusterWithCapacity
}

// Filter returns the elements of candidates for which keep returns true.
func Filter[C any](candidates []C, keep func(C) bool) []C {
	filtered := make([]C, 0, len(candidates))
	for _, c := range candidates {
		if keep(c) {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// NormalizeVirtualization converts the polymorphic JSON "virtualization" field
// (string | []string | null) into a uniform []string.
func NormalizeVirtualization(raw any) ([]string, error) {
	switch v := raw.(type) {
	case string:
		return []string{v}, nil
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			str, ok := item.(string)
			if !ok {
				return nil, errors.New("invalid type in virtualization array")
			}
			result = append(result, str)
		}
		return result, nil
	case nil:
		return nil, nil
	default:
		return nil, errors.New("unexpected type for virtualization")
	}
}
