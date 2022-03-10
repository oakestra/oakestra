package model

type Service struct {
	JobID        string   `json:"_id"`
	Sname        string   `json:"job_name"`
	Image        string   `json:"image"`
	Commands     []string `json:"commands"`
	Port         int      `json:"port"`
	Status       string   `json:"status"`
	Runtime      string   `json:"image_runtime"`
	StatusDetail string   `json:"status_detail"`
	Pid          int
}

const (
	SERVICE_ACTIVE     = "ACTIVE"
	SERVICE_CREATING   = "CREATING"
	SERVICE_DEAD       = "DEAD"
	SERVICE_FAILED     = "FAILED"
	SERVICE_UNDEPLOYED = "UNDEPLOYED"
)
