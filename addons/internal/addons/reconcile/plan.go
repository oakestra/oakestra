package reconcile

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/oakestra/oakestra/addons/internal/addons"
	"github.com/oakestra/oakestra/addons/internal/addons/grant"
	"github.com/oakestra/oakestra/addons/internal/addons/runtime"
	"github.com/oakestra/oakestra/addons/internal/addons/store"
	"github.com/oakestra/oakestra/addons/internal/definition"
)

// PlaneRoot is the orchestration plane that runs the dashboard. Only there are UI services
// attached to the UI network, since the dashboard only talks to the root addons API.
const PlaneRoot = "root"

type servicePlan struct {
	name string // service_name
	spec runtime.ContainerSpec
}

type plan struct {
	services []servicePlan
	networks []runtime.NetworkSpec
	volumes  []runtime.VolumeSpec
}

// prefixed puts the plane in front of a definition-derived name. Root and cluster can share one
// docker daemon (1-DOC), so the same addon installed on both must not collide.
func (r *Reconciler) prefixed(name string) string {
	return r.Plane + "_" + name
}

func (r *Reconciler) containerName(id, service string) string {
	return r.prefixed(definition.ContainerName(id, service))
}

func (r *Reconciler) networkName(id, network string) string {
	return r.prefixed(definition.NetworkName(id, network))
}

func (r *Reconciler) volumeName(id, volume string) string {
	return r.prefixed(definition.VolumeName(id, volume))
}

// verify checks the grant signature and then re-runs every check the API ran at install time
// against the stored snapshot. The record lives in a database anyone with Mongo access can
// edit, so nothing about it is trusted until it passes here.
func (r *Reconciler) verify(a addons.Installed) (definition.Addon, error) {
	def := store.CloneDefinition(a.Definition)
	// First, so a forged record can't even make the loop pull an image or create a network.
	// Re-running the policy below isn't enough on its own: whoever rewrites the snapshot can
	// rewrite the grant to match.
	if err := r.GrantKey.Verify(a, r.Plane); err != nil {
		if errors.Is(err, grant.ErrMissing) {
			return def, errors.New("grant signature missing; re-approve the addon")
		}
		return def, errors.New("grant signature invalid; re-approve the addon")
	}
	if def.ID() != a.AddonID {
		return def, fmt.Errorf("snapshot is for %q but the record is for %q", def.ID(), a.AddonID)
	}
	if def.Version != a.Version {
		return def, fmt.Errorf("snapshot version %q does not match the recorded version %q", def.Version, a.Version)
	}
	if def.Source != a.Source || def.QualifiedName() != a.QualifiedName {
		return def, fmt.Errorf("snapshot is for %q but the record is for %q", def.QualifiedName(), a.QualifiedName)
	}
	// ModeInstall, not ModeIndex: local registrations are legitimate here. The policy may have
	// tightened since the addon was installed, in which case re-approving the same version
	// can't help.
	if err := definition.Check(&def, definition.ModeInstall); err != nil {
		return def, fmt.Errorf("the installed definition does not pass the current permission policy; update to a version that declares what it needs or remove the addon: %w", err)
	}
	// What the admin confirmed has to be exactly what the snapshot declares, and that has to
	// cover everything the snapshot needs. Check already guarantees the second for the declared
	// set, so a tampered snapshot or a tampered grant fails one of these two.
	if def.Permissions.Hash() != a.Granted.Hash() {
		return def, errors.New("the snapshot's permissions differ from the permissions that were granted")
	}
	granted := def
	granted.Permissions = a.Granted
	if v := definition.CheckPolicy(&granted); len(v) > 0 {
		return def, fmt.Errorf("the granted permissions don't cover the definition: %w", v[0])
	}
	return def, nil
}

// buildPlan turns a verified definition into the objects to create. It only reads fields that
// exist on the definition types and never copies anything else into container options.
func (r *Reconciler) buildPlan(a addons.Installed, def *definition.Addon) (*plan, error) {
	id := def.ID()
	p := &plan{}
	ownerLabels := r.recordLabels(a.ID)
	ownerLabels[runtime.LabelAddonKey] = id
	// No record id on volumes: they outlive the record, and a reinstall has a new one.
	volumeLabels := r.scope()
	volumeLabels[runtime.LabelAddonKey] = id

	for _, n := range def.Networks {
		p.networks = append(p.networks, runtime.NetworkSpec{
			Name: r.networkName(id, n.Name), Driver: n.Driver, EnableIPv6: n.EnableIPv6, Labels: maps.Clone(ownerLabels),
		})
	}
	for _, v := range def.Volumes {
		labels := maps.Clone(v.Labels)
		if labels == nil {
			labels = map[string]string{}
		}
		maps.Copy(labels, volumeLabels)
		p.volumes = append(p.volumes, runtime.VolumeSpec{
			Name: r.volumeName(id, v.Name), Driver: v.Driver, DriverOpts: maps.Clone(v.DriverOpts), Labels: labels,
		})
	}

	needDefault, needUI := false, false
	for i := range def.Services {
		s := &def.Services[i]
		spec := runtime.ContainerSpec{
			Name:  r.containerName(id, s.ServiceName),
			Image: s.Image,
		}

		cmd, err := splitCommand(s.Command)
		if err != nil {
			return nil, fmt.Errorf("service %s: command: %w", s.ServiceName, err)
		}
		spec.Cmd = cmd

		for _, k := range slices.Sorted(maps.Keys(s.Environment)) {
			spec.Env = append(spec.Env, k+"="+s.Environment[k])
		}

		// Definition labels first so the runtime's own always win. The policy already rejects
		// the reserved prefixes, this is a second line.
		spec.Labels = maps.Clone(s.Labels)
		if spec.Labels == nil {
			spec.Labels = map[string]string{}
		}
		maps.Copy(spec.Labels, ownerLabels)
		spec.Labels[runtime.LabelService] = s.ServiceName

		for _, v := range s.Volumes {
			m, err := definition.ParseMount(v)
			if err != nil {
				return nil, fmt.Errorf("service %s: %w", s.ServiceName, err)
			}
			switch m.Kind {
			case definition.MountHost:
				spec.Mounts = append(spec.Mounts, runtime.Mount{Source: m.HostPath, Bind: true, Target: m.Target, ReadOnly: m.ReadOnly})
			case definition.MountNamed:
				spec.Mounts = append(spec.Mounts, runtime.Mount{Source: r.volumeName(id, m.Source), Target: m.Target, ReadOnly: m.ReadOnly})
			default:
				spec.Mounts = append(spec.Mounts, runtime.Mount{Target: m.Target})
			}
		}

		for _, cp := range slices.Sorted(maps.Keys(s.Ports)) {
			b, err := definition.ParsePort(cp, s.Ports[cp])
			if err != nil {
				return nil, fmt.Errorf("service %s: %w", s.ServiceName, err)
			}
			spec.Ports = append(spec.Ports, runtime.PortBinding{ContainerPort: b.ContainerPort, Protocol: b.Protocol, HostIP: b.HostIP, HostPort: b.HostPort})
		}

		switch {
		case slices.Contains(s.Networks, "host"):
			spec.NetworkMode = "host"
		case slices.Contains(s.Networks, "none"):
			spec.NetworkMode = "none"
		default:
			if len(s.Networks) == 0 {
				needDefault = true
				spec.Networks = append(spec.Networks, runtime.NetAttach{Name: r.networkName(id, "default")})
			}
			for _, n := range s.Networks {
				if n == definition.CoreNetwork {
					// The core network belongs to compose. It is joined, never created or removed.
					spec.Networks = append(spec.Networks, runtime.NetAttach{Name: definition.CoreNetwork})
					continue
				}
				spec.Networks = append(spec.Networks, runtime.NetAttach{Name: r.networkName(id, n)})
			}
			if r.Plane == PlaneRoot && def.UI != nil && def.UI.Service == s.ServiceName {
				needUI = true
				spec.Networks = append(spec.Networks, runtime.NetAttach{Name: definition.UINetwork, Aliases: []string{definition.UIAlias(id)}})
			}
		}
		p.services = append(p.services, servicePlan{name: s.ServiceName, spec: spec})
	}

	if needDefault {
		p.networks = append(p.networks, runtime.NetworkSpec{Name: r.networkName(id, "default"), Driver: "bridge", Labels: maps.Clone(ownerLabels)})
	}
	if needUI {
		p.networks = append(p.networks, runtime.NetworkSpec{
			Name: definition.UINetwork, Driver: "bridge", Shared: true,
			Labels: r.scope(),
		})
	}
	return p, nil
}

// splitCommand splits a command string the way a shell would for plain words: whitespace
// separates arguments, single and double quotes group, a backslash escapes the next character
// outside single quotes. That matches shlex, so command strings in existing definitions split
// the same way.
func splitCommand(s string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inWord := false
	var quote rune
	escaped := false
	for _, c := range s {
		switch {
		case escaped:
			cur.WriteRune(c)
			escaped = false
		case quote == '\'':
			if c == '\'' {
				quote = 0
			} else {
				cur.WriteRune(c)
			}
		case c == '\\':
			escaped, inWord = true, true
		case quote == '"':
			if c == '"' {
				quote = 0
			} else {
				cur.WriteRune(c)
			}
		case c == '\'' || c == '"':
			quote, inWord = c, true
		case c == ' ' || c == '\t' || c == '\n':
			if inWord {
				args = append(args, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(c)
			inWord = true
		}
	}
	if quote != 0 || escaped {
		return nil, errors.New("unterminated quote or escape")
	}
	if inWord {
		args = append(args, cur.String())
	}
	return args, nil
}
