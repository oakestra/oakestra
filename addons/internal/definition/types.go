package definition

import "encoding/json"

// SourceKind says where a definition comes from.
type SourceKind string

const (
	// SourceOfficial is the curated catalog (oakestra/community-marketplace).
	SourceOfficial SourceKind = "official"
	// SourceTap is a third-party index added by an admin.
	SourceTap SourceKind = "tap"
	// SourceLocal is a definition registered directly on this marketplace.
	SourceLocal SourceKind = "local"
)

// Source identifies the catalog a definition belongs to. Name is "official" for the official
// catalog, the tap name for taps and "local" for local registrations.
type Source struct {
	Kind SourceKind `json:"kind"`
	Name string     `json:"name"`
}

// OfficialSource is the source of every definition in the official catalog.
var OfficialSource = Source{Kind: SourceOfficial, Name: string(SourceOfficial)}

// LocalSource is the source of every locally registered definition.
var LocalSource = Source{Kind: SourceLocal, Name: string(SourceLocal)}

// TapSource returns the source for the tap with the given name.
func TapSource(name string) Source {
	return Source{Kind: SourceTap, Name: name}
}

// Addon is one addon definition. The JSON field names are the ones the dashboard and existing
// catalogs already use.
type Addon struct {
	// Name is the bare name. The namespace comes from Source.
	Name        string   `json:"name"`
	Source      Source   `json:"source,omitzero"`
	Version     string   `json:"version"`
	Description string   `json:"description,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	Maintainers []string `json:"maintainers,omitempty"`
	UI          *UI      `json:"ui,omitempty"`

	Permissions Permissions `json:"permissions"`
	Services    []Service   `json:"services"`
	Volumes     []Volume    `json:"volumes"`
	Networks    []Network   `json:"networks"`
}

// UI describes the service that serves the addon's dashboard UI.
type UI struct {
	// Service is the service_name of the service that serves the UI.
	Service string `json:"service"`
	// SDK is the semver range of @oakestra/dashboard-sdk the UI was built against.
	SDK string `json:"sdk"`
}

// Service is one container of an addon. The fields map onto docker run options; see the
// package doc before adding any.
type Service struct {
	ServiceName string `json:"service_name"`
	Image       string `json:"image"`
	Command     string `json:"command,omitempty"`
	// Networks the container joins: networks declared by the addon, CoreNetwork (needs
	// permissions.core_network), "host" (needs permissions.host_network) or "none". Empty means
	// an addon-private default network, never CoreNetwork.
	Networks []string `json:"networks"`
	// Volumes in docker's short syntax: "/host:/container[:ro|rw]", "named:/container[:ro|rw]"
	// or "/container" for an anonymous volume.
	Volumes []string `json:"volumes"`
	// Ports maps the container port ("80", "80/tcp", "53/udp") to the host port ("8080" or
	// "127.0.0.1:8080").
	Ports       map[string]string `json:"ports"`
	Environment map[string]string `json:"environment"`
	Labels      map[string]string `json:"labels"`
}

// Volume is a named volume the addon creates.
type Volume struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver,omitempty"`
	DriverOpts map[string]string `json:"driver_opts,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// Network is a network the addon creates.
type Network struct {
	Name       string `json:"name"`
	Driver     string `json:"driver,omitempty"`
	EnableIPv6 bool   `json:"enable_ipv6,omitempty"`
}

// Permissions are the risky capabilities an addon gets on the host. A definition declares them,
// RequiredPermissions derives them, and CheckPolicy makes sure the first covers the second.
type Permissions struct {
	// HostPaths are absolute host paths the addon may bind mount, each optionally suffixed with
	// ":ro". An entry covers a mount of exactly that path, not of anything below it (see
	// HostPathGrant.covers for why). A ":ro" entry only covers read-only mounts.
	HostPaths []string `json:"host_paths"`
	// HostPorts are the host ports the addon may publish.
	HostPorts []int `json:"host_ports"`
	// DockerSocket allows mounting a container runtime socket (or a directory holding one),
	// which is equivalent to root on the host.
	DockerSocket bool `json:"docker_socket"`
	// HostRoot allows mounting, even read-only, a host path that is root-equivalent (system
	// configuration, kernel interfaces, runtime state such as /var/lib/docker) or a directory
	// above one. It is separate from DockerSocket: a socket mount at a root-equivalent path
	// needs both.
	HostRoot bool `json:"host_root"`
	// HostNetwork allows services to use the host network namespace.
	HostNetwork bool `json:"host_network"`
	// CoreNetwork allows services to join the "oakestra" network, where Mongo, Redis and the
	// core APIs are reachable without authentication. That is close to root on the control
	// plane, so it is a permission like the others.
	CoreNetwork bool `json:"core_network"`
}

// Normalize replaces nil slices and maps with empty ones so the JSON output has [] and {}
// instead of null, which is what the dashboard expects.
func (a *Addon) Normalize() {
	if a.Permissions.HostPaths == nil {
		a.Permissions.HostPaths = []string{}
	}
	if a.Permissions.HostPorts == nil {
		a.Permissions.HostPorts = []int{}
	}
	if a.Services == nil {
		a.Services = []Service{}
	}
	if a.Volumes == nil {
		a.Volumes = []Volume{}
	}
	if a.Networks == nil {
		a.Networks = []Network{}
	}
	for i := range a.Services {
		s := &a.Services[i]
		if s.Networks == nil {
			s.Networks = []string{}
		}
		if s.Volumes == nil {
			s.Volumes = []string{}
		}
		if s.Ports == nil {
			s.Ports = map[string]string{}
		}
		if s.Environment == nil {
			s.Environment = map[string]string{}
		}
		if s.Labels == nil {
			s.Labels = map[string]string{}
		}
	}
}

// Clone returns a normalized deep copy. Definitions hold maps and slices, and a shallow copy
// would let a caller mutate what a store holds.
func (a *Addon) Clone() Addon {
	b, err := json.Marshal(a)
	if err != nil {
		// Definitions only hold strings, numbers, bools, slices and maps of those.
		panic(err)
	}
	var out Addon
	if err := json.Unmarshal(b, &out); err != nil {
		panic(err)
	}
	out.Normalize()
	return out
}

// QualifiedName returns the user-facing name, see QualifiedName.
func (a *Addon) QualifiedName() string {
	return QualifiedName(a.Source, a.Name)
}

// ID returns the DNS-safe id, see ID.
func (a *Addon) ID() string {
	return ID(a.Source, a.Name)
}

// Service returns the service with the given name, or nil.
func (a *Addon) Service(name string) *Service {
	for i := range a.Services {
		if a.Services[i].ServiceName == name {
			return &a.Services[i]
		}
	}
	return nil
}
