package cloudinit

type UserData struct {
	CloudInitModules   []string          `json:"cloud_init_modules,omitzero"`
	CloudConfigModules []string          `json:"cloud_config_modules,omitzero"`
	CloudFinalModules  []string          `json:"cloud_final_modules,omitzero"`
	DisableRoot        *bool             `json:"disable_root,omitempty"`
	WriteFiles         []WriteFileConfig `json:"write_files,omitempty"`
}

type WriteFileConfig struct {
	Path        string  `json:"path"`
	Content     string  `json:"content"`
	Owner       *string `json:"owner,omitempty"`
	Permissions *string `json:"permissions,omitempty"`
	Append      *bool   `json:"append,omitempty"`
}
