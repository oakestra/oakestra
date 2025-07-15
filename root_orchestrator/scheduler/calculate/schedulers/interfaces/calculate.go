// Package interfaces
package interfaces

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
	ResourceList() []T
	JobData() T
	Calculate(job T, candidates []T) (T, error)
}
