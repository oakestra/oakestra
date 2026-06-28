// Package placement defines the core scheduling abstractions shared by all
// scheduler algorithm implementations.
package placement

import (
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
type Algorithm[J Job, C Candidate] interface {
	// JobData returns an empty value used as a container for the incoming
	// job request.
	JobData() J
	// Calculate returns the best placement candidate for the given job.
	Calculate(job J, candidates []C) (C, error)
}

// MeetsBasicRequirements reports whether candidate satisfies the job's
// virtualization type, CPU, and memory requirements.
// It returns false when the job carries no virtualization entry, which is
// treated as a malformed request.
func MeetsBasicRequirements(job, candidate BaseResources) bool {
	if len(job.Virtualization) == 0 {
		return false
	}
	return slices.Contains(candidate.Virtualization, job.Virtualization[0]) &&
		candidate.AvailableCPU >= job.AvailableCPU &&
		candidate.AvailableMem >= job.AvailableMem
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
