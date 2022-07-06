package model

type Service struct {
	JobID        string   `json:"_id"`
	Sname        string   `json:"job_name"`
	Instance     int      `json:"instance_number"`
	Image        string   `json:"image"`
	Commands     []string `json:"cmd"`
	Env          []string `json:"environment"`
	Ports        string   `json:"port"`
	Status       string   `json:"status"`
	Runtime      string   `json:"virtualization"`
	StatusDetail string   `json:"status_detail"`
	Vtpus        int      `json:"vtpus"`
	Vgpus        int      `json:"vgpus"`
	Vcpus        int      `json:"vcpus"`
	Memory       int      `json:"memory"`
	Pid          int
}

type Resources struct {
	Cpu      string `json:"cpu"`
	Memory   string `json:"memory"`
	Disk     string `json:"disk"`
	Sname    string `json:"job_name"`
	Runtime  string `json:"virtualization"`
	Instance int    `json:"instance"`
}

const (
	SERVICE_ACTIVE     = "ACTIVE"
	SERVICE_CREATING   = "CREATING"
	SERVICE_DEAD       = "DEAD"
	SERVICE_FAILED     = "FAILED"
	SERVICE_UNDEPLOYED = "UNDEPLOYED"
)
