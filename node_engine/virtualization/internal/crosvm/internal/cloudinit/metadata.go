package cloudinit

type MetaData struct {
	InstanceId    string  `json:"instance-id"`
	LocalHostname *string `json:"local-hostname,omitempty"`
}
