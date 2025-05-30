package instance

import (
	"go_node_engine/model"
	"go_node_engine/util/ptr"
	"go_node_engine/virtualization/internal/crosvm/internal/image"
	"path"
)

const configFileName = "config.json"
const socketFileName = "instance.sock"

// InstanceConfig represents the parameters of the "crosvm run" command and are passed to it as a JSON file via the "--cfg" argument.
//
// Many parameters of the "crosvm run" command are only accepted as direct CLI arguments and not supported by the "--cfg" method.
// These parameters are not included in the fields of this struct.
// However, some of these missing parameters are useful for Oakestra and therefore implemented via the InstanceConfigExt struct,
// which is manually turned into direct CLI arguments for "crosvm run".
//
// Additionally, some options that *are* supported by the "--cfg" method are still excluded in this struct:
// - gdb: experimental
// - pmem_ext2: experimental
// - scsi_block: experimental
// - video_decoder: experimental
// - video_encoder: experimental
// - android_display_service: android only
type InstanceConfig struct {
	AcAdapter             *bool                             `json:"ac-adapter,omitempty"`
	Battery               *InstanceConfigBattery            `json:"battery,omitempty"`
	Block                 []InstanceConfigBlock             `json:"block,omitempty"`
	BreakLinuxPciConfigIo *bool                             `json:"break-linux-pci-config-io,omitempty"`
	BusLockRatelimit      *uint64                           `json:"bus-lock-ratelimit,omitempty"`
	Cfg                   []string                          `json:"cfg,omitempty"`
	CoreScheduling        *bool                             `json:"core-scheduling,omitempty"`
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
	Pci                   *InstanceConfigPci                `json:"pci,omitempty"`
	PciHotplugSlots       *uint8                            `json:"pci-hotplug-slots,omitempty"`
	Pmem                  []InstanceConfigPmem              `json:"pmem,omitempty"`
	Serial                []InstanceConfigSerial            `json:"serial,omitempty"`
	SimpleMediaDevice     *bool                             `json:"simple-media-device,omitempty"`
	Smbios                *InstanceConfigSmbios             `json:"smbios,omitempty"`
	Socket                *string                           `json:"socket,omitempty"`
	V4l2Proxy             []string                          `json:"v4l2-proxy,omitempty"`
	Vfio                  []InstanceConfigVfio              `json:"vfio,omitempty"`
	VhostScmi             *bool                             `json:"vhost-scmi,omitempty"`
	VhostUser             []InstanceConfigVhostUser         `json:"vhost-user,omitempty"`
	Vsock                 *InstanceConfigVsock              `json:"vsock,omitempty"`
}

type InstanceConfigBattery struct {
	Type string `json:"type"`
}

type InstanceConfigBlock struct {
	Path          string  `json:"path"`
	Ro            *bool   `json:"ro,omitempty"`
	Root          *bool   `json:"root,omitempty"`
	Sparse        *bool   `json:"sparse,omitempty"`
	Direct        *bool   `json:"direct,omitempty"`
	Lock          *bool   `json:"lock,omitempty"`
	BlockSize     *uint32 `json:"block-size,omitempty"`
	ID            *string `json:"id,omitempty"`
	AsyncExecutor *string `json:"async-executor,omitempty"`
	PackedQueue   *bool   `json:"packed-queue,omitempty"`
	Bootindex     *uint   `json:"bootindex,omitempty"`
	PciAddress    *string `json:"pci-address,omitempty"`
}

type InstanceConfigCpus struct {
	NumCores    *uint                      `json:"num-cores,omitempty"`
	Clusters    []uint                     `json:"clusters,omitempty"`
	CoreTypes   *InstanceConfigCpuCoreType `json:"core-types,omitempty"`
	BootCPU     *uint                      `json:"boot-cpu,omitempty"`
	FreqDomains []uint                     `json:"freq-domains,omitempty"`
	Sve         *InstanceConfigSveConfig   `json:"sve,omitempty"`
}

type InstanceConfigCpuCoreType struct {
	Atom []uint `json:"atom"`
	Core []uint `json:"core"`
}

type InstanceConfigSveConfig struct {
	Enable *bool `json:"enable,omitempty"`
	Auto   *bool `json:"auto,omitempty"`
}

type InstanceConfigDeviceTreeOverlay struct {
	Path   string `json:"path"`
	Filter *bool  `json:"filter,omitempty"`
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
	Size *uint64 `json:"size,omitempty"`
}

type InstanceConfigNet struct {
	TapName     *string                 `json:"tap-name,omitempty"`
	TapFd       *int                    `json:"tap-fd,omitempty"`
	HostIP      *string                 `json:"host-ip,omitempty"`
	Netmask     *string                 `json:"netmask,omitempty"`
	Mac         *string                 `json:"mac,omitempty"`
	VqPairs     *uint16                 `json:"vq-pairs,omitempty"`
	VhostNet    *InstanceConfigVhostNet `json:"vhost-net,omitempty"`
	PackedQueue *bool                   `json:"packed-queue,omitempty"`
	PciAddress  *string                 `json:"pci-address,omitempty"`
}

type InstanceConfigVhostNet struct {
	Device *string `json:"device,omitempty"`
}

type InstanceConfigPci struct {
	Cam  *InstanceConfigPciMemoryRegion `json:"cam,omitempty"`
	Ecam *InstanceConfigPciMemoryRegion `json:"ecam,omitempty"`
	Mem  *InstanceConfigPciMemoryRegion `json:"mem,omitempty"`
}

type InstanceConfigPciMemoryRegion struct {
	Start uint64  `json:"start"`
	Size  *uint64 `json:"size,omitempty"`
}

type InstanceConfigPmem struct {
	Path           string  `json:"path"`
	Ro             *bool   `json:"ro,omitempty"`
	Root           *bool   `json:"root,omitempty"`
	VmaSize        *uint64 `json:"vma-size,omitempty"`
	SwapIntervalMs *uint64 `json:"swap-interval-ms,omitempty"`
}

type InstanceConfigSerial struct {
	Type            string  `json:"type"`
	Hardware        *string `json:"hardware,omitempty"`
	Name            *string `json:"name,omitempty"`
	Path            *string `json:"path,omitempty"`
	Input           *string `json:"input,omitempty"`
	InputUnixStream *bool   `json:"input-unix-stream,omitempty"`
	Num             *uint8  `json:"num,omitempty"`
	Console         *bool   `json:"console,omitempty"`
	Earlycon        *bool   `json:"earlycon,omitempty"`
	Stdin           *bool   `json:"stdin,omitempty"`
	OutTimestamp    *bool   `json:"out-timestamp,omitempty"`
	DebugconPort    *uint16 `json:"debugcon-port,omitempty"`
	PciAddress      *string `json:"pci-address,omitempty"`
}

type InstanceConfigSmbios struct {
	BiosVendor   *string  `json:"bios-vendor,omitempty"`
	BiosVersion  *string  `json:"bios-version,omitempty"`
	Manufacturer *string  `json:"manufacturer,omitempty"`
	ProductName  *string  `json:"product-name,omitempty"`
	SerialNumber *string  `json:"serial-number,omitempty"`
	Uuid         *string  `json:"uuid,omitempty"`
	OemStrings   []string `json:"oem-strings,omitempty"`
}

type InstanceConfigVfio struct {
	Path         string  `json:"path"`
	Iommu        *string `json:"iommu,omitempty"`
	GuestAddress *string `json:"guest-address,omitempty"`
	DtSymbol     *string `json:"dt-symbol,omitempty"`
}

type InstanceConfigVhostUser struct {
	Type         string  `json:"type"`
	Socket       string  `json:"socket"`
	MaxQueueSize *uint16 `json:"max-queue-size,omitempty"`
	PciAddress   *string `json:"pci-address,omitempty"`
}

type InstanceConfigVsock struct {
	Cid    uint64  `json:"cid"`
	Device *string `json:"device,omitempty"`
}

func NewInstanceConfig(
	service *model.Service,
	img *image.Image,
	netConf *networkConfig,
	runtimeDirPath string,
	stateDirPath string,
) (*InstanceConfig, error) {
	var net []InstanceConfigNet
	if netConf != nil {
		net = append(net, InstanceConfigNet{
			TapName:  ptr.Ptr("tap0"),
			Mac:      ptr.Ptr(netConf.Mac),
			VhostNet: &InstanceConfigVhostNet{},
		})
	}

	var initrd *string
	if img.HasInitrd {
		initrd = ptr.Ptr(path.Join(stateDirPath, image.InitrdFileName))
	}

	return &InstanceConfig{
		Block: []InstanceConfigBlock{
			{
				Path:   path.Join(stateDirPath, image.RootfsFileName),
				Root:   ptr.Ptr(true),
				Sparse: ptr.Ptr(true),
			},
			{
				Path: path.Join(stateDirPath, cloudInitFileName),
			},
		},
		Cpus: &InstanceConfigCpus{
			NumCores: ptr.Ptr(uint(service.Vcpus)),
		},
		Initrd: initrd,
		Kernel: ptr.Ptr(path.Join(stateDirPath, image.KernelFileName)),
		Mem: &InstanceConfigMem{
			Size: ptr.Ptr(uint64(service.Memory)),
		},
		Net:    net,
		Socket: ptr.Ptr(path.Join(runtimeDirPath, socketFileName)),
	}, nil
}
