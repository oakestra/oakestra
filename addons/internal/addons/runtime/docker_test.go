package runtime

import (
	"testing"

	"github.com/docker/docker/api/types/container"
)

func TestBuildCreateConfigIsUnprivileged(t *testing.T) {
	cfg, host := buildCreateConfig(ContainerSpec{
		Name: "root_addon-x--web", Image: "img@sha256:abc", Cmd: []string{"run"},
		Env:    []string{"A=b"},
		Labels: map[string]string{LabelAddonID: "1"},
		Mounts: []Mount{
			{Source: "/var/log", Bind: true, Target: "/logs", ReadOnly: true},
			{Source: "root_addon-x_data", Target: "/data"},
			{Target: "/anon"},
		},
		Ports:       []PortBinding{{ContainerPort: 80, Protocol: "tcp", HostIP: "127.0.0.1", HostPort: 8080}},
		NetworkMode: "none",
	}, Limits{PidsLimit: 1024, MemoryBytes: 512 << 20, NanoCPUs: 1e9})

	if host.Privileged || len(host.CapAdd) != 0 || len(host.Devices) != 0 {
		t.Errorf("host config is not locked down: %+v", host)
	}
	if len(host.SecurityOpt) != 1 || host.SecurityOpt[0] != "no-new-privileges:true" {
		t.Errorf("security opts = %v", host.SecurityOpt)
	}
	if len(host.CapDrop) != 1 || host.CapDrop[0] != "NET_RAW" {
		t.Errorf("cap drop = %v", host.CapDrop)
	}
	if host.PidsLimit == nil || *host.PidsLimit != 1024 || host.Memory != 512<<20 || host.NanoCPUs != 1e9 {
		t.Errorf("limits: pids %v memory %d cpus %d", host.PidsLimit, host.Memory, host.NanoCPUs)
	}
	if host.LogConfig.Type != "json-file" || host.LogConfig.Config["max-size"] != "10m" || host.LogConfig.Config["max-file"] != "3" {
		t.Errorf("log config = %+v", host.LogConfig)
	}
	if host.PidMode != "" || host.IpcMode != "" || host.UsernsMode != "" || host.UTSMode != "" {
		t.Errorf("namespaces are shared: %+v", host)
	}
	if host.RestartPolicy.Name != container.RestartPolicyUnlessStopped {
		t.Errorf("restart policy = %v", host.RestartPolicy)
	}
	if host.NetworkMode != "none" {
		t.Errorf("network mode = %q", host.NetworkMode)
	}
	if len(host.Binds) != 2 || host.Binds[0] != "/var/log:/logs:ro" || host.Binds[1] != "root_addon-x_data:/data" {
		t.Errorf("binds = %v", host.Binds)
	}
	if _, ok := cfg.Volumes["/anon"]; !ok {
		t.Errorf("anonymous volume missing: %v", cfg.Volumes)
	}
	pb := host.PortBindings["80/tcp"]
	if len(pb) != 1 || pb[0].HostIP != "127.0.0.1" || pb[0].HostPort != "8080" {
		t.Errorf("port bindings = %v", host.PortBindings)
	}
}

func TestBuildCreateConfigWithoutLimits(t *testing.T) {
	_, host := buildCreateConfig(ContainerSpec{Name: "x", Image: "img"}, Limits{})
	if host.PidsLimit != nil || host.Memory != 0 || host.NanoCPUs != 0 {
		t.Errorf("disabled limits were set: pids %v memory %d cpus %d", host.PidsLimit, host.Memory, host.NanoCPUs)
	}
	// Hardening that isn't a tunable stays on.
	if len(host.SecurityOpt) != 1 || len(host.CapDrop) != 1 {
		t.Errorf("hardening depends on limits: %v %v", host.SecurityOpt, host.CapDrop)
	}
}

func TestSameVolumeConfig(t *testing.T) {
	bind := map[string]string{"type": "none", "o": "bind", "device": "/etc"}
	for _, tc := range []struct {
		name   string
		driver string
		opts   map[string]string
		spec   VolumeSpec
		want   bool
	}{
		{"default driver", "local", nil, VolumeSpec{}, true},
		{"empty opts", "local", map[string]string{}, VolumeSpec{Driver: "local"}, true},
		{"old bind, new plain", "local", bind, VolumeSpec{}, false},
		{"same bind", "local", bind, VolumeSpec{DriverOpts: map[string]string{"type": "none", "o": "bind", "device": "/etc"}}, true},
		{"other device", "local", bind, VolumeSpec{DriverOpts: map[string]string{"type": "none", "o": "bind", "device": "/srv"}}, false},
		{"other driver", "nfs", nil, VolumeSpec{}, false},
	} {
		if got := sameVolumeConfig(tc.driver, tc.opts, tc.spec); got != tc.want {
			t.Errorf("%s: %v", tc.name, got)
		}
	}
}

func TestSameOwnerNeedsEveryLabel(t *testing.T) {
	want := map[string]string{LabelAddonKey: "a", LabelPlane: "root", LabelInstance: "i1"}
	if !sameOwner(map[string]string{LabelAddonKey: "a", LabelPlane: "root", LabelInstance: "i1", "x": "y"}, want, volumeOwnerLabels) {
		t.Error("own volume rejected")
	}
	for _, have := range []map[string]string{
		{LabelAddonKey: "a", LabelPlane: "root"},
		{LabelAddonKey: "a", LabelPlane: "root", LabelInstance: "i2"},
		{LabelAddonKey: "b", LabelPlane: "root", LabelInstance: "i1"},
		nil,
	} {
		if sameOwner(have, want, volumeOwnerLabels) {
			t.Errorf("%v accepted", have)
		}
	}
	if sameOwner(nil, map[string]string{}, volumeOwnerLabels) {
		t.Error("empty owner matches")
	}
}
