package instance

import (
	"fmt"
	"go_node_engine/model"
	"go_node_engine/util/ptr"
	"strings"
)

// InstanceConfigExt represents a selection of parameters of the "crosvm run" command that cannot be passed via the "--cfg" method.
//
// See InstanceConfig for an explanation.
type InstanceConfigExt struct {
	Gpu             []InstanceConfigExtGpu
	GpuRenderServer *InstanceConfigExtGpuRenderServer
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
// On Windows, there's an additional mode "borderless_full_screen", which is also omitted.
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

type InstanceConfigExtGpuRenderServer struct {
	Path                 string
	CachePath            *string
	CacheSize            *string
	FozDbListPath        *string
	PrecompiledCachePath *string
	LdPreloadPath        *string
}

func NewInstanceConfigExt(service *model.Service) *InstanceConfigExt {
	var gpus []InstanceConfigExtGpu
	for range service.Vgpus {
		gpus = append(gpus, InstanceConfigExtGpu{
			Backend:     ptr.Ptr("virglrenderer"),
			Egl:         ptr.Ptr(true),
			Gles:        ptr.Ptr(true),
			Glx:         ptr.Ptr(true),
			Surfaceless: ptr.Ptr(true),
			Vulkan:      ptr.Ptr(true),
			Udmabuf:     ptr.Ptr(true),
			ContextTypes: []string{
				"virgl",
				"virgl2",
				"venus",
				"drm",
			},
		})
	}
	var gpuRenderServer *InstanceConfigExtGpuRenderServer = nil
	if len(gpus) > 0 {
		gpuRenderServer = &InstanceConfigExtGpuRenderServer{
			Path: "/opt/oakestra/libexec/virgl_render_server",
		}
	}

	return &InstanceConfigExt{
		Gpu:             gpus,
		GpuRenderServer: gpuRenderServer,
	}
}

func (c *InstanceConfigExt) ToArgs() []string {
	var args []string
	for _, gpu := range c.Gpu {
		args = append(args, "--gpu", gpu.toArgString())
	}
	if c.GpuRenderServer != nil {
		args = append(args, "--gpu-render-server", c.GpuRenderServer.toArgString())
	}
	return args
}

func (c *InstanceConfigExtGpu) toArgString() string {
	var args []string
	if c.Backend != nil {
		args = append(args, fmt.Sprintf("backend=%s", *c.Backend))
	}
	if c.MaxNumDisplays != nil {
		args = append(args, fmt.Sprintf("max-num-displays=%d", *c.MaxNumDisplays))
	}
	if c.AudioDeviceMode != nil {
		args = append(args, fmt.Sprintf("audio-device-mode=%s", *c.AudioDeviceMode))
	}
	if len(c.Displays) != 0 {
		var displayArgs []string
		for _, display := range c.Displays {
			displayArgs = append(displayArgs, display.toArgString())
		}
		args = append(args, fmt.Sprintf("displays=[%s]", strings.Join(displayArgs, ",")))
	}
	if c.Egl != nil {
		args = append(args, fmt.Sprintf("egl=%t", *c.Egl))
	}
	if c.Gles != nil {
		args = append(args, fmt.Sprintf("gles=%t", *c.Gles))
	}
	if c.Glx != nil {
		args = append(args, fmt.Sprintf("glx=%t", *c.Glx))
	}
	if c.Surfaceless != nil {
		args = append(args, fmt.Sprintf("surfaceless=%t", *c.Surfaceless))
	}
	if c.Vulkan != nil {
		args = append(args, fmt.Sprintf("vulkan=%t", *c.Vulkan))
	}
	if c.Wsi != nil {
		args = append(args, fmt.Sprintf("wsi=%s", *c.Wsi))
	}
	if c.Udmabuf != nil {
		args = append(args, fmt.Sprintf("udmabuf=%t", *c.Udmabuf))
	}
	if c.CachePath != nil {
		args = append(args, fmt.Sprintf("cache-path=%s", *c.CachePath))
	}
	if c.CacheSize != nil {
		args = append(args, fmt.Sprintf("cache-size=%s", *c.CacheSize))
	}
	if c.PciAddress != nil {
		args = append(args, fmt.Sprintf("pci-address=%s", *c.PciAddress))
	}
	if c.PciBarSize != nil {
		args = append(args, fmt.Sprintf("pci-bar-size=%d", *c.PciBarSize))
	}
	if c.ContextTypes != nil {
		args = append(args, fmt.Sprintf("context-types=%s", strings.Join(c.ContextTypes, ":")))
	}
	if c.ExternalBlob != nil {
		args = append(args, fmt.Sprintf("external-blob=%t", *c.ExternalBlob))
	}
	if c.SystemBlob != nil {
		args = append(args, fmt.Sprintf("system-blob=%t", *c.SystemBlob))
	}
	if c.FixedBlobMapping != nil {
		args = append(args, fmt.Sprintf("fixed-blob-mapping=%t", *c.FixedBlobMapping))
	}
	if c.ImplicitRenderServer != nil {
		args = append(args, fmt.Sprintf("implicit-render-server=%t", *c.ImplicitRenderServer))
	}
	if c.RendererFeatures != nil {
		args = append(args, fmt.Sprintf("renderer-features=%s", *c.RendererFeatures))
	}
	if c.SnapshotScratchPath != nil {
		args = append(args, fmt.Sprintf("snapshot-scratch-path=%s", *c.SnapshotScratchPath))
	}
	return strings.Join(args, ",")
}

func (c *InstanceConfigExtGpuDisplay) toArgString() string {
	var args []string
	if c.ModeWindowed != nil {
		args = append(args, fmt.Sprintf("mode=windowed[%d,%d]", c.ModeWindowed.Width, c.ModeWindowed.Height))
	}
	if c.Hidden != nil {
		args = append(args, fmt.Sprintf("hidden=%t", *c.Hidden))
	}
	if c.RefreshRate != nil {
		args = append(args, fmt.Sprintf("refresh-rate=%d", *c.RefreshRate))
	}
	if c.Dpi != nil {
		args = append(args, fmt.Sprintf("dpi=[%d,%d]", c.Dpi.Horizontal, c.Dpi.Vertical))
	}
	return fmt.Sprintf("[%s]", strings.Join(args, ","))
}

func (c *InstanceConfigExtGpuRenderServer) toArgString() string {
	args := []string{
		fmt.Sprintf("path=%s", c.Path),
	}
	if c.CachePath != nil {
		args = append(args, fmt.Sprintf("cache-path=%s", *c.CachePath))
	}
	if c.CacheSize != nil {
		args = append(args, fmt.Sprintf("cache-size=%s", *c.CacheSize))
	}
	if c.FozDbListPath != nil {
		args = append(args, fmt.Sprintf("foz-db-list-path=%s", *c.FozDbListPath))
	}
	if c.PrecompiledCachePath != nil {
		args = append(args, fmt.Sprintf("precompiled-cache-path=%s", *c.PrecompiledCachePath))
	}
	if c.LdPreloadPath != nil {
		args = append(args, fmt.Sprintf("ld-preload-path=%s", *c.LdPreloadPath))
	}
	return strings.Join(args, ",")
}
