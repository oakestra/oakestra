package crosvm

// InstanceConfigExt represents a selection of parameters of the "crosvm run" command that cannot be passed via the "--cfg" method.
//
// See InstanceConfig for an explanation.
type InstanceConfigExt struct {
	Gpu []InstanceConfigExtGpu
}

// InstanceConfigExtGpu represents the set of sub-options that can be specified in the "--gpu" argument of "crosvm run".
// The legacy "width" and "height" options are omitted.
type InstanceConfigExtGpu struct {
	Backend              *string
	MaxNumDisplays       *uint32
	AudioDeviceMode      *string
	Displays             []InstanceConfigExtGpuDisplay
	Egl                  *bool
	Gles                 *bool
	Glx                  *bool
	Surfaceless          *bool
	Vulkan               *bool
	Wsi                  *string
	Udmabuf              *bool
	CachePath            *string
	CacheSize            *string
	PciAddress           *string
	PciBarSize           *uint64
	ContextTypes         []string
	ExternalBlob         *bool
	SystemBlob           *bool
	FixedBlobMapping     *bool
	ImplicitRenderServer *bool
	RendererFeatures     *string
	SnapshotScratchPath  *string
}

// InstanceConfigExtGpuDisplay represents the set of sub-options that can be specified in the displays sub-option of the "--gpu" argument of "crosvm run".
// The legacy "horizontal-dpi" and "vertical-dpi" options are omitted.
type InstanceConfigExtGpuDisplay struct {
	ModeWindowed *InstanceConfigExtGpuDisplayModeWindowed
	Hidden       *bool
	RefreshRate  *uint32
	Dpi          *InstanceConfigExtGpuDisplayDpi
}

type InstanceConfigExtGpuDisplayModeWindowed struct {
	Width  uint32
	Height uint32
}

type InstanceConfigExtGpuDisplayDpi struct {
	Horizontal uint32
	Vertical   uint32
}
