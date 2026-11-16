package gpuconfig

type RuntimeGpuConfig struct {
	DrmEnabled bool
}

func NewRuntimeGpuConfig(options []string) *RuntimeGpuConfig {
	drmEnabled := false
	for _, option := range options {
		if option == "gpu-drm=true" {
			drmEnabled = true
		}
	}
	return &RuntimeGpuConfig{
		DrmEnabled: drmEnabled,
	}
}
