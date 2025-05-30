package cloudinit

type UserData struct {
	CloudInitModules   []string `json:"cloud_init_modules,omitzero"`
	CloudConfigModules []string `json:"cloud_config_modules,omitzero"`
	CloudFinalModules  []string `json:"cloud_final_modules,omitzero"`
}
