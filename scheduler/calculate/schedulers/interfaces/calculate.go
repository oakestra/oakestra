// Package interfaces
package interfaces

type NegativeSchedulingStatus int

const (
	TargetClusterNotFound NegativeSchedulingStatus = iota
	TargetClusterNotActive
	TargetClusterNoCapacity
	NoActiveClusterWithCapacity
	NoWorkerCapacity
	NoQualifiedWorkerFound
	NoNodeFound
)

var NegativeSchedulingStatusName = map[NegativeSchedulingStatus]string{
	TargetClusterNotFound:       "TargetClusterNotFound",
	TargetClusterNotActive:      "TargetClusterNotActive",
	TargetClusterNoCapacity:     "TargetClusterNoCapacity",
	NoActiveClusterWithCapacity: "NoActiveClusterWithCapacity",
	NoWorkerCapacity:            "NO_WORKER_CAPACITY",
	NoQualifiedWorkerFound:      "NO_QUALIFIED_WORKER_FOUND",
	NoNodeFound:                 "NO_NODE_FOUND",
}

type SchedulingError struct {
	NegativeSchedulingStatus NegativeSchedulingStatus
}

func (e SchedulingError) Error() string {
	return NegativeSchedulingStatusName[e.NegativeSchedulingStatus]
}

type GenericConstraints struct {
	Type    string `json:"type"`
	Node    string `json:"node"`
	Cluster string `json:"cluster"`
}

// ResourceList is a map of named resources and values
type ResourceList interface {
	GetId() string                          // Must implement the id of the placement candidate or job
	ResourceConstraints() map[string]string // Job constraints as map
}

// Algorithm chooses the best PlacementCandidate for a Job
type Algorithm[T ResourceList] interface {
	ResourceList() []T                          // Return a slice of empty ResourceList objects as a containers for the Resource Abstractor response
	JobData() T                                 // Return an empty ResourceList object as a container for the job request
	Calculate(job T, candidates []T) (T, error) // Return the best placement candidate with respect to the job
}
