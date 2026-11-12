package definition

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// MountKind says what the source of a service volume is.
type MountKind int

const (
	// MountAnonymous is "/container": docker creates an anonymous volume.
	MountAnonymous MountKind = iota
	// MountNamed is "name:/container": a named volume.
	MountNamed
	// MountHost is "/host:/container": a bind mount from the host.
	MountHost
)

// Mount is a parsed entry of Service.Volumes.
type Mount struct {
	Kind MountKind
	// Source is the volume name for MountNamed and the path exactly as written for MountHost.
	Source string
	// HostPath is path.Clean(Source) for MountHost. Policy decisions use it, so
	// "/var//run/./docker.sock" is treated like "/var/run/docker.sock".
	HostPath string
	Target   string
	ReadOnly bool
}

// dockerObjectName is docker's rule for volume and network names.
var dockerObjectName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// ParseMount parses docker's short volume syntax. It is stricter than docker on purpose: the
// only modes are "ro" and "rw" (no SELinux relabeling, propagation or nocopy), and relative or
// "~" host paths are rejected because docker would not resolve them the way an author expects.
func ParseMount(spec string) (Mount, error) {
	parts := strings.Split(spec, ":")
	var m Mount
	switch len(parts) {
	case 1:
		m.Kind = MountAnonymous
		m.Target = parts[0]
	case 2, 3:
		m.Source, m.Target = parts[0], parts[1]
		if len(parts) == 3 {
			switch parts[2] {
			case "ro":
				m.ReadOnly = true
			case "rw":
			default:
				return Mount{}, fmt.Errorf("mode %q is not allowed, use \"ro\" or \"rw\"", parts[2])
			}
		}
		switch {
		case m.Source == "":
			return Mount{}, fmt.Errorf("%q has an empty source", spec)
		case strings.HasPrefix(m.Source, "/"):
			m.Kind = MountHost
			m.HostPath = path.Clean(m.Source)
		case strings.HasPrefix(m.Source, ".") || strings.HasPrefix(m.Source, "~"):
			return Mount{}, fmt.Errorf("host path %q must be absolute", m.Source)
		case dockerObjectName.MatchString(m.Source):
			m.Kind = MountNamed
		default:
			return Mount{}, fmt.Errorf("%q is neither an absolute host path nor a valid volume name", m.Source)
		}
	default:
		return Mount{}, fmt.Errorf("%q is not a valid volume, use \"source:/target[:ro|rw]\"", spec)
	}
	if !strings.HasPrefix(m.Target, "/") {
		return Mount{}, fmt.Errorf("target %q must be an absolute path", m.Target)
	}
	return m, nil
}

// PortBinding is a parsed entry of Service.Ports.
type PortBinding struct {
	ContainerPort int
	Protocol      string
	// HostIP is empty when the port binds on all interfaces.
	HostIP   string
	HostPort int
}

var containerPortPattern = regexp.MustCompile(`^([0-9]{1,5})(?:/(tcp|udp|sctp))?$`)

// ParsePort parses one Service.Ports entry. Host port ranges and random host ports aren't
// supported, since the host port has to be declared up front.
func ParsePort(containerPort, host string) (PortBinding, error) {
	m := containerPortPattern.FindStringSubmatch(containerPort)
	if m == nil {
		return PortBinding{}, fmt.Errorf("container port %q must look like \"80\" or \"80/tcp\"", containerPort)
	}
	cp, err := parsePortNumber(m[1])
	if err != nil {
		return PortBinding{}, fmt.Errorf("container port: %w", err)
	}
	b := PortBinding{ContainerPort: cp, Protocol: m[2]}
	if b.Protocol == "" {
		b.Protocol = "tcp"
	}

	hostPort := host
	if i := strings.LastIndex(host, ":"); i >= 0 {
		b.HostIP, hostPort = host[:i], host[i+1:]
		if !hostIPPattern.MatchString(b.HostIP) {
			return PortBinding{}, fmt.Errorf("host address %q must be an IPv4 address", b.HostIP)
		}
	}
	hp, err := parsePortNumber(hostPort)
	if err != nil {
		return PortBinding{}, fmt.Errorf("host port: %w", err)
	}
	b.HostPort = hp
	return b, nil
}

var hostIPPattern = regexp.MustCompile(`^((25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])\.){3}(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])$`)

func parsePortNumber(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != s {
		return 0, fmt.Errorf("%q is not a port between 1 and 65535", s)
	}
	return n, nil
}

// HostPathGrant is a parsed entry of Permissions.HostPaths.
type HostPathGrant struct {
	Path     string
	ReadOnly bool
}

// ParseHostPathGrant parses "/path" or "/path:ro". The path must be absolute and already in
// clean form, so what the admin confirms is exactly what gets compared.
func ParseHostPathGrant(s string) (HostPathGrant, error) {
	p, ro := strings.CutSuffix(s, ":ro")
	if strings.Contains(p, ":") {
		return HostPathGrant{}, fmt.Errorf("%q must be \"/path\" or \"/path:ro\"", s)
	}
	if err := checkCleanAbs(p); err != nil {
		return HostPathGrant{}, err
	}
	return HostPathGrant{Path: p, ReadOnly: ro}, nil
}

func (g HostPathGrant) String() string {
	if g.ReadOnly {
		return g.Path + ":ro"
	}
	return g.Path
}

// covers reports whether the grant allows mounting p with the given access. Grants match the
// exact mount source, never a subtree: docker follows symlinks in bind sources, so an addon
// with a read-write grant for /opt/addon could plant /opt/addon/s -> / and then mount
// /opt/addon/s. Subtree coverage would let that mount through without the admin ever seeing a
// new path.
func (g HostPathGrant) covers(p string, readOnly bool) bool {
	if g.ReadOnly && !readOnly {
		return false
	}
	return p == g.Path
}

func checkCleanAbs(p string) error {
	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("host path %q must be absolute", p)
	}
	if path.Clean(p) != p {
		return fmt.Errorf("host path %q is not normalized, use %q", p, path.Clean(p))
	}
	return nil
}

// isWithin reports whether p is dir or below it. Both must be clean absolute paths.
func isWithin(p, dir string) bool {
	return dir == "/" || p == dir || strings.HasPrefix(p, dir+"/")
}
