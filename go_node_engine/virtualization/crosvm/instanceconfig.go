package crosvm

// Missing:
// - android_display_service
// - pci
// - pci_hotplug_slots
// - pmem
// - serial
// - simple_media_device
// - smbios
// - socket
// - v4l2_proxy
// - vfio
// - vhost_scmi
// - vsock

// Experimental
// - gdb
// - pmem_ext2
// - scsi_block
// - video_decoder
// - video_encoder

type InstanceConfig struct {
	AcAdapter             *bool                             `json:"ac-adapter,omitempty"`
	Battery               *InstanceConfigBattery            `json:"battery,omitempty"`
	Block                 []InstanceConfigBlock             `json:"block,omitempty"`
	BreakLinuxPciConfigIo *bool                             `json:"break-linux-pci-config-io,omitempty"`
	BusLockRatelimit      *uint64                           `json:"bus-lock-ratelimit,omitempty"`
	Cfg                   []string                          `json:"cfg,omitempty"`
	CoreScheduling        *bool                             `json:"core-scheduling"`
	Cpus                  *InstanceConfigCpus               `json:"cpus,omitempty"`
	DeviceTreeOverlay     []InstanceConfigDeviceTreeOverlay `json:"device-tree-overlay,omitempty"`
	Hypervisor            *InstanceConfigHypervisor         `json:"hypervisor,omitempty"`
	Initrd                *string                           `json:"initrd,omitempty"`
	Input                 []InstanceConfigInput             `json:"input,omitempty"`
	Irqchip               *string                           `json:"irqchip,omitempty"`
	Kernel                *string                           `json:"kernel,omitempty"`
	MediaDecoder          []InstanceConfigMediaDecoder      `json:"media-decoder,omitempty"`
	Mem                   *InstanceConfigMem                `json:"mem,omitempty"`
	Name                  *string                           `json:"name,omitempty"`
	Net                   []InstanceConfigNet               `json:"net,omitempty"`
	Params                []string                          `json:"params,omitempty"`
	VhostUser             []InstanceConfigVhostUser         `json:"vhost-user,omitempty"`
}

type InstanceConfigBattery struct {
	Type string `json:"type"`
}

type InstanceConfigBlock struct {
	Path          string  `json:"path"`
	ReadOnly      *bool   `json:"ro,omitempty"`
	Root          *bool   `json:"root,omitempty"`
	Sparse        *bool   `json:"sparse,omitempty"`
	Direct        *bool   `json:"direct,omitempty"`
	Lock          *bool   `json:"lock,omitempty"`
	BlockSize     *uint32 `json:"block-size,omitempty"`
	ID            *string `json:"id,omitempty"`
	AsyncExecutor *string `json:"async-executor,omitempty"`
	PackedQueue   *bool   `json:"packed-queue,omitempty"`
	Bootindex     *int    `json:"bootindex,omitempty"`
	PciAddress    *string `json:"pci-address,omitempty"`
}

type InstanceConfigCpus struct {
	NumCores    *int                       `json:"num-cores,omitempty"`
	Clusters    []int                      `json:"clusters,omitempty"`
	CoreTypes   *InstanceConfigCpuCoreType `json:"core-types,omitempty"`
	BootCPU     *int                       `json:"boot-cpu,omitempty"`
	FreqDomains []int                      `json:"freq-domains,omitempty"`
	Sve         *InstanceConfigSveConfig   `json:"sve,omitempty"`
}

type InstanceConfigCpuCoreType struct {
	Atom []int `json:"atom"`
	Core []int `json:"core"`
}

type InstanceConfigSveConfig struct {
	Enable *bool `json:"enable,omitempty"`
	Auto   *bool `json:"auto,omitempty"`
}

type InstanceConfigDeviceTreeOverlay struct {
	Path       string `json:"path"`
	FilterDevs *bool  `json:"filter,omitempty"`
}

type InstanceConfigHypervisor struct {
	Kvm       *InstanceConfigKvm       `json:"kvm,omitempty"`
	Geniezone *InstanceConfigGeniezone `json:"geniezone,omitempty"`
	Gunyah    *InstanceConfigGunyah    `json:"gunyah,omitempty"`
}

type InstanceConfigKvm struct {
	Device *string `json:"device,omitempty"`
}

type InstanceConfigGeniezone struct {
	Device *string `json:"device,omitempty"`
}

type InstanceConfigGunyah struct {
	Device             *string `json:"device,omitempty"`
	QcomTrustedVmID    *uint16 `json:"qcom-trusted-vm-id,omitempty"`
	QcomTrustedVmPasID *uint32 `json:"qcom-trusted-vm-pas-id,omitempty"`
}

type InstanceConfigInput struct {
	Evdev              *InstanceConfigInputEvdev              `json:"evdev,omitempty"`
	Keyboard           *InstanceConfigInputKeyboard           `json:"keyboard,omitempty"`
	Mouse              *InstanceConfigInputMouse              `json:"mouse,omitempty"`
	MultiTouch         *InstanceConfigInputMultiTouch         `json:"multi-touch,omitempty"`
	Rotary             *InstanceConfigInputRotary             `json:"rotary,omitempty"`
	SingleTouch        *InstanceConfigInputSingleTouch        `json:"single-touch,omitempty"`
	Switches           *InstanceConfigInputSwitches           `json:"switches,omitempty"`
	Trackpad           *InstanceConfigInputTrackpad           `json:"trackpad,omitempty"`
	MultiTouchTrackpad *InstanceConfigInputMultiTouchTrackpad `json:"multi-touch-trackpad,omitempty"`
	Custom             *InstanceConfigInputCustom             `json:"custom,omitempty"`
}

type InstanceConfigInputEvdev struct {
	Path string `json:"path"`
}

type InstanceConfigInputKeyboard struct {
	Path string `json:"path"`
}

type InstanceConfigInputMouse struct {
	Path string `json:"path"`
}

type InstanceConfigInputMultiTouch struct {
	Path   string  `json:"path"`
	Width  *uint32 `json:"width,omitempty"`
	Height *uint32 `json:"height,omitempty"`
	Name   *string `json:"name,omitempty"`
}

type InstanceConfigInputRotary struct {
	Path string `json:"path"`
}

type InstanceConfigInputSingleTouch struct {
	Path   string  `json:"path"`
	Width  *uint32 `json:"width,omitempty"`
	Height *uint32 `json:"height,omitempty"`
	Name   *string `json:"name,omitempty"`
}

type InstanceConfigInputSwitches struct {
	Path string `json:"path"`
}

type InstanceConfigInputTrackpad struct {
	Path   string  `json:"path"`
	Width  *uint32 `json:"width,omitempty"`
	Height *uint32 `json:"height,omitempty"`
	Name   *string `json:"name,omitempty"`
}

type InstanceConfigInputMultiTouchTrackpad struct {
	Path   string  `json:"path"`
	Width  *uint32 `json:"width,omitempty"`
	Height *uint32 `json:"height,omitempty"`
	Name   *string `json:"name,omitempty"`
}

type InstanceConfigInputCustom struct {
	Path       string `json:"path"`
	ConfigPath string `json:"config-path"`
}

type InstanceConfigMediaDecoder struct {
	Backend string `json:"backend"`
}

type InstanceConfigMem struct {
	Size int `json:"size"`
}

type InstanceConfigNet struct {
	TapName     InstanceConfigNetTapName   `json:"tap-name,omitempty"`
	TapFd       InstanceConfigNetTapFd     `json:"tap-fd,omitempty"`
	RawConfig   InstanceConfigNetRawConfig `json:"raw-config,omitempty"`
	VqPairs     *uint16                    `json:"vq-pairs,omitempty"`
	VhostNet    *InstanceConfigVhostNet    `json:"vhost-net,omitempty"`
	PackedQueue *bool                      `json:"packed-queue,omitempty"`
	PciAddress  *string                    `json:"pci-address,omitempty"`
}

type InstanceConfigNetTapName struct {
	TapName string  `json:"tap-name"`
	Mac     *string `json:"mac,omitempty"`
}

type InstanceConfigNetTapFd struct {
	TapFd int     `json:"tap-fd"`
	Mac   *string `json:"mac,omitempty"`
}

type InstanceConfigNetRawConfig struct {
	HostIP  string `json:"host-ip"`
	Netmask string `json:"netmask"`
	Mac     string `json:"mac"`
}

type InstanceConfigVhostNet struct {
	Device *string `json:"device,omitempty"`
}

type InstanceConfigVhostUser struct {
	Type         string  `json:"type"`
	Socket       string  `json:"socket"`
	MaxQueueSize *uint16 `json:"max-queue-size,omitempty"`
	PciAddress   *string `json:"pci-address,omitempty"`
}
