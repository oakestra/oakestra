package model

type Service struct {
	JobID    string        `json:"job_id"`
	Image    string        `json:"image"`
	Commands string        `json:"commands"`
	Port     string        `json:"ports"`
	Status   ServiceStatus `json:"status"`
}

type ServiceStatus string

const (
	ACTIVE   ServiceStatus = "ACTIVE"
	CREATING ServiceStatus = "CREATING"
	DEAD     ServiceStatus = "DEAD"
)

var services = make([]Service, 0)
