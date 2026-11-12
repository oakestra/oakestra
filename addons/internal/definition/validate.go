package definition

import (
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// Mode selects the rules that depend on where a definition comes from.
type Mode int

const (
	// ModeIndex is for official and tap index entries: images must be digest pinned, so what
	// was reviewed is exactly what runs.
	ModeIndex Mode = iota
	// ModeLocal is for local registrations: tags are allowed and get resolved to a digest when
	// the definition is registered.
	ModeLocal
	// ModeInstall is for definitions about to run: the installer and the reconcile loop. Any
	// source kind is fine, since by then local registrations have had their tags resolved, but
	// every image must be digest pinned so what runs is what the admin accepted.
	ModeInstall
)

// requiresPinnedImages reports whether the mode only accepts digest pinned images.
func (m Mode) requiresPinnedImages() bool {
	return m == ModeIndex || m == ModeInstall
}

const (
	maxDescription = 4096
	maxHomepage    = 2048
	maxMaintainers = 32
	maxShortText   = 256
)

// reservedNetworkNames can't be declared by an addon: they are docker built-ins or the shared
// network, which already exists and needs no declaration.
var reservedNetworkNames = map[string]bool{
	"host":      true,
	"none":      true,
	"bridge":    true,
	"default":   true,
	CoreNetwork: true,
	UINetwork:   true,
}

// Check runs Validate and CheckPolicy and returns all problems as one Errors value, or nil.
func Check(a *Addon, mode Mode) error {
	errs := Validate(a, mode)
	for _, v := range CheckPolicy(a) {
		errs = append(errs, FieldError{Path: v.Path, Message: v.Message})
	}
	return errs.Err()
}

// Validate checks that a definition is well formed. It does not check permissions; see
// CheckPolicy. The addon's Source must be set (index decoding does that from the index).
func Validate(a *Addon, mode Mode) Errors {
	var errs Errors

	if err := ValidateSegment(a.Name); err != nil {
		errs = errs.add("name", "%v", err)
	}
	if err := ValidateSource(a.Source); err != nil {
		errs = errs.add("source", "%v", err)
	} else {
		switch {
		case mode == ModeIndex && a.Source.Kind == SourceLocal:
			errs = errs.add("source", "local definitions can't be part of an index")
		case mode == ModeLocal && a.Source.Kind != SourceLocal:
			errs = errs.add("source", "local registrations must use the local source")
		}
	}
	id := a.ID()
	if len(UIAlias(id)) > MaxDNSLabel {
		errs = errs.add("name", "id %q is too long, %q must fit in %d characters", id, UIAlias(id), MaxDNSLabel)
	}

	if a.Version == "" {
		errs = errs.add("version", "is required")
	} else if _, err := semver.StrictNewVersion(a.Version); err != nil {
		errs = errs.add("version", "%q is not a semantic version (e.g. 1.2.3): %v", a.Version, err)
	}
	if len(a.Description) > maxDescription {
		errs = errs.add("description", "is longer than %d characters", maxDescription)
	}
	if a.Homepage != "" {
		u, err := url.Parse(a.Homepage)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || len(a.Homepage) > maxHomepage {
			errs = errs.add("homepage", "must be an http(s) URL")
		}
	}
	if len(a.Maintainers) > maxMaintainers {
		errs = errs.add("maintainers", "has more than %d entries", maxMaintainers)
	}
	for i, m := range a.Maintainers {
		if strings.TrimSpace(m) == "" || len(m) > maxShortText {
			errs = errs.add(idx("maintainers", i), "must be a non-empty string of at most %d characters", maxShortText)
		}
	}

	errs = append(errs, validatePermissions(a.Permissions)...)

	volumes := map[string]bool{}
	for i, v := range a.Volumes {
		vp := idx("volumes", i)
		if !dockerObjectName.MatchString(v.Name) || len(v.Name) > maxShortText {
			errs = errs.add(vp+".name", "%q is not a valid volume name", v.Name)
		} else if volumes[v.Name] {
			errs = errs.add(vp+".name", "volume %q is declared twice", v.Name)
		}
		volumes[v.Name] = true
	}

	networks := map[string]bool{}
	for i, n := range a.Networks {
		np := idx("networks", i)
		switch {
		case !dockerObjectName.MatchString(n.Name) || len(n.Name) > maxShortText:
			errs = errs.add(np+".name", "%q is not a valid network name", n.Name)
		case reservedNetworkNames[n.Name]:
			errs = errs.add(np+".name", "%q is reserved and can't be declared", n.Name)
		case networks[n.Name]:
			errs = errs.add(np+".name", "network %q is declared twice", n.Name)
		}
		networks[n.Name] = true
	}

	if len(a.Services) == 0 {
		errs = errs.add("services", "at least one service is required")
	}
	serviceNames := map[string]bool{}
	hostPorts := map[string]string{} // "port/proto" -> field that binds it
	for i := range a.Services {
		sp := idx("services", i)
		s := &a.Services[i]
		errs = append(errs, validateService(s, mode, volumes, networks, hostPorts, sp)...)
		if serviceNames[s.ServiceName] {
			errs = errs.add(sp+".service_name", "service %q is declared twice", s.ServiceName)
		}
		serviceNames[s.ServiceName] = true
		if name := ContainerName(id, s.ServiceName); len(name) > MaxDNSLabel {
			errs = errs.add(sp+".service_name", "container name %q is longer than %d characters", name, MaxDNSLabel)
		}
	}

	if a.UI != nil {
		errs = append(errs, validateUI(a, id)...)
	}
	return errs
}

func validateUI(a *Addon, id string) Errors {
	var errs Errors
	if a.UI.Service == "" {
		errs = errs.add("ui.service", "is required")
	} else if s := a.Service(a.UI.Service); s == nil {
		errs = errs.add("ui.service", "%q must match one of the addon's services", a.UI.Service)
	} else {
		// The runtime attaches the UI service to UINetwork under the UI alias, which isn't
		// possible in the host or none network namespaces.
		for _, n := range s.Networks {
			if n == "host" || n == "none" {
				errs = errs.add("ui.service", "the UI service can't use the %q network", n)
			}
		}
	}
	if a.UI.SDK == "" {
		errs = errs.add("ui.sdk", "is required")
	} else if _, err := semver.NewConstraint(a.UI.SDK); err != nil {
		errs = errs.add("ui.sdk", "%q is not a semver range: %v", a.UI.SDK, err)
	}
	if ReservedUINames[id] {
		errs = errs.add("name", "%q is a built-in dashboard module, the dashboard would ignore this UI", id)
	}
	return errs
}

func validatePermissions(p Permissions) Errors {
	var errs Errors
	for i, s := range p.HostPaths {
		if _, err := ParseHostPathGrant(s); err != nil {
			errs = errs.add(idx("permissions.host_paths", i), "%v", err)
		}
	}
	seen := map[int]bool{}
	for i, port := range p.HostPorts {
		if port < 1 || port > 65535 {
			errs = errs.add(idx("permissions.host_ports", i), "%d is not a port between 1 and 65535", port)
		} else if seen[port] {
			errs = errs.add(idx("permissions.host_ports", i), "port %d is listed twice", port)
		}
		seen[port] = true
	}
	return errs
}

func validateService(s *Service, mode Mode, volumes, networks map[string]bool, hostPorts map[string]string, sp string) Errors {
	var errs Errors
	if err := ValidateSegment(s.ServiceName); err != nil {
		errs = errs.add(sp+".service_name", "%v", err)
	}

	if s.Image == "" {
		errs = errs.add(sp+".image", "is required")
	} else if ref, err := ParseImage(s.Image); err != nil {
		errs = errs.add(sp+".image", "%v", err)
	} else if mode.requiresPinnedImages() && !ref.Pinned() {
		errs = errs.add(sp+".image", "%q must be pinned by digest (name[:tag]@sha256:<64 hex>)", s.Image)
	}

	seenNet := map[string]bool{}
	for j, n := range s.Networks {
		np := idx(sp+".networks", j)
		switch {
		case seenNet[n]:
			errs = errs.add(np, "network %q is listed twice", n)
		case n == "host" || n == "none":
			if len(s.Networks) > 1 {
				errs = errs.add(np, "the %q network can't be combined with other networks", n)
			}
		case strings.HasPrefix(n, "container:"):
			// CheckPolicy rejects this; reporting it twice would only be noise.
		case n == UINetwork:
			errs = errs.add(np, "%q is attached by the runtime to the ui service only and can't be listed", n)
		case n != CoreNetwork && !networks[n]:
			errs = errs.add(np, "network %q is neither declared in networks nor %q", n, CoreNetwork)
		}
		seenNet[n] = true
	}

	targets := map[string]bool{}
	for j, spec := range s.Volumes {
		vp := idx(sp+".volumes", j)
		m, err := ParseMount(spec)
		if err != nil {
			errs = errs.add(vp, "%v", err)
			continue
		}
		if m.Kind == MountHost {
			if err := checkCleanAbs(m.Source); err != nil {
				errs = errs.add(vp, "%v", err)
			}
		}
		// Undeclared named volumes would be created by docker on the fly under the literal
		// name, which bypasses the per-addon namespacing in VolumeName and could attach a core
		// service's volume.
		if m.Kind == MountNamed && !volumes[m.Source] {
			errs = errs.add(vp, "volume %q is not declared in volumes", m.Source)
		}
		if targets[m.Target] {
			errs = errs.add(vp, "target %q is mounted twice", m.Target)
		}
		targets[m.Target] = true
	}

	for _, cp := range slices.Sorted(maps.Keys(s.Ports)) {
		pp := key(sp+".ports", cp)
		b, err := ParsePort(cp, s.Ports[cp])
		if err != nil {
			errs = errs.add(pp, "%v", err)
			continue
		}
		hk := fmt.Sprintf("%d/%s", b.HostPort, b.Protocol)
		if other, dup := hostPorts[hk]; dup {
			errs = errs.add(pp, "host port %s is already bound by %s", hk, other)
		} else {
			hostPorts[hk] = pp
		}
	}

	for _, k := range slices.Sorted(maps.Keys(s.Environment)) {
		if k == "" || strings.ContainsAny(k, "=\x00") {
			errs = errs.add(key(sp+".environment", k), "is not a valid environment variable name")
		}
	}
	for _, k := range slices.Sorted(maps.Keys(s.Labels)) {
		if k == "" {
			errs = errs.add(sp+".labels", "label keys must not be empty")
		}
	}
	return errs
}
