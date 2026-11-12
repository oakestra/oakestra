package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

// stopTimeoutSeconds is how long a container gets to exit on SIGTERM before it is killed.
const stopTimeoutSeconds = 10

// Docker is the Runtime backed by the Docker Engine API.
type Docker struct {
	cli *client.Client
	// Limits applies to every container created. Set it before the first CreateAndStart.
	Limits Limits
}

// NewDocker connects using the standard DOCKER_HOST environment and negotiates the API
// version, so the binary works against older and newer daemons.
func NewDocker() (*Docker, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Docker{cli: cli}, nil
}

// Close releases the client.
func (d *Docker) Close() error { return d.cli.Close() }

func (d *Docker) PullImage(ctx context.Context, ref string) error {
	// A digest names exactly one image, so a local copy can't be stale. Skipping the pull also
	// lets a redeploy work on a node that has lost its registry access.
	if strings.Contains(ref, "@sha256:") {
		if _, err := d.cli.ImageInspect(ctx, ref); err == nil {
			return nil
		}
	}
	rc, err := d.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return err
	}
	defer rc.Close()
	// The pull only finishes once the stream is drained, and failures that happen after the
	// request was accepted show up as {"error": ...} messages in it.
	dec := json.NewDecoder(rc)
	for {
		var msg struct {
			Error string `json:"error"`
		}
		if err := dec.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if msg.Error != "" {
			return errors.New(msg.Error)
		}
	}
}

func (d *Docker) ImageLabels(ctx context.Context, ref string) (map[string]string, error) {
	img, err := d.cli.ImageInspect(ctx, ref)
	if err != nil {
		return nil, err
	}
	if img.Config == nil {
		return nil, nil
	}
	return img.Config.Labels, nil
}

func (d *Docker) InspectContainer(ctx context.Context, name string) (*Container, error) {
	j, err := d.cli.ContainerInspect(ctx, name)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	c := &Container{ID: j.ID, Name: strings.TrimPrefix(j.Name, "/"), RestartCount: j.RestartCount}
	if j.Config != nil {
		c.Labels = j.Config.Labels
	}
	if j.State != nil {
		c.State, c.ExitCode = j.State.Status, j.State.ExitCode
	}
	return c, nil
}

func labelFilter(labels map[string]string) filters.Args {
	args := filters.NewArgs()
	for k, v := range labels {
		args.Add("label", k+"="+v)
	}
	return args
}

func (d *Docker) ListContainers(ctx context.Context, labels map[string]string) ([]Container, error) {
	list, err := d.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: labelFilter(labels)})
	if err != nil {
		return nil, err
	}
	out := make([]Container, 0, len(list))
	for _, c := range list {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		if c.State == "running" || c.State == "paused" {
			// Exit codes and restart counts only matter for containers that aren't running.
			out = append(out, Container{ID: c.ID, Name: name, Labels: c.Labels, State: c.State})
			continue
		}
		// The list call doesn't report exit codes or restart counts, so fetch them.
		full, err := d.InspectContainer(ctx, c.ID)
		if err != nil || full == nil {
			out = append(out, Container{ID: c.ID, Name: name, Labels: c.Labels, State: c.State})
			continue
		}
		out = append(out, *full)
	}
	return out, nil
}

func (d *Docker) CreateAndStart(ctx context.Context, spec ContainerSpec) error {
	cfg, host := buildCreateConfig(spec, d.Limits)

	var netCfg *network.NetworkingConfig
	rest := spec.Networks
	if len(spec.Networks) > 0 {
		// A container can only be created on one network. The others are connected before
		// start. Aliases of the first network have to be set at create time.
		first := spec.Networks[0]
		host.NetworkMode = container.NetworkMode(first.Name)
		netCfg = &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{
			first.Name: {Aliases: first.Aliases},
		}}
		rest = spec.Networks[1:]
	}

	created, err := d.cli.ContainerCreate(ctx, cfg, host, netCfg, nil, spec.Name)
	if err != nil {
		if errdefs.IsNotFound(err) && netCfg != nil {
			return fmt.Errorf("%w: %v", ErrNetworkMissing, err)
		}
		return err
	}
	cleanup := func(cause error) error {
		_ = d.cli.ContainerRemove(context.WithoutCancel(ctx), created.ID, container.RemoveOptions{Force: true})
		return cause
	}
	for _, n := range rest {
		if err := d.cli.NetworkConnect(ctx, n.Name, created.ID, &network.EndpointSettings{Aliases: n.Aliases}); err != nil {
			if errdefs.IsNotFound(err) {
				err = fmt.Errorf("%w: %s", ErrNetworkMissing, n.Name)
			}
			return cleanup(err)
		}
	}
	if err := d.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return cleanup(err)
	}
	return nil
}

// Log rotation for addon containers. Without it the json-file driver grows without bound, and
// a chatty or hostile addon can fill the disk the control plane runs on.
const (
	logMaxSize  = "10m"
	logMaxFiles = "3"
)

// buildCreateConfig maps a spec onto docker's create options. The privilege related fields
// are set explicitly rather than left at their zero value, so a future change to the spec type
// can't silently widen what addons get.
func buildCreateConfig(spec ContainerSpec, limits Limits) (*container.Config, *container.HostConfig) {
	cfg := &container.Config{
		Image:        spec.Image,
		Cmd:          spec.Cmd,
		Env:          spec.Env,
		Labels:       spec.Labels,
		ExposedPorts: nat.PortSet{},
		Volumes:      map[string]struct{}{},
	}
	host := &container.HostConfig{
		Privileged: false,
		CapAdd:     nil,
		// NET_RAW is in docker's default set but only needed for raw sockets (ping, packet
		// crafting); dropping it stops ARP and DNS spoofing on the networks an addon joins.
		CapDrop: []string{"NET_RAW"},
		// Stops setuid binaries in the image from gaining privileges the process didn't start
		// with.
		SecurityOpt:   []string{"no-new-privileges:true"},
		PidMode:       "",
		IpcMode:       "",
		UsernsMode:    "",
		UTSMode:       "",
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
		NetworkMode:   container.NetworkMode(spec.NetworkMode),
		PortBindings:  nat.PortMap{},
		LogConfig: container.LogConfig{
			Type:   "json-file",
			Config: map[string]string{"max-size": logMaxSize, "max-file": logMaxFiles},
		},
	}

	host.Resources.Devices = nil
	// Addons share the host with the control plane, which targets ~1 GB in total; a fork bomb
	// or a leak in one addon must not take the orchestrator down with it.
	if limits.PidsLimit > 0 {
		pids := limits.PidsLimit
		host.Resources.PidsLimit = &pids
	}
	host.Resources.Memory = limits.MemoryBytes
	host.Resources.NanoCPUs = limits.NanoCPUs

	for _, m := range spec.Mounts {
		switch {
		case m.Source == "":
			cfg.Volumes[m.Target] = struct{}{}
		default:
			bind := m.Source + ":" + m.Target
			if m.ReadOnly {
				bind += ":ro"
			}
			host.Binds = append(host.Binds, bind)
		}
	}
	for _, p := range spec.Ports {
		port := nat.Port(strconv.Itoa(p.ContainerPort) + "/" + p.Protocol)
		cfg.ExposedPorts[port] = struct{}{}
		host.PortBindings[port] = append(host.PortBindings[port], nat.PortBinding{HostIP: p.HostIP, HostPort: strconv.Itoa(p.HostPort)})
	}
	return cfg, host
}

func (d *Docker) RemoveContainer(ctx context.Context, id string) error {
	timeout := stopTimeoutSeconds
	if err := d.cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout}); err != nil && !errdefs.IsNotFound(err) {
		// A failed stop shouldn't leave the container behind, Force kills it.
		_ = err
	}
	err := d.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true, RemoveVolumes: false})
	if err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	return nil
}

func (d *Docker) ContainerLogs(ctx context.Context, id string, tail int) (string, error) {
	rc, err := d.cli.ContainerLogs(ctx, id, container.LogsOptions{ShowStdout: true, ShowStderr: true, Tail: strconv.Itoa(tail)})
	if err != nil {
		return "", err
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, 1<<20))
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	// Containers without a TTY multiplex stdout and stderr into one stream with headers.
	if _, err := stdcopy.StdCopy(&out, &out, bytes.NewReader(raw)); err != nil {
		return string(raw), nil
	}
	return out.String(), nil
}

func (d *Docker) findNetwork(ctx context.Context, name string) (*network.Inspect, error) {
	// The name filter matches substrings ("oakestra" also finds "oakestra_addon_ui"), so the
	// exact name is checked on the result.
	list, err := d.cli.NetworkList(ctx, network.ListOptions{Filters: filters.NewArgs(filters.Arg("name", name))})
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Name == name {
			return &list[i], nil
		}
	}
	return nil, nil
}

func (d *Docker) EnsureNetwork(ctx context.Context, spec NetworkSpec) error {
	have, err := d.findNetwork(ctx, spec.Name)
	if err != nil {
		return err
	}
	if have != nil {
		if spec.Shared || sameOwner(have.Labels, spec.Labels, networkOwnerLabels) {
			return nil
		}
		return fmt.Errorf("network %q: %w", spec.Name, ErrForeign)
	}
	opts := network.CreateOptions{Driver: spec.Driver, Labels: spec.Labels}
	if spec.EnableIPv6 {
		t := true
		opts.EnableIPv6 = &t
	}
	if _, err := d.cli.NetworkCreate(ctx, spec.Name, opts); err != nil {
		// Two reconcilers (or a compose up) racing on a shared network is fine.
		if spec.Shared && errdefs.IsConflict(err) {
			return nil
		}
		return err
	}
	return nil
}

func (d *Docker) ListNetworks(ctx context.Context, labels map[string]string) ([]string, error) {
	list, err := d.cli.NetworkList(ctx, network.ListOptions{Filters: labelFilter(labels)})
	if err != nil {
		return nil, err
	}
	names := make([]string, len(list))
	for i, n := range list {
		names[i] = n.Name
	}
	return names, nil
}

func (d *Docker) RemoveNetwork(ctx context.Context, name string) error {
	err := d.cli.NetworkRemove(ctx, name)
	if err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	return nil
}

func (d *Docker) EnsureVolume(ctx context.Context, spec VolumeSpec) error {
	have, err := d.cli.VolumeInspect(ctx, spec.Name)
	if err == nil {
		if !sameOwner(have.Labels, spec.Labels, volumeOwnerLabels) {
			return fmt.Errorf("volume %q: %w", spec.Name, ErrForeign)
		}
		if !sameVolumeConfig(have.Driver, have.Options, spec) {
			return fmt.Errorf("volume %q: %w; remove it by hand if its data can go", spec.Name, ErrVolumeMismatch)
		}
		return nil
	}
	if !errdefs.IsNotFound(err) {
		return err
	}
	_, err = d.cli.VolumeCreate(ctx, volume.CreateOptions{
		Name: spec.Name, Driver: spec.Driver, DriverOpts: spec.DriverOpts, Labels: spec.Labels,
	})
	return err
}

var _ Runtime = (*Docker)(nil)
