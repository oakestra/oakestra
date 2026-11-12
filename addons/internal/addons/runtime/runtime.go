// Package runtime is the seam between the reconcile loop and the container engine. The
// interface is deliberately narrow: a ContainerSpec can only express what a definition may ask
// for, so there is no way to set privileged mode, capabilities or devices through it.
package runtime

import (
	"context"
	"errors"
	"maps"
)

// Labels the runtime puts on everything it creates. The "oak." prefix is reserved in
// definitions, so an addon can't forge them.
const (
	LabelAddonID  = "oak.addon.id"   // the installed record's id
	LabelAddonKey = "oak.addon.name" // the definition id, e.g. "acme--tool"
	LabelPlane    = "oak.addon.plane"
	// LabelInstance is the engine's instance id (statedir.LoadOrCreateInstanceID). Two
	// deployments of the same plane can share one docker daemon; without it each would treat
	// the other's containers as orphans.
	LabelInstance = "oak.addon.instance"
	LabelService  = "oak.service.name"
)

// networkOwnerLabels decide whether an existing network belongs to the addon. Networks are
// removed on teardown, so the record id is the right owner.
var networkOwnerLabels = []string{LabelAddonID, LabelPlane, LabelInstance}

// volumeOwnerLabels decide whether an existing volume belongs to the addon. Volumes outlive
// their record on purpose (data survives a disable and reinstall), so they are owned by the
// addon id rather than the record id.
var volumeOwnerLabels = []string{LabelAddonKey, LabelPlane, LabelInstance}

// sameOwner reports whether have carries the same non-empty value as want for every key.
// A missing label never matches, so an object without one counts as foreign.
func sameOwner(have, want map[string]string, keys []string) bool {
	for _, k := range keys {
		if want[k] == "" || have[k] != want[k] {
			return false
		}
	}
	return true
}

// ErrVolumeMismatch is returned by EnsureVolume when the addon's volume exists with a different
// driver or driver options than the definition asks for. Reusing it would keep e.g. an old
// bind of a host path the new version no longer declares.
var ErrVolumeMismatch = errors.New("exists with a different driver or driver options")

// sameVolumeConfig compares an existing volume's driver and options with a spec. Docker
// reports the default driver as "local".
func sameVolumeConfig(haveDriver string, haveOpts map[string]string, spec VolumeSpec) bool {
	want := spec.Driver
	if want == "" {
		want = "local"
	}
	if haveDriver == "" {
		haveDriver = "local"
	}
	if haveDriver != want {
		return false
	}
	if len(haveOpts) == 0 && len(spec.DriverOpts) == 0 {
		return true
	}
	return maps.Equal(haveOpts, spec.DriverOpts)
}

// Limits are the resource limits every addon container gets. Zero disables a limit.
type Limits struct {
	PidsLimit   int64
	MemoryBytes int64
	NanoCPUs    int64
}

// ErrNetworkMissing is returned by CreateAndStart when a network the spec joins doesn't exist
// and isn't one the runtime may create (the core network).
var ErrNetworkMissing = errors.New("network not found")

// ErrForeign is returned when a network or volume the reconcile loop wants to create already
// exists but belongs to something else.
var ErrForeign = errors.New("exists but is not owned by this addon")

// Container is a container as the reconcile loop sees it.
type Container struct {
	ID           string
	Name         string
	Labels       map[string]string
	State        string // docker's state: running, exited, restarting, ...
	ExitCode     int
	RestartCount int
}

// NetAttach is a network a container joins and its aliases there.
type NetAttach struct {
	Name    string
	Aliases []string
}

// Mount is a volume mount of a container.
type Mount struct {
	// Source is a host path for binds and a volume name for named volumes. Empty for
	// anonymous volumes.
	Source   string
	Bind     bool
	Target   string
	ReadOnly bool
}

// PortBinding publishes a container port on the host.
type PortBinding struct {
	ContainerPort int
	Protocol      string
	HostIP        string
	HostPort      int
}

// ContainerSpec is everything the runtime may set on a container.
type ContainerSpec struct {
	Name   string
	Image  string
	Cmd    []string
	Env    []string
	Labels map[string]string
	Mounts []Mount
	Ports  []PortBinding
	// NetworkMode is "host" or "none" when the service uses one of those. Networks is empty
	// then.
	NetworkMode string
	Networks    []NetAttach
}

// NetworkSpec is a network to create.
type NetworkSpec struct {
	Name       string
	Driver     string
	EnableIPv6 bool
	Labels     map[string]string
	// Shared networks (the UI network) may already exist without our labels, e.g. created by
	// compose, and are never removed by us.
	Shared bool
}

// VolumeSpec is a volume to create.
type VolumeSpec struct {
	Name       string
	Driver     string
	DriverOpts map[string]string
	Labels     map[string]string
}

// Runtime is a container engine.
type Runtime interface {
	// PullImage pulls an image reference anonymously.
	PullImage(ctx context.Context, ref string) error
	// ImageLabels returns the labels in a pulled image's config. They end up on every
	// container made from the image, so they have to pass the reserved label policy too.
	ImageLabels(ctx context.Context, ref string) (map[string]string, error)
	// InspectContainer returns nil, nil when no container has that name.
	InspectContainer(ctx context.Context, name string) (*Container, error)
	// ListContainers returns all containers, running or not, carrying every given label.
	ListContainers(ctx context.Context, labels map[string]string) ([]Container, error)
	// CreateAndStart creates the container, joins its networks and starts it. On failure it
	// removes what it created.
	CreateAndStart(ctx context.Context, spec ContainerSpec) error
	// RemoveContainer stops and removes a container by id.
	RemoveContainer(ctx context.Context, id string) error
	ContainerLogs(ctx context.Context, id string, tail int) (string, error)

	// EnsureNetwork creates the network if it doesn't exist. An existing network is accepted
	// if it carries the same record id, plane and instance labels, or if spec.Shared is set.
	// Otherwise it returns ErrForeign.
	EnsureNetwork(ctx context.Context, spec NetworkSpec) error
	// ListNetworks returns the names of networks carrying every given label.
	ListNetworks(ctx context.Context, labels map[string]string) ([]string, error)
	RemoveNetwork(ctx context.Context, name string) error
	// EnsureVolume creates the volume if it doesn't exist. An existing volume is accepted if
	// it carries the same addon id (oak.addon.name), plane and instance labels, so a reinstall
	// adopts its data; otherwise it returns ErrForeign. An adopted volume must also have the
	// spec's driver and driver options, or EnsureVolume returns ErrVolumeMismatch.
	EnsureVolume(ctx context.Context, spec VolumeSpec) error
}
