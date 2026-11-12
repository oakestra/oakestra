package definition

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"
)

// ViolationKind separates capabilities that merely need declaring from ones that are never
// allowed.
type ViolationKind string

const (
	// Undeclared means the definition needs a permission it doesn't declare.
	Undeclared ViolationKind = "undeclared"
	// Forbidden means the definition uses something no permission can grant.
	Forbidden ViolationKind = "forbidden"
)

// Violation is one policy failure.
type Violation struct {
	Path    string        `json:"path"`
	Kind    ViolationKind `json:"kind"`
	Message string        `json:"message"`
}

func (v Violation) Error() string {
	return fmt.Sprintf("%s: %s (%s)", v.Path, v.Message, v.Kind)
}

// CoreNetwork is the docker network of the core services. Joining it needs
// Permissions.CoreNetwork.
const CoreNetwork = "oakestra"

// UINetwork is the network the runtime attaches an addon's UI service to, under the UIAlias.
// The dashboard joins it too, so UIs are reachable without putting addons on CoreNetwork.
// Definitions can't list or declare it; only the runtime attaches it.
const UINetwork = "oakestra_addon_ui"

// runtimeSockets are host paths that hand out control over the container runtime. Mounting one,
// or any directory above one, needs docker_socket. /var/run is a symlink to /run on most
// distributions, so both spellings are listed.
var runtimeSockets = []string{
	"/var/run/docker.sock",
	"/run/docker.sock",
	"/var/run/containerd/containerd.sock",
	"/run/containerd/containerd.sock",
	"/var/run/crio/crio.sock",
	"/run/crio/crio.sock",
	"/var/run/podman/podman.sock",
	"/run/podman/podman.sock",
	// dockerd's state directory holds the containerd and plugin sockets it talks to.
	"/run/docker",
	"/var/run/docker",
	// Rootless runtimes put their socket in $XDG_RUNTIME_DIR, i.e. /run/user/<uid>.
	"/run/user",
	"/var/run/user",
}

// runtimeSocketNames catch sockets in places we can't enumerate, e.g. Docker Desktop's
// ~/.docker/run/docker.sock.
var runtimeSocketNames = map[string]bool{
	"docker.sock":     true,
	"containerd.sock": true,
	"crio.sock":       true,
	"podman.sock":     true,
}

// reservedLabelPrefixes are label namespaces the runtime and docker compose use for bookkeeping.
// An addon setting them could make the runtime treat its containers as another addon's or a
// core service's.
var reservedLabelPrefixes = []string{
	"oak.",
	"oakestra.",
	"org.oakestra.",
	"com.docker.compose.",
}

// rootEquivalentPaths can't be mounted at all, not even read-only, without host_root, and
// neither can any directory above them. Reading /etc/shadow or /root/.ssh, writing to
// /proc/sysrq-trigger or /sys, or writing into /usr/bin or another container's layers under
// /var/lib/docker all amount to root on the host.
var rootEquivalentPaths = []string{
	"/",
	"/etc",
	"/root",
	"/boot",
	"/proc",
	"/sys",
	"/dev",
	"/usr",
	"/bin",
	"/sbin",
	"/lib",
	"/lib64",
	"/home",
	"/var/lib/docker",
	"/var/lib/containerd",
	"/var/lib/kubelet",
	"/run/containerd",
	// /var/run is a symlink to /run on most distributions.
	"/var/run/containerd",
}

// rootEquivalentTrees are root-equivalent paths whose subdirectories are too. The others in
// rootEquivalentPaths are not listed here on purpose: read-only mounts like
// /usr/share/zoneinfo, /lib/modules or a single user's home directory are common and harmless
// enough to only need a host_paths entry.
var rootEquivalentTrees = []string{
	"/etc",
	"/root",
	"/boot",
	"/proc",
	"/sys",
	"/dev",
	"/var/lib/docker",
	"/var/lib/containerd",
	// Container bundles (rootfs, config) live below these, and pod secrets below kubelet's.
	"/var/lib/kubelet",
	"/run/containerd",
	"/var/run/containerd",
}

func isRootEquivalent(hostPath string) bool {
	for _, p := range rootEquivalentPaths {
		if isWithin(p, hostPath) {
			return true
		}
	}
	for _, t := range rootEquivalentTrees {
		if isWithin(hostPath, t) {
			return true
		}
	}
	return false
}

// IsReservedLabel reports whether a label key is in a namespace the runtime uses for
// bookkeeping. The runtime must also apply it to the labels baked into an image, since those
// end up on the container without ever passing through the definition check.
func IsReservedLabel(key string) bool {
	k := strings.ToLower(key)
	for _, prefix := range reservedLabelPrefixes {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

func touchesRuntimeSocket(hostPath string) bool {
	if runtimeSocketNames[path.Base(hostPath)] {
		return true
	}
	for _, s := range runtimeSockets {
		if isWithin(s, hostPath) || isWithin(hostPath, s) {
			return true
		}
	}
	return false
}

// hostPathNeed is a single host path the definition would mount.
type hostPathNeed struct {
	field    string
	path     string
	readOnly bool
}

// analysis is everything the policy derives from a definition.
type analysis struct {
	hostPaths    []hostPathNeed
	hostPorts    map[int][]string // port -> fields that publish it
	dockerSocket []string         // fields that need docker_socket
	hostRoot     []string         // fields that need host_root
	hostNetwork  []string         // fields that need host_network
	coreNetwork  []string         // fields that need core_network
	forbidden    []Violation
}

func analyze(a *Addon) analysis {
	an := analysis{hostPorts: map[int][]string{}}
	forbid := func(field, format string, args ...any) {
		an.forbidden = append(an.forbidden, Violation{Path: field, Kind: Forbidden, Message: fmt.Sprintf(format, args...)})
	}
	needPath := func(field, p string, readOnly bool) {
		an.hostPaths = append(an.hostPaths, hostPathNeed{field: field, path: p, readOnly: readOnly})
		if touchesRuntimeSocket(p) {
			an.dockerSocket = append(an.dockerSocket, field)
		}
		if isRootEquivalent(p) {
			an.hostRoot = append(an.hostRoot, field)
		}
	}

	for i, s := range a.Services {
		sp := idx("services", i)
		for j, spec := range s.Volumes {
			m, err := ParseMount(spec)
			if err != nil || m.Kind != MountHost {
				// Syntax errors are Validate's job. Named and anonymous volumes need nothing.
				continue
			}
			needPath(idx(sp+".volumes", j), m.HostPath, m.ReadOnly)
		}
		for _, cp := range sortedKeys(s.Ports) {
			b, err := ParsePort(cp, s.Ports[cp])
			if err != nil {
				continue
			}
			an.hostPorts[b.HostPort] = append(an.hostPorts[b.HostPort], key(sp+".ports", cp))
		}
		for j, n := range s.Networks {
			np := idx(sp+".networks", j)
			switch {
			case n == "host":
				an.hostNetwork = append(an.hostNetwork, np)
			case n == CoreNetwork:
				an.coreNetwork = append(an.coreNetwork, np)
			case strings.HasPrefix(n, "container:"):
				forbid(np, "joining another container's network namespace is not allowed")
			}
		}
		for _, k := range sortedKeys(s.Labels) {
			if IsReservedLabel(k) {
				forbid(key(sp+".labels", k), "label %q is in a namespace reserved for the runtime (%s)", k, strings.Join(reservedLabelPrefixes, ", "))
			}
		}
	}

	for i, v := range a.Volumes {
		analyzeVolume(idx("volumes", i), v, forbid, needPath)
	}

	// Docker resolves symlinks in a bind source on the host, at container start. If any service
	// can write to a bind source, it can replace a directory below it with a symlink to / or the
	// docker socket, and a second mount of that lower path (another service, or this one after a
	// restart) would then mount whatever the symlink points at. Mounting an ancestor is fine:
	// symlinks below a mount point are resolved inside the container's mount namespace.
	for _, n := range an.hostPaths {
		for _, w := range an.hostPaths {
			if !w.readOnly && n.path != w.path && isWithin(n.path, w.path) {
				forbid(n.field, "host path %q is below %q, which %s mounts read-write; a symlink planted there would redirect this mount, mount %q itself instead", n.path, w.path, w.field, w.path)
				break
			}
		}
	}

	for i, n := range a.Networks {
		if n.Driver != "" && n.Driver != "bridge" {
			forbid(idx("networks", i)+".driver", "network driver %q is not allowed, only \"bridge\"", n.Driver)
		}
	}
	return an
}

// analyzeVolume handles top-level volumes. The local driver's driver_opts are passed to
// mount(2), so {type: none, o: bind, device: /etc} is a host bind mount in disguise. Anything we
// can't classify with certainty is forbidden rather than guessed at.
func analyzeVolume(vp string, v Volume, forbid func(string, string, ...any), needPath func(string, string, bool)) {
	if v.Driver != "" && v.Driver != "local" {
		forbid(vp+".driver", "volume driver %q is not allowed, only \"local\"", v.Driver)
		return
	}
	if len(v.DriverOpts) == 0 {
		return
	}
	op := vp + ".driver_opts"
	for _, k := range sortedKeys(v.DriverOpts) {
		if k != "type" && k != "o" && k != "device" {
			forbid(key(op, k), "driver option %q is not allowed", k)
			return
		}
	}

	typ, device := v.DriverOpts["type"], v.DriverOpts["device"]
	opts := map[string]bool{}
	for _, o := range strings.Split(v.DriverOpts["o"], ",") {
		if o = strings.TrimSpace(o); o != "" {
			opts[strings.SplitN(o, "=", 2)[0]] = true
		}
	}

	// The bind flag turns any mount into a bind of device, whatever type says.
	if opts["bind"] || opts["rbind"] {
		if typ != "" && typ != "none" {
			forbid(key(op, "type"), "bind volumes must use type \"none\"")
			return
		}
		if err := checkCleanAbs(device); err != nil {
			forbid(key(op, "device"), "%v", err)
			return
		}
		for o := range opts {
			if o != "bind" && o != "rbind" && o != "ro" && o != "rw" {
				forbid(key(op, "o"), "mount option %q is not allowed on bind volumes", o)
				return
			}
		}
		needPath(key(op, "device"), device, opts["ro"] && !opts["rw"])
		return
	}

	if typ != "tmpfs" {
		forbid(key(op, "type"), "only tmpfs and bind volumes are allowed, got type %q", typ)
		return
	}
	if device != "" && device != "tmpfs" {
		forbid(key(op, "device"), "tmpfs volumes must use device \"tmpfs\"")
		return
	}
	for o := range opts {
		switch o {
		case "size", "mode", "uid", "gid", "nr_inodes", "noexec", "nosuid", "nodev", "ro", "rw":
		default:
			forbid(key(op, "o"), "mount option %q is not allowed on tmpfs volumes", o)
			return
		}
	}
}

// RequiredPermissions derives the permissions the definition needs to run. It ignores
// syntactically invalid entries, so run Validate as well. The result is in canonical form.
func RequiredPermissions(a *Addon) Permissions {
	an := analyze(a)
	access := map[string]bool{} // path -> read-only
	for _, n := range an.hostPaths {
		ro, seen := access[n.path]
		access[n.path] = n.readOnly && (!seen || ro)
	}
	p := Permissions{
		HostPaths:    []string{},
		HostPorts:    []int{},
		DockerSocket: len(an.dockerSocket) > 0,
		HostRoot:     len(an.hostRoot) > 0,
		HostNetwork:  len(an.hostNetwork) > 0,
		CoreNetwork:  len(an.coreNetwork) > 0,
	}
	for hp, ro := range access {
		p.HostPaths = append(p.HostPaths, HostPathGrant{Path: hp, ReadOnly: ro}.String())
	}
	for port := range an.hostPorts {
		p.HostPorts = append(p.HostPorts, port)
	}
	return p.Canonical()
}

// CheckPolicy returns every capability the definition uses without declaring it, plus every
// capability that can't be granted at all. Malformed permission entries are reported by
// Validate and treated as absent here.
func CheckPolicy(a *Addon) []Violation {
	an := analyze(a)
	out := an.forbidden
	undeclared := func(field, format string, args ...any) {
		out = append(out, Violation{Path: field, Kind: Undeclared, Message: fmt.Sprintf(format, args...)})
	}

	grants := a.Permissions.grants()
	for _, n := range an.hostPaths {
		if !slices.ContainsFunc(grants, func(g HostPathGrant) bool { return g.covers(n.path, n.readOnly) }) {
			want := HostPathGrant{Path: n.path, ReadOnly: n.readOnly}
			undeclared(n.field, "host path %q is not covered by permissions.host_paths", want.String())
		}
	}
	if !a.Permissions.DockerSocket {
		for _, f := range an.dockerSocket {
			undeclared(f, "mount exposes a container runtime socket, which needs permissions.docker_socket")
		}
	}
	if !a.Permissions.HostRoot {
		for _, f := range an.hostRoot {
			undeclared(f, "mount exposes a root-equivalent host path, which needs permissions.host_root")
		}
	}
	if !a.Permissions.HostNetwork {
		for _, f := range an.hostNetwork {
			undeclared(f, "host network needs permissions.host_network")
		}
	}
	if !a.Permissions.CoreNetwork {
		for _, f := range an.coreNetwork {
			undeclared(f, "the %q network gives access to the core services and needs permissions.core_network", CoreNetwork)
		}
	}
	ports := sortedKeys(an.hostPorts)
	for _, port := range ports {
		if !slices.Contains(a.Permissions.HostPorts, port) {
			for _, f := range an.hostPorts[port] {
				undeclared(f, "host port %d is not in permissions.host_ports", port)
			}
		}
	}
	return out
}

// grants parses HostPaths, dropping malformed entries.
func (p Permissions) grants() []HostPathGrant {
	var gs []HostPathGrant
	for _, s := range p.HostPaths {
		if g, err := ParseHostPathGrant(s); err == nil {
			gs = append(gs, g)
		}
	}
	return gs
}

// Canonical returns a copy with host paths and ports sorted and deduplicated, and with a path
// listed both read-only and read-write collapsed to read-write. Malformed host paths are kept
// verbatim so Hash still changes when they do.
func (p Permissions) Canonical() Permissions {
	out := Permissions{DockerSocket: p.DockerSocket, HostRoot: p.HostRoot, HostNetwork: p.HostNetwork, CoreNetwork: p.CoreNetwork}

	rw := map[string]bool{}
	var bad []string
	for _, s := range p.HostPaths {
		g, err := ParseHostPathGrant(s)
		if err != nil {
			bad = append(bad, s)
			continue
		}
		rw[g.Path] = rw[g.Path] || !g.ReadOnly
	}
	out.HostPaths = make([]string, 0, len(rw)+len(bad))
	for _, hp := range sortedKeys(rw) {
		out.HostPaths = append(out.HostPaths, HostPathGrant{Path: hp, ReadOnly: !rw[hp]}.String())
	}
	out.HostPaths = append(out.HostPaths, bad...)
	slices.Sort(out.HostPaths)
	out.HostPaths = slices.Compact(out.HostPaths)

	out.HostPorts = slices.Clone(p.HostPorts)
	if out.HostPorts == nil {
		out.HostPorts = []int{}
	}
	slices.Sort(out.HostPorts)
	out.HostPorts = slices.Compact(out.HostPorts)
	return out
}

// Hash is a stable digest of the canonical permissions, "sha256:<hex>". The install API makes
// the admin echo it back, which proves they saw exactly this set.
func (p Permissions) Hash() string {
	b, err := json.Marshal(p.Canonical())
	if err != nil {
		// Marshaling strings, ints and bools can't fail.
		panic(err)
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Grows reports whether p grants anything old doesn't, i.e. whether an update from old to p
// needs the admin to confirm again. A path counts as covered only when old grants that exact
// path with at least the same access; a new path below an old one is growth, because the old
// version may have planted a symlink there.
func (p Permissions) Grows(old Permissions) bool {
	if (p.DockerSocket && !old.DockerSocket) || (p.HostRoot && !old.HostRoot) ||
		(p.HostNetwork && !old.HostNetwork) || (p.CoreNetwork && !old.CoreNetwork) {
		return true
	}
	for _, port := range p.HostPorts {
		if !slices.Contains(old.HostPorts, port) {
			return true
		}
	}
	oldGrants := old.grants()
	for _, s := range p.HostPaths {
		g, err := ParseHostPathGrant(s)
		if err != nil {
			// Can't reason about it, so make the admin look at it.
			if !slices.Contains(old.HostPaths, s) {
				return true
			}
			continue
		}
		if !slices.ContainsFunc(oldGrants, func(o HostPathGrant) bool { return o.covers(g.Path, g.ReadOnly) }) {
			return true
		}
	}
	return false
}

func sortedKeys[K ~string | ~int, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// HostBind is one host path an addon bind mounts, either from a service volume or from a
// volume's bind driver_opts. ReadOnly is false if any mount of the path is read-write.
type HostBind struct {
	Path     string `json:"path"`
	ReadOnly bool   `json:"read_only"`
}

// HostBindSources returns the cleaned host paths a definition bind mounts, sorted by path, one
// entry per path. Syntactically invalid entries are skipped, so run Validate as well.
//
// A bind volume declared in driver_opts counts as read-write unless its "o" options say "ro",
// whatever the service mounts say, since docker creates the bind with those options.
func HostBindSources(a *Addon) []HostBind {
	rw := map[string]bool{}
	for _, n := range analyze(a).hostPaths {
		rw[n.path] = rw[n.path] || !n.readOnly
	}
	out := make([]HostBind, 0, len(rw))
	for _, p := range sortedKeys(rw) {
		out = append(out, HostBind{Path: p, ReadOnly: !rw[p]})
	}
	return out
}

// OverlapConflict reports whether two addons' bind sources could be used against each other:
// one source equals, contains or lies below the other and at least one side is read-write.
// The side that can write can plant a symlink the other side's mount then follows, or read and
// modify the other addon's data. The returned message names the first conflicting pair.
//
// CheckPolicy already rejects this within one addon; the runtime has to check it across
// addons, see the package doc.
func OverlapConflict(a, b []HostBind) (string, bool) {
	for _, x := range a {
		for _, y := range b {
			if x.ReadOnly && y.ReadOnly {
				continue
			}
			if isWithin(x.Path, y.Path) || isWithin(y.Path, x.Path) {
				return fmt.Sprintf("host path %s overlaps %s", x.describe(), y.describe()), true
			}
		}
	}
	return "", false
}

// UpgradeConflict reports whether next mounts a path strictly below a path prev mounted
// read-write. Files the previous version wrote stay on the host, so it may have planted a
// symlink there that next's mount would follow. Equal paths are fine, that is just the same
// data directory carried over.
func UpgradeConflict(prev, next []HostBind) (string, bool) {
	for _, n := range next {
		for _, p := range prev {
			if !p.ReadOnly && n.Path != p.Path && isWithin(n.Path, p.Path) {
				return fmt.Sprintf("host path %s is below %s mounted by the previous version", n.describe(), p.describe()), true
			}
		}
	}
	return "", false
}

func (b HostBind) describe() string {
	if b.ReadOnly {
		return fmt.Sprintf("%q (read-only)", b.Path)
	}
	return fmt.Sprintf("%q (read-write)", b.Path)
}
