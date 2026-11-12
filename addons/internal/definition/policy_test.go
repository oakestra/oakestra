package definition

import (
	"slices"
	"strings"
	"testing"
)

const testDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// baseAddon returns a definition that passes Validate in ModeIndex and CheckPolicy.
func baseAddon() *Addon {
	a := &Addon{
		Name:    "monitoring",
		Source:  OfficialSource,
		Version: "1.3.0",
		Services: []Service{{
			ServiceName: "web",
			Image:       "ghcr.io/oakestra/web:1.3.0@" + testDigest,
		}},
	}
	a.Normalize()
	return a
}

// policyResult renders violations as "kind path" strings so tests can match on them.
func policyResult(vs []Violation) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = string(v.Kind) + " " + v.Path
	}
	return out
}

func TestCheckPolicy(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(a *Addon)
		// want lists "kind path" for every expected violation, in order.
		want []string
	}{
		{
			name:   "plain addon",
			mutate: func(a *Addon) {},
		},
		{
			name: "named volume needs nothing",
			mutate: func(a *Addon) {
				a.Volumes = []Volume{{Name: "data"}}
				a.Services[0].Volumes = []string{"data:/data", "/cache"}
			},
		},
		{
			name: "root filesystem",
			mutate: func(a *Addon) {
				a.Services[0].Volumes = []string{"/:/host"}
			},
			want: []string{"undeclared services[0].volumes[0]", "undeclared services[0].volumes[0]", "undeclared services[0].volumes[0]"},
		},
		{
			name: "root filesystem declared but socket not",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/"}
				a.Services[0].Volumes = []string{"/:/host:ro"}
			},
			want: []string{"undeclared services[0].volumes[0]", "undeclared services[0].volumes[0]"},
		},
		{
			name: "root filesystem fully declared",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/:ro"}
				a.Permissions.DockerSocket = true
				a.Permissions.HostRoot = true
				a.Services[0].Volumes = []string{"/:/host:ro"}
			},
		},
		{
			name: "docker socket",
			mutate: func(a *Addon) {
				a.Services[0].Volumes = []string{"/var/run/docker.sock:/var/run/docker.sock"}
			},
			want: []string{"undeclared services[0].volumes[0]", "undeclared services[0].volumes[0]"},
		},
		{
			name: "docker socket listed in host_paths only",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/var/run/docker.sock"}
				a.Services[0].Volumes = []string{"/var/run/docker.sock:/var/run/docker.sock"}
			},
			want: []string{"undeclared services[0].volumes[0]"},
		},
		{
			name: "docker socket fully declared",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/var/run/docker.sock"}
				a.Permissions.DockerSocket = true
				a.Services[0].Volumes = []string{"/var/run/docker.sock:/var/run/docker.sock"}
			},
		},
		{
			name: "docker socket via parent directory",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/var/run"}
				a.Services[0].Volumes = []string{"/var/run:/hostrun"}
			},
			want: []string{"undeclared services[0].volumes[0]", "undeclared services[0].volumes[0]"},
		},
		{
			name: "docker socket via /run",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/run"}
				a.Services[0].Volumes = []string{"/run:/hostrun"}
			},
			want: []string{"undeclared services[0].volumes[0]", "undeclared services[0].volumes[0]"},
		},
		{
			name: "docker socket via /var",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/var"}
				a.Services[0].Volumes = []string{"/var:/hostvar:ro"}
			},
			want: []string{"undeclared services[0].volumes[0]", "undeclared services[0].volumes[0]"},
		},
		{
			name: "rootless docker socket",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/run/user/1000"}
				a.Services[0].Volumes = []string{"/run/user/1000:/x"}
			},
			want: []string{"undeclared services[0].volumes[0]"},
		},
		{
			name: "docker socket in an unusual place",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/home/me/.docker/run/docker.sock"}
				a.Services[0].Volumes = []string{"/home/me/.docker/run/docker.sock:/var/run/docker.sock"}
			},
			want: []string{"undeclared services[0].volumes[0]"},
		},
		{
			name: "containerd socket",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/run/containerd/containerd.sock"}
				a.Services[0].Volumes = []string{"/run/containerd/containerd.sock:/c.sock"}
			},
			want: []string{"undeclared services[0].volumes[0]", "undeclared services[0].volumes[0]"},
		},
		{
			name: "non-normalized socket path",
			mutate: func(a *Addon) {
				a.Services[0].Volumes = []string{"/var//run/./docker.sock:/var/run/docker.sock"}
			},
			want: []string{"undeclared services[0].volumes[0]", "undeclared services[0].volumes[0]"},
		},
		{
			name: "dot-dot socket path",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/var/log"}
				a.Services[0].Volumes = []string{"/var/log/../run/docker.sock:/s"}
			},
			want: []string{"undeclared services[0].volumes[0]", "undeclared services[0].volumes[0]"},
		},
		{
			name: "dot-dot escape from a declared directory",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/var/log"}
				a.Services[0].Volumes = []string{"/var/log/../../etc:/etc-host"}
			},
			want: []string{"undeclared services[0].volumes[0]", "undeclared services[0].volumes[0]"},
		},
		{
			name: "dot-dot is compared after cleaning",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/var/log/app"}
				a.Services[0].Volumes = []string{"/var/log/nginx/../app:/logs"}
			},
		},
		{
			name: "subdirectory of a declared path is not covered",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/var/log:ro"}
				a.Services[0].Volumes = []string{"/var/log/nginx:/logs:ro"}
			},
			want: []string{"undeclared services[0].volumes[0]"},
		},
		{
			name: "sibling with a shared prefix is not covered",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/var/log"}
				a.Services[0].Volumes = []string{"/var/logs-secret:/x"}
			},
			want: []string{"undeclared services[0].volumes[0]"},
		},
		{
			name: "read-only grant mounted read-write",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/var/log:ro"}
				a.Services[0].Volumes = []string{"/var/log:/logs"}
			},
			want: []string{"undeclared services[0].volumes[0]"},
		},
		{
			name: "read-only grant mounted explicitly rw",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/var/log:ro"}
				a.Services[0].Volumes = []string{"/var/log:/logs:rw"}
			},
			want: []string{"undeclared services[0].volumes[0]"},
		},
		{
			name: "read-write grant covers read-only mount",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/var/log"}
				a.Services[0].Volumes = []string{"/var/log:/logs:ro"}
			},
		},
		{
			name: "malformed grant counts as absent",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/var//log"}
				a.Services[0].Volumes = []string{"/var/log:/logs"}
			},
			want: []string{"undeclared services[0].volumes[0]"},
		},
		{
			name: "host network",
			mutate: func(a *Addon) {
				a.Services[0].Networks = []string{"host"}
			},
			want: []string{"undeclared services[0].networks[0]"},
		},
		{
			name: "host network declared",
			mutate: func(a *Addon) {
				a.Permissions.HostNetwork = true
				a.Services[0].Networks = []string{"host"}
			},
		},
		{
			name: "core network",
			mutate: func(a *Addon) {
				a.Services[0].Networks = []string{"oakestra"}
			},
			want: []string{"undeclared services[0].networks[0]"},
		},
		{
			name: "core network declared",
			mutate: func(a *Addon) {
				a.Permissions.CoreNetwork = true
				a.Services[0].Networks = []string{"oakestra"}
			},
		},
		{
			name: "no networks does not mean core network",
			mutate: func(a *Addon) {
				a.Services[0].Networks = nil
			},
		},
		{
			name: "none network needs nothing",
			mutate: func(a *Addon) {
				a.Services[0].Networks = []string{"none"}
			},
		},
		{
			name: "container network namespace",
			mutate: func(a *Addon) {
				a.Permissions.HostNetwork = true
				a.Services[0].Networks = []string{"container:root_system_manager"}
			},
			want: []string{"forbidden services[0].networks[0]"},
		},
		{
			name: "host port",
			mutate: func(a *Addon) {
				a.Services[0].Ports = map[string]string{"80/tcp": "8080"}
			},
			want: []string{`undeclared services[0].ports["80/tcp"]`},
		},
		{
			name: "host port with address",
			mutate: func(a *Addon) {
				a.Permissions.HostPorts = []int{8080}
				a.Services[0].Ports = map[string]string{"80": "127.0.0.1:8080", "53/udp": "53"}
			},
			want: []string{`undeclared services[0].ports["53/udp"]`},
		},
		{
			name: "bind volume via driver_opts",
			mutate: func(a *Addon) {
				a.Volumes = []Volume{{Name: "etc", DriverOpts: map[string]string{"type": "none", "o": "bind", "device": "/etc"}}}
				a.Services[0].Volumes = []string{"etc:/etc-host:ro"}
			},
			want: []string{`undeclared volumes[0].driver_opts["device"]`, `undeclared volumes[0].driver_opts["device"]`},
		},
		{
			name: "bind volume via driver_opts declared",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/etc"}
				a.Permissions.HostRoot = true
				a.Volumes = []Volume{{Name: "etc", Driver: "local", DriverOpts: map[string]string{"type": "none", "o": "bind", "device": "/etc"}}}
			},
		},
		{
			name: "read-only bind volume via driver_opts",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/etc:ro"}
				a.Permissions.HostRoot = true
				a.Volumes = []Volume{{Name: "etc", DriverOpts: map[string]string{"type": "none", "o": "bind,ro", "device": "/etc"}}}
			},
		},
		{
			name: "bind flag hidden behind tmpfs type",
			mutate: func(a *Addon) {
				a.Volumes = []Volume{{Name: "etc", DriverOpts: map[string]string{"type": "tmpfs", "o": "size=1m,bind", "device": "/etc"}}}
			},
			want: []string{`forbidden volumes[0].driver_opts["type"]`},
		},
		{
			name: "rbind to the docker socket directory",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/var/run"}
				a.Volumes = []Volume{{Name: "run", DriverOpts: map[string]string{"type": "none", "o": "rbind", "device": "/var/run"}}}
			},
			want: []string{`undeclared volumes[0].driver_opts["device"]`, `undeclared volumes[0].driver_opts["device"]`},
		},
		{
			name: "bind with non-normalized device",
			mutate: func(a *Addon) {
				a.Volumes = []Volume{{Name: "etc", DriverOpts: map[string]string{"type": "none", "o": "bind", "device": "/var/../etc"}}}
			},
			want: []string{`forbidden volumes[0].driver_opts["device"]`},
		},
		{
			name: "tmpfs volume",
			mutate: func(a *Addon) {
				a.Volumes = []Volume{{Name: "scratch", DriverOpts: map[string]string{"type": "tmpfs", "device": "tmpfs", "o": "size=100m,uid=1000"}}}
			},
		},
		{
			name: "nfs volume",
			mutate: func(a *Addon) {
				a.Volumes = []Volume{{Name: "nfs", DriverOpts: map[string]string{"type": "nfs", "o": "addr=10.0.0.1", "device": ":/export"}}}
			},
			want: []string{`forbidden volumes[0].driver_opts["type"]`},
		},
		{
			name: "device without type",
			mutate: func(a *Addon) {
				a.Volumes = []Volume{{Name: "dev", DriverOpts: map[string]string{"device": "/dev/sda1"}}}
			},
			want: []string{`forbidden volumes[0].driver_opts["type"]`},
		},
		{
			name: "unknown driver option",
			mutate: func(a *Addon) {
				a.Volumes = []Volume{{Name: "x", DriverOpts: map[string]string{"foo": "bar"}}}
			},
			want: []string{`forbidden volumes[0].driver_opts["foo"]`},
		},
		{
			name: "third-party volume driver",
			mutate: func(a *Addon) {
				a.Volumes = []Volume{{Name: "x", Driver: "rexray/ebs"}}
			},
			want: []string{"forbidden volumes[0].driver"},
		},
		{
			name: "overlay network",
			mutate: func(a *Addon) {
				a.Networks = []Network{{Name: "mesh", Driver: "overlay"}}
			},
			want: []string{"forbidden networks[0].driver"},
		},
		{
			name: "host network driver",
			mutate: func(a *Addon) {
				a.Permissions.HostNetwork = true
				a.Networks = []Network{{Name: "h", Driver: "host"}}
			},
			want: []string{"forbidden networks[0].driver"},
		},
		{
			name: "bridge network",
			mutate: func(a *Addon) {
				a.Networks = []Network{{Name: "internal", Driver: "bridge"}, {Name: "other"}}
			},
		},
		{
			name: "dockerd state directory",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/run/docker"}
				a.Services[0].Volumes = []string{"/run/docker:/d"}
			},
			want: []string{"undeclared services[0].volumes[0]"},
		},
		{
			name: "dockerd state directory via /var/run",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/var/run/docker/plugins:ro"}
				a.Services[0].Volumes = []string{"/var/run/docker/plugins:/d:ro"}
			},
			want: []string{"undeclared services[0].volumes[0]"},
		},
		{
			name: "read-only /proc needs host_root",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/proc:ro"}
				a.Services[0].Volumes = []string{"/proc:/host/proc:ro"}
			},
			want: []string{"undeclared services[0].volumes[0]"},
		},
		{
			name: "read-only /proc with host_root",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/proc:ro"}
				a.Permissions.HostRoot = true
				a.Services[0].Volumes = []string{"/proc:/host/proc:ro"}
			},
		},
		{
			name: "file below /etc",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/etc/x:ro"}
				a.Services[0].Volumes = []string{"/etc/x:/x:ro"}
			},
			want: []string{"undeclared services[0].volumes[0]"},
		},
		{
			name: "docker volume data",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/var/lib/docker/volumes"}
				a.Services[0].Volumes = []string{"/var/lib/docker/volumes:/v"}
			},
			want: []string{"undeclared services[0].volumes[0]"},
		},
		{
			name: "ancestor of a root-equivalent path",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/var/lib:ro"}
				a.Services[0].Volumes = []string{"/var/lib:/v:ro"}
			},
			want: []string{"undeclared services[0].volumes[0]"},
		},
		{
			name: "ordinary host path needs no host_root",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/var/log"}
				a.Services[0].Volumes = []string{"/var/log:/logs"}
			},
		},
		{
			name: "a home directory is not the whole of /home",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/home/app/data"}
				a.Services[0].Volumes = []string{"/home/app/data:/data"}
			},
		},
		{
			name: "symlink plant: rw parent and child mount in another service",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/opt/addon", "/opt/addon/s:ro"}
				a.Services[0].Volumes = []string{"/opt/addon:/data"}
				a.Services = append(a.Services, Service{
					ServiceName: "victim",
					Image:       "x@" + testDigest,
					Volumes:     []string{"/opt/addon/s:/s:ro"},
				})
			},
			want: []string{"forbidden services[1].volumes[0]"},
		},
		{
			name: "symlink plant: child mount in the same service",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/opt/addon", "/opt/addon/a/b"}
				a.Services[0].Volumes = []string{"/opt/addon/a/b:/b", "/opt/addon:/data"}
			},
			want: []string{"forbidden services[0].volumes[0]"},
		},
		{
			name: "symlink plant: driver_opts bind device below a rw bind",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/opt/addon", "/opt/addon/s"}
				a.Services[0].Volumes = []string{"/opt/addon:/data"}
				a.Volumes = []Volume{{Name: "s", DriverOpts: map[string]string{"type": "none", "o": "bind", "device": "/opt/addon/s"}}}
			},
			want: []string{`forbidden volumes[0].driver_opts["device"]`},
		},
		{
			name: "symlink plant: rw driver_opts bind with a child service mount",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/opt/addon", "/opt/addon/s:ro"}
				a.Volumes = []Volume{{Name: "d", DriverOpts: map[string]string{"type": "none", "o": "bind", "device": "/opt/addon"}}}
				a.Services[0].Volumes = []string{"/opt/addon/s:/s:ro"}
			},
			want: []string{"forbidden services[0].volumes[0]"},
		},
		{
			name: "read-only parent and child mount",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/opt/addon:ro", "/opt/addon/s:ro"}
				a.Services[0].Volumes = []string{"/opt/addon:/data:ro", "/opt/addon/s:/s:ro"}
			},
		},
		{
			name: "rw child below a read-only parent",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/opt/addon:ro", "/opt/addon/s"}
				a.Services[0].Volumes = []string{"/opt/addon:/data:ro", "/opt/addon/s:/s"}
			},
		},
		{
			name: "same rw path in two services",
			mutate: func(a *Addon) {
				a.Permissions.HostPaths = []string{"/opt/addon"}
				a.Services[0].Volumes = []string{"/opt/addon:/data"}
				a.Services = append(a.Services, Service{ServiceName: "b", Image: "x@" + testDigest, Volumes: []string{"/opt/addon:/d:ro"}})
			},
		},
		{
			name: "runtime bookkeeping labels",
			mutate: func(a *Addon) {
				a.Services[0].Labels = map[string]string{
					"oak.addon.id":               "someone-else",
					"com.docker.compose.project": "root_orchestrator",
					"OAK.service.name":           "x",
					"org.example.fine":           "yes",
				}
			},
			want: []string{
				`forbidden services[0].labels["OAK.service.name"]`,
				`forbidden services[0].labels["com.docker.compose.project"]`,
				`forbidden services[0].labels["oak.addon.id"]`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := baseAddon()
			tt.mutate(a)
			got := policyResult(CheckPolicy(a))
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("CheckPolicy:\n got %q\nwant %q\n%v", got, tt.want, CheckPolicy(a))
			}
		})
	}
}

func TestRequiredPermissions(t *testing.T) {
	a := baseAddon()
	a.Volumes = []Volume{{Name: "etc", DriverOpts: map[string]string{"type": "none", "o": "bind,ro", "device": "/etc"}}}
	a.Services[0].Volumes = []string{"/var/log:/a:ro", "/var/log:/b", "/srv/data:/c:ro", "/var/run/docker.sock:/d"}
	a.Services[0].Ports = map[string]string{"80": "8080", "443": "8443"}
	a.Services = append(a.Services, Service{
		ServiceName: "agent",
		Image:       "x@" + testDigest,
		Networks:    []string{"host"},
		Ports:       map[string]string{"81": "8080"},
	}, Service{
		ServiceName: "api",
		Image:       "x@" + testDigest,
		Networks:    []string{"oakestra"},
	})

	got := RequiredPermissions(a)
	want := Permissions{
		HostPaths:    []string{"/etc:ro", "/srv/data:ro", "/var/log", "/var/run/docker.sock"},
		HostPorts:    []int{8080, 8443},
		DockerSocket: true,
		HostRoot:     true,
		HostNetwork:  true,
		CoreNetwork:  true,
	}
	if got.Hash() != want.Hash() {
		t.Fatalf("RequiredPermissions = %+v, want %+v", got, want)
	}

	// Declaring exactly what is required must satisfy the policy.
	a.Permissions = got
	for _, v := range CheckPolicy(a) {
		t.Errorf("unexpected violation after declaring required permissions: %v", v)
	}
}

func TestPermissionsHash(t *testing.T) {
	a := Permissions{HostPaths: []string{"/b", "/a:ro", "/a"}, HostPorts: []int{443, 80, 80}}
	b := Permissions{HostPaths: []string{"/a", "/b"}, HostPorts: []int{80, 443}}
	if a.Hash() != b.Hash() {
		t.Errorf("equivalent permissions hash differently: %s vs %s", a.Hash(), b.Hash())
	}
	if !strings.HasPrefix(a.Hash(), "sha256:") || len(a.Hash()) != len("sha256:")+64 {
		t.Errorf("unexpected hash format %q", a.Hash())
	}
	if (Permissions{}).Hash() != (Permissions{HostPaths: []string{}, HostPorts: []int{}}).Hash() {
		t.Error("nil and empty slices hash differently")
	}

	different := []Permissions{
		{HostPaths: []string{"/a:ro", "/b"}, HostPorts: []int{80, 443}},
		{HostPaths: []string{"/a", "/b"}, HostPorts: []int{80}},
		{HostPaths: []string{"/a", "/b"}, HostPorts: []int{80, 443}, DockerSocket: true},
		{HostPaths: []string{"/a", "/b"}, HostPorts: []int{80, 443}, HostRoot: true},
		{HostPaths: []string{"/a", "/b"}, HostPorts: []int{80, 443}, HostNetwork: true},
		{HostPaths: []string{"/a", "/b"}, HostPorts: []int{80, 443}, CoreNetwork: true},
		{HostPaths: []string{"/a", "/b", "/a//"}, HostPorts: []int{80, 443}},
	}
	for _, d := range different {
		if d.Hash() == b.Hash() {
			t.Errorf("%+v hashes like %+v", d, b)
		}
	}
}

func TestPermissionsGrows(t *testing.T) {
	old := Permissions{HostPaths: []string{"/var/log:ro", "/srv"}, HostPorts: []int{80}}
	tests := []struct {
		name  string
		p     Permissions
		grows bool
	}{
		{"same", old, false},
		{"empty", Permissions{}, false},
		{"subset", Permissions{HostPaths: []string{"/srv"}}, false},
		{"rw becomes ro", Permissions{HostPaths: []string{"/srv:ro", "/var/log:ro"}}, false},
		{"child of a read-write path", Permissions{HostPaths: []string{"/srv", "/srv/data"}}, true},
		{"read-only child of a read-write path", Permissions{HostPaths: []string{"/srv/x:ro"}}, true},
		{"host root", Permissions{HostRoot: true}, true},
		{"ro becomes rw", Permissions{HostPaths: []string{"/var/log"}}, true},
		{"parent directory", Permissions{HostPaths: []string{"/var:ro"}}, true},
		{"new port", Permissions{HostPorts: []int{80, 81}}, true},
		{"docker socket", Permissions{DockerSocket: true}, true},
		{"host network", Permissions{HostNetwork: true}, true},
		{"core network", Permissions{CoreNetwork: true}, true},
		{"malformed path", Permissions{HostPaths: []string{"relative"}}, true},
	}
	for _, tt := range tests {
		if got := tt.p.Grows(old); got != tt.grows {
			t.Errorf("%s: Grows = %v, want %v", tt.name, got, tt.grows)
		}
	}
}

func TestHostBindSources(t *testing.T) {
	a := baseAddon()
	a.Services[0].Volumes = []string{"/srv/b:/b:ro", "/srv/a:/a:ro", "/srv/a/../a:/a2", "data:/data"}
	a.Volumes = []Volume{
		{Name: "data"},
		{Name: "bound", DriverOpts: map[string]string{"type": "none", "o": "bind", "device": "/srv/c"}},
		{Name: "bound-ro", DriverOpts: map[string]string{"type": "none", "o": "bind,ro", "device": "/srv/d"}},
	}
	got := HostBindSources(a)
	want := []HostBind{{"/srv/a", false}, {"/srv/b", true}, {"/srv/c", false}, {"/srv/d", true}}
	if !slices.Equal(got, want) {
		t.Errorf("HostBindSources = %v, want %v", got, want)
	}
}

func TestOverlapConflict(t *testing.T) {
	rw := func(p string) HostBind { return HostBind{Path: p} }
	ro := func(p string) HostBind { return HostBind{Path: p, ReadOnly: true} }
	tests := []struct {
		name     string
		a, b     []HostBind
		conflict bool
	}{
		{"disjoint", []HostBind{rw("/opt/a")}, []HostBind{rw("/opt/b")}, false},
		{"shared prefix is not nesting", []HostBind{rw("/opt/a")}, []HostBind{rw("/opt/ab")}, false},
		{"same path both read-only", []HostBind{ro("/srv")}, []HostBind{ro("/srv")}, false},
		{"nested both read-only", []HostBind{ro("/srv")}, []HostBind{ro("/srv/x")}, false},
		{"same path one rw", []HostBind{rw("/srv")}, []HostBind{ro("/srv")}, true},
		{"other addon mounts below a rw path", []HostBind{rw("/opt/a")}, []HostBind{ro("/opt/a/s")}, true},
		{"other addon mounts above a rw path", []HostBind{rw("/opt/a/s")}, []HostBind{ro("/opt")}, true},
		{"rw below a read-only path", []HostBind{ro("/opt")}, []HostBind{rw("/opt/a")}, true},
		{"root and anything rw", []HostBind{ro("/")}, []HostBind{rw("/opt/a")}, true},
		{"empty", nil, []HostBind{rw("/")}, false},
	}
	for _, tt := range tests {
		msg, got := OverlapConflict(tt.a, tt.b)
		if got != tt.conflict {
			t.Errorf("%s: OverlapConflict = %v (%q), want %v", tt.name, got, msg, tt.conflict)
		}
		if _, rev := OverlapConflict(tt.b, tt.a); rev != got {
			t.Errorf("%s: OverlapConflict is not symmetric", tt.name)
		}
		if got && msg == "" {
			t.Errorf("%s: conflict without a message", tt.name)
		}
	}
}

func TestUpgradeConflict(t *testing.T) {
	prev := []HostBind{{Path: "/opt/a"}, {Path: "/srv", ReadOnly: true}}
	tests := []struct {
		name     string
		next     []HostBind
		conflict bool
	}{
		{"same paths", prev, false},
		{"same path now read-only", []HostBind{{Path: "/opt/a", ReadOnly: true}}, false},
		{"below a previously rw path", []HostBind{{Path: "/opt/a/s", ReadOnly: true}}, true},
		{"below a previously read-only path", []HostBind{{Path: "/srv/x"}}, false},
		{"above a previously rw path", []HostBind{{Path: "/opt", ReadOnly: true}}, false},
	}
	for _, tt := range tests {
		if _, got := UpgradeConflict(prev, tt.next); got != tt.conflict {
			t.Errorf("%s: UpgradeConflict = %v, want %v", tt.name, got, tt.conflict)
		}
	}
}

func TestIsReservedLabel(t *testing.T) {
	for k, want := range map[string]bool{
		"oak.addon.id":                     true,
		"OAK.addon.id":                     true,
		"Oakestra.role":                    true,
		"org.oakestra.x":                   true,
		"com.docker.compose.project":       true,
		"org.opencontainers.image.version": false,
		"oakley":                           false,
	} {
		if got := IsReservedLabel(k); got != want {
			t.Errorf("IsReservedLabel(%q) = %v, want %v", k, got, want)
		}
	}
}
