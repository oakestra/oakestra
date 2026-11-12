package definition

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

const validIndex = `{
  "schema": 1,
  "source": "acme",
  "serial": 42,
  "generated_at": "2026-11-01T10:00:00Z",
  "addons": [{
    "name": "monitoring",
    "version": "1.3.0",
    "description": "Metrics",
    "ui": {"service": "web", "sdk": "^1.2.0"},
    "permissions": {"host_paths": ["/var/log:ro"], "host_ports": [9100], "docker_socket": false, "host_network": false},
    "services": [{
      "service_name": "web",
      "image": "ghcr.io/x/web@` + testDigest + `",
      "volumes": ["/var/log:/logs:ro"],
      "ports": {"9100": "9100"}
    }],
    "volumes": [],
    "networks": []
  }],
  "signatures": [{"keyid": "future", "sig": "ignored"}]
}`

func TestDecodeIndex(t *testing.T) {
	idx, err := DecodeIndex(strings.NewReader(validIndex))
	if err != nil {
		t.Fatalf("DecodeIndex: %v", err)
	}
	if idx.Serial != 42 || len(idx.Addons) != 1 {
		t.Fatalf("unexpected index %+v", idx)
	}
	a := idx.Addons[0]
	if a.Source != TapSource("acme") {
		t.Errorf("source not stamped: %+v", a.Source)
	}
	if a.ID() != "acme--monitoring" || a.QualifiedName() != "acme/monitoring" {
		t.Errorf("id %q qualified %q", a.ID(), a.QualifiedName())
	}
	if a.Services[0].Environment == nil || a.Services[0].Networks == nil {
		t.Error("addon not normalized")
	}
	if errs := ValidateIndex(idx); len(errs) != 0 {
		t.Errorf("ValidateIndex: %v", errs)
	}
}

func TestDecodeIndexRejects(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"unknown top-level field", strings.Replace(validIndex, `"serial": 42,`, `"serial": 42, "extra": 1,`, 1)},
		{"unknown addon field", strings.Replace(validIndex, `"version": "1.3.0",`, `"version": "1.3.0", "privileged": true,`, 1)},
		{"unknown service field", strings.Replace(validIndex, `"service_name": "web",`, `"service_name": "web", "cap_add": ["SYS_ADMIN"],`, 1)},
		{"unknown permission", strings.Replace(validIndex, `"docker_socket": false,`, `"docker_socket": false, "privileged": true,`, 1)},
		{"wrong schema", strings.Replace(validIndex, `"schema": 1`, `"schema": 2`, 1)},
		{"tap named local", strings.Replace(validIndex, `"source": "acme"`, `"source": "local"`, 1)},
		{"addon from another source", strings.Replace(validIndex, `"name": "monitoring",`, `"name": "monitoring", "source": {"kind": "official", "name": "official"},`, 1)},
		{"trailing data", validIndex + `{}`},
		{"negative serial", strings.Replace(validIndex, `"serial": 42`, `"serial": -1`, 1)},
		{"not json", "<html>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeIndex(strings.NewReader(tt.in)); err == nil {
				t.Error("DecodeIndex succeeded")
			}
		})
	}
}

func TestDecodeIndexSizeLimit(t *testing.T) {
	big := strings.Replace(validIndex, `"Metrics"`, `"`+strings.Repeat("x", MaxIndexSize)+`"`, 1)
	if _, err := DecodeIndex(strings.NewReader(big)); !errors.Is(err, ErrTooLarge) {
		t.Errorf("DecodeIndex(oversized) = %v, want ErrTooLarge", err)
	}
}

func TestValidateIndexReportsEverything(t *testing.T) {
	idx, err := DecodeIndex(strings.NewReader(validIndex))
	if err != nil {
		t.Fatal(err)
	}
	idx.Serial = 0
	idx.Addons = append(idx.Addons, idx.Addons[0])
	idx.Addons[1].Services = []Service{{ServiceName: "x", Image: "nginx:latest", Volumes: []string{"/var/run/docker.sock:/s"}}}
	idx.Addons[1].Normalize()

	got := errorPaths(ValidateIndex(idx))
	want := []string{
		"serial",
		"addons[1].name",
		"addons[1].services[0].image",
		"addons[1].ui.service",
		"addons[1].services[0].volumes[0]",
		"addons[1].services[0].volumes[0]",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("ValidateIndex:\n got %q\nwant %q", got, want)
	}
}

func TestNewIndexIsDeterministic(t *testing.T) {
	b := *baseAddon()
	b.Name = "b"
	a := *baseAddon()
	a.Name = "a"
	at := time.Date(2026, 11, 1, 10, 0, 0, 0, time.FixedZone("x", 3600))

	out1, _ := json.Marshal(NewIndex("official", 7, at, []Addon{b, a}))
	out2, _ := json.Marshal(NewIndex("official", 7, at, []Addon{a, b}))
	if !bytes.Equal(out1, out2) {
		t.Fatalf("index output depends on input order:\n%s\n%s", out1, out2)
	}
	if bytes.Contains(out1, []byte(`"source":{`)) {
		t.Error("per-addon source leaked into the index")
	}

	idx, err := DecodeIndex(bytes.NewReader(out1))
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if idx.Addons[0].Name != "a" || !idx.GeneratedAt.Equal(at) || idx.GeneratedAt.Location() != time.UTC {
		t.Errorf("unexpected round trip %+v", idx)
	}
	if errs := ValidateIndex(idx); len(errs) != 0 {
		t.Errorf("ValidateIndex(round trip): %v", errs)
	}
}

func TestUnmarshalAddonYAML(t *testing.T) {
	good := `
name: monitoring
version: 1.3.0
permissions:
  host_ports: [9100]
services:
  - service_name: web
    image: ghcr.io/x/web@` + testDigest + `
    ports:
      "9100": "9100"
`
	a, err := UnmarshalAddonYAML([]byte(good))
	if err != nil {
		t.Fatalf("UnmarshalAddonYAML: %v", err)
	}
	a.Source = OfficialSource
	if err := Check(a, ModeIndex); err != nil {
		t.Errorf("Check: %v", err)
	}

	bad := map[string]string{
		"unknown field":  good + "privileged: true\n",
		"duplicate key":  good + "name: other\n",
		"unquoted port":  strings.Replace(good, `"9100": "9100"`, `"9100": 9100`, 1),
		"not a document": "- a\n- b\n",
	}
	for name, in := range bad {
		if _, err := UnmarshalAddonYAML([]byte(in)); err == nil {
			t.Errorf("%s: UnmarshalAddonYAML succeeded", name)
		}
	}
}

func TestDecodeRejectsAmbiguousKeys(t *testing.T) {
	img := `"image": "ghcr.io/x/web@` + testDigest + `"`
	evil := `"Image": "ghcr.io/x/evil@` + testDigest + `"`
	tests := []struct {
		name string
		in   string
	}{
		{"case variant of a service field in an index", strings.Replace(validIndex, `"service_name": "web",`, `"service_name": "web", `+evil+`,`, 1)},
		{"exact duplicate in an index", strings.Replace(validIndex, `"serial": 42,`, `"serial": 42, "serial": 43,`, 1)},
		{"miscased top-level field", strings.Replace(validIndex, `"serial": 42`, `"Serial": 42`, 1)},
		{"miscased permission", strings.Replace(validIndex, `"docker_socket": false`, `"Docker_Socket": true`, 1)},
		{"duplicate host port mapping", strings.Replace(validIndex, `"9100": "9100"`, `"9100": "9100", "9100": "9200"`, 1)},
		{"duplicate inside signatures", strings.Replace(validIndex, `"sig": "ignored"`, `"sig": "a", "sig": "b"`, 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeIndex(strings.NewReader(tt.in)); err == nil {
				t.Error("DecodeIndex succeeded")
			}
		})
	}

	addon := func(svc string) string {
		return `{"name": "a", "version": "1.0.0", "services": [{"service_name": "web", ` + svc + `}]}`
	}
	if _, err := DecodeAddon(strings.NewReader(addon(img))); err != nil {
		t.Fatalf("DecodeAddon(valid): %v", err)
	}
	for name, in := range map[string]string{
		"case variant after the real key":  addon(img + ", " + evil),
		"case variant before the real key": addon(evil + ", " + img),
		"case variant alone":               addon(evil),
		"exact duplicate":                  addon(img + ", " + img),
		"duplicate environment variable":   addon(img + `, "environment": {"A": "1", "A": "2"}`),
		"duplicate label":                  addon(img + `, "labels": {"x": "1", "x": "2"}`),
		"miscased nested field":            `{"name": "a", "version": "1.0.0", "ui": {"Service": "web", "sdk": "^1"}, "services": []}`,
	} {
		if _, err := DecodeAddon(strings.NewReader(in)); err == nil {
			t.Errorf("%s: DecodeAddon succeeded", name)
		}
	}

	// Keys that only differ in case are distinct environment variables and labels.
	a, err := DecodeAddon(strings.NewReader(addon(img + `, "environment": {"PATH": "/a", "path": "/b"}, "labels": {"Tier": "1", "tier": "2"}`)))
	if err != nil {
		t.Fatalf("DecodeAddon(case-distinct map keys): %v", err)
	}
	if len(a.Services[0].Environment) != 2 || len(a.Services[0].Labels) != 2 {
		t.Errorf("map keys collapsed: %+v", a.Services[0])
	}

	yamlDoc := "name: a\nversion: 1.0.0\nservices:\n  - service_name: web\n    image: ghcr.io/x/web@" + testDigest + "\n"
	if _, err := UnmarshalAddonYAML([]byte(yamlDoc)); err != nil {
		t.Fatalf("UnmarshalAddonYAML(valid): %v", err)
	}
	for name, in := range map[string]string{
		"case variant":   yamlDoc + "    Image: ghcr.io/x/evil@" + testDigest + "\n",
		"miscased field": strings.Replace(yamlDoc, "version:", "Version:", 1),
	} {
		if _, err := UnmarshalAddonYAML([]byte(in)); err == nil {
			t.Errorf("YAML %s: UnmarshalAddonYAML succeeded", name)
		}
	}
	if _, err := UnmarshalAddonYAML([]byte(yamlDoc + "    environment:\n      PATH: /a\n      path: /b\n")); err != nil {
		t.Errorf("YAML case-distinct environment: %v", err)
	}
}
