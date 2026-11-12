package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oakestra/oakestra/addons/internal/definition"
)

const digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

const monitoringYAML = `name: monitoring
version: 1.3.0
description: Node metrics
ui:
  service: web
  sdk: ^1.2.0
permissions:
  host_paths: ["/proc:ro"]
  host_ports: [9100]
  host_root: true
services:
  - service_name: web
    image: ghcr.io/oakestra/monitoring-web:1.3.0@` + digest + `
  - service_name: exporter
    image: ghcr.io/oakestra/node-exporter@` + digest + `
    volumes: ["/proc:/host/proc:ro"]
    ports:
      "9100": "9100"
`

const loggingJSON = `{"name": "logging", "version": "0.1.0", "services": [{"service_name": "agent", "image": "x/agent@` + digest + `"}]}`

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func runCmd(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestValidate(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"monitoring.yaml": monitoringYAML,
		"logging.json":    loggingJSON,
		"hostile.yaml": `name: hostile
version: 1.0.0
services:
  - service_name: pwn
    image: alpine:latest
    networks: [host]
    volumes:
      - /:/host
      - /var/run/docker.sock:/var/run/docker.sock
`,
		"broken.yaml":   "name: [",
		"unknown.yaml":  strings.Replace(monitoringYAML, "description:", "privileged: true\ndescription:", 1),
		"sourced.yaml":  "source: {kind: local, name: local}\n" + monitoringYAML,
		"readme.txt":    "hello",
		"Uppercase.yml": strings.Replace(monitoringYAML, "name: monitoring", "name: Monitoring", 1),
		// encoding/json would let the second spelling win silently.
		"shadowed.yaml": strings.Replace(monitoringYAML, "  - service_name: exporter\n", "  - service_name: exporter\n    Image: evil/exporter@"+digest+"\n", 1),
	})
	p := func(name string) string { return filepath.Join(dir, name) }

	code, out, errOut := runCmd("validate", p("monitoring.yaml"), p("logging.json"))
	if code != 0 || !strings.Contains(out, "2 files valid") {
		t.Fatalf("valid files: code %d\n%s\n%s", code, out, errOut)
	}

	code, _, errOut = runCmd("validate", p("hostile.yaml"))
	if code != 1 {
		t.Fatalf("hostile: code %d", code)
	}
	for _, want := range []string{
		"services[0].image",
		"services[0].networks[0]: host network needs permissions.host_network",
		`services[0].volumes[0]: host path "/" is not covered`,
		"services[0].volumes[1]: mount exposes a container runtime socket",
		"services[0].volumes[0]: mount exposes a root-equivalent host path",
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("hostile: output lacks %q:\n%s", want, errOut)
		}
	}
	// Every problem is reported, not just the first one.
	if n := strings.Count(errOut, "hostile.yaml:"); n != 8 {
		t.Errorf("hostile: expected 8 problems, got %d:\n%s", n, errOut)
	}

	for _, f := range []string{"broken.yaml", "unknown.yaml", "sourced.yaml", "readme.txt", "Uppercase.yml", "shadowed.yaml", "missing.yaml"} {
		if code, _, errOut := runCmd("validate", p(f)); code != 1 {
			t.Errorf("%s: code %d, want 1\n%s", f, code, errOut)
		}
	}

	// A mix of good and bad files still fails, and names the bad one.
	code, _, errOut = runCmd("validate", p("monitoring.yaml"), p("hostile.yaml"))
	if code != 1 || !strings.Contains(errOut, "1 of 2 files failed") {
		t.Errorf("mixed: code %d\n%s", code, errOut)
	}

	if code, _, _ := runCmd("validate", "--source", "local", p("monitoring.yaml")); code != 2 {
		t.Errorf("--source local: code %d, want 2", code)
	}
	if code, _, _ := runCmd("validate"); code != 2 {
		t.Errorf("no files: code %d, want 2", code)
	}
}

func TestBuildIndex(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"addons/monitoring.yaml": monitoringYAML,
		"addons/logging.json":    loggingJSON,
		"README.md":              "not a definition",
	})
	out1 := filepath.Join(dir, "index1.json")
	out2 := filepath.Join(dir, "index2.json")

	args := []string{"build-index", "--source", "official", "--serial", "42", "--generated-at", "2026-11-01T10:00:00Z"}
	if code, stdout, stderr := runCmd(append(args, "--out", out1, dir)...); code != 0 {
		t.Fatalf("build-index: code %d\n%s\n%s", code, stdout, stderr)
	}
	// Passing the addons directory directly works too and gives identical output.
	if code, _, stderr := runCmd(append(args, "--out", out2, filepath.Join(dir, "addons"))...); code != 0 {
		t.Fatalf("build-index (addons dir): %s", stderr)
	}
	b1, _ := os.ReadFile(out1)
	b2, _ := os.ReadFile(out2)
	if !bytes.Equal(b1, b2) {
		t.Fatal("build-index is not deterministic")
	}

	idx, err := definition.DecodeIndex(bytes.NewReader(b1))
	if err != nil {
		t.Fatalf("decoding output: %v", err)
	}
	if idx.Serial != 42 || idx.Source != "official" || len(idx.Addons) != 2 {
		t.Fatalf("unexpected index: %+v", idx)
	}
	if idx.Addons[0].Name != "logging" || idx.Addons[1].Name != "monitoring" {
		t.Errorf("addons not sorted: %s, %s", idx.Addons[0].Name, idx.Addons[1].Name)
	}
	if !bytes.Contains(b1, []byte(`"signatures": []`)) {
		t.Error("signatures not emitted as an empty list")
	}
}

func TestBuildIndexRejects(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		args  []string
		code  int
	}{
		{
			name:  "invalid definition",
			files: map[string]string{"addons/monitoring.yaml": strings.Replace(monitoringYAML, "@"+digest, "", 1)},
			code:  1,
		},
		{
			name:  "file name differs from addon name",
			files: map[string]string{"addons/metrics.yaml": monitoringYAML},
			code:  1,
		},
		{
			name: "duplicate addon",
			files: map[string]string{
				"addons/monitoring.yaml": monitoringYAML,
				"addons/monitoring.yml":  monitoringYAML,
			},
			code: 1,
		},
		{name: "empty catalog", files: map[string]string{"addons/.keep": ""}, code: 1},
		{name: "zero serial", files: map[string]string{"addons/monitoring.yaml": monitoringYAML}, args: []string{"--serial", "0"}, code: 2},
		{name: "negative serial", files: map[string]string{"addons/monitoring.yaml": monitoringYAML}, args: []string{"--serial", "-1"}, code: 2},
		{name: "reserved source", files: map[string]string{"addons/monitoring.yaml": monitoringYAML}, args: []string{"--source", "local"}, code: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, tt.files)
			out := filepath.Join(dir, "index.json")
			args := []string{"build-index", "--source", "official", "--serial", "1", "--out", out}
			args = append(args, tt.args...)
			code, _, stderr := runCmd(append(args, dir)...)
			if code != tt.code {
				t.Errorf("code %d, want %d\n%s", code, tt.code, stderr)
			}
			if _, err := os.Stat(out); err == nil {
				t.Error("index written despite errors")
			}
		})
	}
}
