package model

type Service struct {
	JobID        string                 `json:"_id"`
	Sname        string                 `json:"job_name"`
	Image        string                 `json:"image"`
	Commands     []string               `json:"commands"`
	Env          []string               `json:"environment"`
	Ports        string                 `json:"port"`
	Status       string                 `json:"status"`
	Runtime      string                 `json:"virtualization"`
	StatusDetail string                 `json:"status_detail"`
	Requirements map[string]interface{} `json:"requirements"`
	Pid          int
}

type Resources struct {
	Cpu    string `json:"cpu"`
	Memory string `json:"memory"`
	Disk   string `json:"disk"`
	Sname  string `json:"sname"`
}

const (
	SERVICE_ACTIVE     = "ACTIVE"
	SERVICE_CREATING   = "CREATING"
	SERVICE_DEAD       = "DEAD"
	SERVICE_FAILED     = "FAILED"
	SERVICE_UNDEPLOYED = "UNDEPLOYED"
)
