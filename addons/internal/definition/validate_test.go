package definition

import (
	"strings"
	"testing"
)

func errorPaths(errs Errors) []string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Path
	}
	return out
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		mode   Mode
		mutate func(a *Addon)
		// want lists the paths of the expected errors, in order.
		want []string
	}{
		{name: "valid", mutate: func(a *Addon) {}},
		{
			name: "valid with everything",
			mutate: func(a *Addon) {
				a.Description = "Metrics"
				a.Homepage = "https://example.com/monitoring"
				a.Maintainers = []string{"Jane <jane@example.com>"}
				a.UI = &UI{Service: "web", SDK: "^1.2.0"}
				a.Permissions = Permissions{HostPaths: []string{"/var/log:ro"}, HostPorts: []int{9100}}
				a.Volumes = []Volume{{Name: "data"}}
				a.Networks = []Network{{Name: "internal"}}
				a.Services[0].Networks = []string{"internal", "oakestra"}
				a.Services[0].Volumes = []string{"data:/data", "/var/log:/logs:ro", "/tmp/x"}
				a.Services[0].Ports = map[string]string{"9100": "9100"}
				a.Services[0].Environment = map[string]string{"LOG_LEVEL": "info"}
				a.Services[0].Command = "serve --port 80"
			},
		},
		{
			name:   "uppercase name",
			mutate: func(a *Addon) { a.Name = "Monitoring" },
			want:   []string{"name"},
		},
		{
			name:   "name with separator",
			mutate: func(a *Addon) { a.Name = "a--b" },
			want:   []string{"name"},
		},
		{
			name: "id too long for the UI alias",
			mutate: func(a *Addon) {
				a.Source = TapSource(strings.Repeat("t", 30))
				a.Name = strings.Repeat("n", 30)
			},
			want: []string{"name", "services[0].service_name"},
		},
		{
			name: "container name too long",
			mutate: func(a *Addon) {
				a.Source = TapSource(strings.Repeat("t", 20))
				a.Name = strings.Repeat("n", 20)
				a.Services[0].ServiceName = strings.Repeat("s", 20)
			},
			want: []string{"services[0].service_name"},
		},
		{
			name:   "tap named local",
			mutate: func(a *Addon) { a.Source = TapSource("local") },
			want:   []string{"source"},
		},
		{
			name:   "missing source",
			mutate: func(a *Addon) { a.Source = Source{} },
			want:   []string{"source"},
		},
		{
			name:   "local source in an index",
			mutate: func(a *Addon) { a.Source = LocalSource },
			want:   []string{"source"},
		},
		{
			name:   "official source as local registration",
			mode:   ModeLocal,
			mutate: func(a *Addon) {},
			want:   []string{"source"},
		},
		{
			name: "local registration with tag",
			mode: ModeLocal,
			mutate: func(a *Addon) {
				a.Source = LocalSource
				a.Services[0].Image = "localhost:5000/web:latest"
			},
		},
		{
			name:   "install of a pinned local registration",
			mode:   ModeInstall,
			mutate: func(a *Addon) { a.Source = LocalSource },
		},
		{
			name:   "install of a tap entry",
			mode:   ModeInstall,
			mutate: func(a *Addon) { a.Source = TapSource("acme") },
		},
		{
			name: "install of an unpinned local registration",
			mode: ModeInstall,
			mutate: func(a *Addon) {
				a.Source = LocalSource
				a.Services[0].Image = "localhost:5000/web:latest"
			},
			want: []string{"services[0].image"},
		},
		{
			name:   "install without a source",
			mode:   ModeInstall,
			mutate: func(a *Addon) { a.Source = Source{} },
			want:   []string{"source"},
		},
		{
			name:   "tag-only image in an index",
			mutate: func(a *Addon) { a.Services[0].Image = "ghcr.io/oakestra/web:1.3.0" },
			want:   []string{"services[0].image"},
		},
		{
			name:   "digest without tag",
			mutate: func(a *Addon) { a.Services[0].Image = "ghcr.io/oakestra/web@" + testDigest },
		},
		{
			name:   "short digest",
			mutate: func(a *Addon) { a.Services[0].Image = "web@sha256:abc" },
			want:   []string{"services[0].image"},
		},
		{
			name:   "uppercase digest",
			mutate: func(a *Addon) { a.Services[0].Image = "web@" + strings.ToUpper(testDigest) },
			want:   []string{"services[0].image"},
		},
		{
			name:   "image with whitespace",
			mutate: func(a *Addon) { a.Services[0].Image = "web @" + testDigest },
			want:   []string{"services[0].image"},
		},
		{
			name:   "missing image",
			mutate: func(a *Addon) { a.Services[0].Image = "" },
			want:   []string{"services[0].image"},
		},
		{
			name:   "missing version",
			mutate: func(a *Addon) { a.Version = "" },
			want:   []string{"version"},
		},
		{
			name:   "loose version",
			mutate: func(a *Addon) { a.Version = "v1.2" },
			want:   []string{"version"},
		},
		{
			name:   "no services",
			mutate: func(a *Addon) { a.Services = nil },
			want:   []string{"services"},
		},
		{
			name: "duplicate service",
			mutate: func(a *Addon) {
				a.Services = append(a.Services, a.Services[0])
			},
			want: []string{"services[1].service_name"},
		},
		{
			name:   "service name with underscore",
			mutate: func(a *Addon) { a.Services[0].ServiceName = "system_manager" },
			want:   []string{"services[0].service_name"},
		},
		{
			name:   "ui service unknown",
			mutate: func(a *Addon) { a.UI = &UI{Service: "frontend", SDK: "^1.0.0"} },
			want:   []string{"ui.service"},
		},
		{
			name:   "ui sdk missing",
			mutate: func(a *Addon) { a.UI = &UI{Service: "web"} },
			want:   []string{"ui.sdk"},
		},
		{
			name:   "ui sdk not a range",
			mutate: func(a *Addon) { a.UI = &UI{Service: "web", SDK: "latest"} },
			want:   []string{"ui.sdk"},
		},
		{
			name: "ui service on host network",
			mutate: func(a *Addon) {
				a.UI = &UI{Service: "web", SDK: "^1.0.0"}
				a.Services[0].Networks = []string{"host"}
			},
			want: []string{"ui.service"},
		},
		{
			name: "ui named like a dashboard module",
			mutate: func(a *Addon) {
				a.Name = "applications"
				a.UI = &UI{Service: "web", SDK: "^1.0.0"}
			},
			want: []string{"name"},
		},
		{
			name:   "dashboard module name without ui is fine",
			mutate: func(a *Addon) { a.Name = "applications" },
		},
		{
			name:   "undeclared network",
			mutate: func(a *Addon) { a.Services[0].Networks = []string{"root_orchestrator_default"} },
			want:   []string{"services[0].networks[0]"},
		},
		{
			name:   "host network combined",
			mutate: func(a *Addon) { a.Services[0].Networks = []string{"host", "oakestra"} },
			want:   []string{"services[0].networks[0]"},
		},
		{
			name:   "duplicate network entry",
			mutate: func(a *Addon) { a.Services[0].Networks = []string{"oakestra", "oakestra"} },
			want:   []string{"services[0].networks[1]"},
		},
		{
			name:   "declaring the core network",
			mutate: func(a *Addon) { a.Networks = []Network{{Name: "oakestra"}} },
			want:   []string{"networks[0].name"},
		},
		{
			name:   "listing the UI network",
			mutate: func(a *Addon) { a.Services[0].Networks = []string{UINetwork} },
			want:   []string{"services[0].networks[0]"},
		},
		{
			name:   "declaring the UI network",
			mutate: func(a *Addon) { a.Networks = []Network{{Name: UINetwork}} },
			want:   []string{"networks[0].name"},
		},
		{
			name:   "declaring host",
			mutate: func(a *Addon) { a.Networks = []Network{{Name: "host"}} },
			want:   []string{"networks[0].name"},
		},
		{
			name:   "undeclared named volume",
			mutate: func(a *Addon) { a.Services[0].Volumes = []string{"root_orchestrator_mongodb_data:/data"} },
			want:   []string{"services[0].volumes[0]"},
		},
		{
			name: "non-normalized host path",
			mutate: func(a *Addon) {
				a.Services[0].Volumes = []string{"/var//run/./docker.sock:/s", "/var/run/../run/docker.sock:/t", "/var/log/:/u"}
			},
			want: []string{"services[0].volumes[0]", "services[0].volumes[1]", "services[0].volumes[2]"},
		},
		{
			name: "relative and home paths",
			mutate: func(a *Addon) {
				a.Services[0].Volumes = []string{"./data:/data", "../etc:/etc2", "~/.ssh:/ssh"}
			},
			want: []string{"services[0].volumes[0]", "services[0].volumes[1]", "services[0].volumes[2]"},
		},
		{
			name: "volume modes",
			mutate: func(a *Addon) {
				a.Services[0].Volumes = []string{"/a:/a:z", "/b:/b:ro,rshared", "/c:ro", "/d:/d:ro:extra"}
			},
			want: []string{"services[0].volumes[0]", "services[0].volumes[1]", "services[0].volumes[2]", "services[0].volumes[3]"},
		},
		{
			name: "same target twice",
			mutate: func(a *Addon) {
				a.Services[0].Volumes = []string{"/a:/x", "/b:/x"}
			},
			want: []string{"services[0].volumes[1]"},
		},
		{
			name: "invalid ports",
			mutate: func(a *Addon) {
				a.Services[0].Ports = map[string]string{"80": "", "81": "70000", "82": "8000-8010", "83": "::1:80", "http": "80"}
			},
			want: []string{
				`services[0].ports["80"]`, `services[0].ports["81"]`, `services[0].ports["82"]`,
				`services[0].ports["83"]`, `services[0].ports["http"]`,
			},
		},
		{
			name: "same host port in two services",
			mutate: func(a *Addon) {
				a.Services[0].Ports = map[string]string{"80": "8080"}
				a.Services = append(a.Services, Service{ServiceName: "api", Image: a.Services[0].Image, Ports: map[string]string{"81": "8080"}})
			},
			want: []string{`services[1].ports["81"]`},
		},
		{
			name: "same host port different protocol",
			mutate: func(a *Addon) {
				a.Services[0].Ports = map[string]string{"53/udp": "53", "53/tcp": "53"}
			},
		},
		{
			name: "bad permissions",
			mutate: func(a *Addon) {
				a.Permissions = Permissions{HostPaths: []string{"var/log", "/var/log:rw", "/a/../b", "/x:ro"}, HostPorts: []int{0, 80, 80}}
			},
			want: []string{
				"permissions.host_paths[0]", "permissions.host_paths[1]", "permissions.host_paths[2]",
				"permissions.host_ports[0]", "permissions.host_ports[2]",
			},
		},
		{
			name:   "bad homepage",
			mutate: func(a *Addon) { a.Homepage = "javascript:alert(1)" },
			want:   []string{"homepage"},
		},
		{
			name:   "environment key with equals",
			mutate: func(a *Addon) { a.Services[0].Environment = map[string]string{"A=B": "c"} },
			want:   []string{`services[0].environment["A=B"]`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := baseAddon()
			tt.mutate(a)
			errs := Validate(a, tt.mode)
			if got := errorPaths(errs); strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("Validate:\n got %q\nwant %q\n%v", got, tt.want, errs)
			}
		})
	}
}

func TestCheckCombinesValidateAndPolicy(t *testing.T) {
	a := baseAddon()
	if err := Check(a, ModeIndex); err != nil {
		t.Fatalf("Check(valid) = %v", err)
	}
	a.Version = "nope"
	a.Services[0].Volumes = []string{"/:/host"}
	err := Check(a, ModeIndex)
	errs, ok := err.(Errors)
	// version, host path, docker socket and host root.
	if !ok || len(errs) != 4 {
		t.Fatalf("Check = %#v, want 4 errors", err)
	}
}

func TestParseImage(t *testing.T) {
	tests := []struct {
		in   string
		want ImageRef
		ok   bool
	}{
		{"nginx", ImageRef{Name: "nginx"}, true},
		{"nginx:1.25", ImageRef{Name: "nginx", Tag: "1.25"}, true},
		{"localhost:5000/ui", ImageRef{Name: "localhost:5000/ui"}, true},
		{"localhost:5000/ui:dev", ImageRef{Name: "localhost:5000/ui", Tag: "dev"}, true},
		{"ghcr.io/a/b:1@" + testDigest, ImageRef{Name: "ghcr.io/a/b", Tag: "1", Digest: testDigest}, true},
		{"ghcr.io/a/b@" + testDigest, ImageRef{Name: "ghcr.io/a/b", Digest: testDigest}, true},
		{"Nginx", ImageRef{}, false},
		{"nginx@sha512:abc", ImageRef{}, false},
		{"nginx:", ImageRef{}, false},
		{"", ImageRef{}, false},
		{"a//b", ImageRef{}, false},
	}
	for _, tt := range tests {
		got, err := ParseImage(tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("ParseImage(%q) = %+v, %v", tt.in, got, err)
		}
		if tt.ok && got.String() != tt.in {
			t.Errorf("ParseImage(%q).String() = %q", tt.in, got.String())
		}
	}
}
