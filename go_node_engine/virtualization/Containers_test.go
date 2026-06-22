package virtualization

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

func TestSnameExtractionFromTaskid(t *testing.T) {
	sname := "test.test.nginx.test"
	taskid := genTaskID(sname, 23)
	extracted := extractSnameFromTaskID(taskid)
	assert.Equal(t, extracted, sname)
}

func TestSnameExtractionFromTaskidMultipleInstance(t *testing.T) {
	sname := "test.test.instance.test"
	taskid := genTaskID(sname, 23)
	extracted := extractSnameFromTaskID(taskid)
	assert.Equal(t, extracted, sname)
}

func TestInstanceExtractionFromTaskid(t *testing.T) {
	sname := "test.test.nginx.test"
	iid := 23
	taskid := genTaskID(sname, iid)
	extracted := extractInstanceNumberFromTaskID(taskid)
	assert.Equal(t, extracted, iid)
}

func TestInstanceExtractionFromTaskidMultipleInstance(t *testing.T) {
	sname := "test.test.instance.test"
	iid := 23
	taskid := genTaskID(sname, iid)
	extracted := extractInstanceNumberFromTaskID(taskid)
	assert.Equal(t, extracted, iid)
}

// Regression test for #512: ranging the result of findAdditionalRuntimePlugins
// must not panic when the containerd config file is missing, empty, or fully
// commented out (all top-level keys absent).
func TestFindAdditionalRuntimePluginsHandlesEmptyConfig(t *testing.T) {
	cases := map[string]string{
		"empty file":         "",
		"fully commented":    "# disabled_plugins = [\"cri\"]\n# [grpc]\n# address = \"/run/containerd/containerd.sock\"\n",
		"no plugins section": "version = 2\n",
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.toml")
			assert.NilError(t, os.WriteFile(path, []byte(contents), 0o644))

			count := 0
			for range findAdditionalRuntimePluginsAt(path) {
				count++
			}
			assert.Equal(t, count, 0)
		})
	}
}

func TestFindAdditionalRuntimePluginsHandlesMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.toml")
	count := 0
	for range findAdditionalRuntimePluginsAt(missing) {
		count++
	}
	assert.Equal(t, count, 0)
}

func TestFindAdditionalRuntimePluginsReturnsConfiguredRuntimes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	contents := `version = 2
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc]
runtime_type = "io.containerd.runc.v2"
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.kata]
runtime_type = "io.containerd.kata.v2"
`
	assert.NilError(t, os.WriteFile(path, []byte(contents), 0o644))

	got := map[string]bool{}
	for name := range findAdditionalRuntimePluginsAt(path) {
		got[name] = true
	}
	assert.Assert(t, got["runc"], "expected runc, got %v", got)
	assert.Assert(t, got["kata"], "expected kata, got %v", got)
}
