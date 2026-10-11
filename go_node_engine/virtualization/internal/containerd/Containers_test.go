package containerd

import (
	"crypto/sha256"
	"encoding/base32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	containerdidentifiers "github.com/containerd/containerd/identifiers"
	"gotest.tools/v3/assert"
)

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

func expectedContainerIdHash(taskId string) string {
	encoding := base32.NewEncoding(
		"abcdefghijklmnopqrstuvwxyz234567",
	).WithPadding(base32.NoPadding)

	hashBytes := sha256.Sum256([]byte(taskId))
	return encoding.EncodeToString(hashBytes[:])[:20]
}

func TestConvertTaskIdToContainerId(t *testing.T) {
	truncatesAtSeparator := strings.Repeat("a", 54) +
		"-" + strings.Repeat("b", 20)

	tests := []struct {
		name       string
		taskId     string
		wantPrefix string
	}{
		{
			name:       "ordinary identifier",
			taskId:     "task-01",
			wantPrefix: "task.01",
		},
		{
			name:       "uppercase characters",
			taskId:     "Task_ID",
			wantPrefix: "task.id",
		},
		{
			name:       "repeated dots",
			taskId:     "foo..bar",
			wantPrefix: "foo.bar",
		},
		{
			name:       "mixed consecutive separators",
			taskId:     "foo._-bar",
			wantPrefix: "foo.bar",
		},
		{
			name:       "leading separators",
			taskId:     ".-_foo",
			wantPrefix: "foo",
		},
		{
			name:       "trailing separators",
			taskId:     "foo_-.",
			wantPrefix: "foo",
		},
		{
			name:       "leading trailing and repeated separators",
			taskId:     ".-_Foo_01_-. ",
			wantPrefix: "foo.01",
		},
		{
			name:       "invalid characters removed",
			taskId:     "foo/@bar",
			wantPrefix: "foobar",
		},
		{
			name:       "unicode characters removed",
			taskId:     "Täst-🔥ID",
			wantPrefix: "tst.id",
		},
		{
			name:       "maximum readable prefix",
			taskId:     strings.Repeat("a", 55),
			wantPrefix: strings.Repeat("a", 55),
		},
		{
			name:       "long readable prefix truncated",
			taskId:     strings.Repeat("a", 100),
			wantPrefix: strings.Repeat("a", 55),
		},
		{
			name:       "truncation lands on separator",
			taskId:     truncatesAtSeparator,
			wantPrefix: strings.Repeat("a", 54),
		},
		{
			name:       "only separators",
			taskId:     "._-",
			wantPrefix: "",
		},
		{
			name:       "only invalid characters",
			taskId:     "🔥///",
			wantPrefix: "",
		},
		{
			name:       "empty task ID",
			taskId:     "",
			wantPrefix: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertTaskIdToContainerId(tt.taskId)

			want := expectedContainerIdHash(tt.taskId)
			if tt.wantPrefix != "" {
				want = tt.wantPrefix + "." + want
			}

			if got != want {
				t.Fatalf(
					"convertTaskIdToContainerId(%q) = %q; want %q",
					tt.taskId,
					got,
					want,
				)
			}

			if len(got) > containerIdMaxLen {
				t.Errorf(
					"generated ID is %d bytes; maximum is %d",
					len(got),
					containerIdMaxLen,
				)
			}

			if err := containerdidentifiers.Validate(got); err != nil {
				t.Errorf(
					"generated ID %q is rejected by containerd: %v",
					got,
					err,
				)
			}
		})
	}
}

func TestConvertTaskIdToContainerIdIsDeterministic(t *testing.T) {
	const taskId = "Task._-with/invalid🔥characters"

	first := convertTaskIdToContainerId(taskId)
	second := convertTaskIdToContainerId(taskId)

	if first != second {
		t.Fatalf(
			"conversion is not deterministic: first %q, second %q",
			first,
			second,
		)
	}
}

func TestConvertTaskIdToContainerIdHashDistinguishesNormalizedInputs(
	t *testing.T,
) {
	tests := []struct {
		name   string
		first  string
		second string
	}{
		{
			name:   "repeated separator",
			first:  "foo..bar",
			second: "foo.bar",
		},
		{
			name:   "uppercase",
			first:  "FOO",
			second: "foo",
		},
		{
			name:   "invalid characters",
			first:  "foo/@bar",
			second: "foobar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := convertTaskIdToContainerId(tt.first)
			second := convertTaskIdToContainerId(tt.second)

			if first == second {
				t.Fatalf(
					"different task IDs %q and %q generated the same ID %q",
					tt.first,
					tt.second,
					first,
				)
			}
		})
	}
}

func FuzzConvertTaskIdToContainerId(f *testing.F) {
	seeds := []string{
		"",
		"task",
		"Task_ID",
		"foo..bar",
		".-_foo_01_-.",
		"🔥///",
		strings.Repeat("a", 100),
		strings.Repeat("a", 54) + "-" + strings.Repeat("b", 20),
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, taskId string) {
		got := convertTaskIdToContainerId(taskId)

		if got == "" {
			t.Fatal("generated an empty container ID")
		}

		if len(got) > containerIdMaxLen {
			t.Fatalf(
				"generated ID %q is %d bytes; maximum is %d",
				got,
				len(got),
				containerIdMaxLen,
			)
		}

		if err := containerdidentifiers.Validate(got); err != nil {
			t.Fatalf(
				"generated ID %q is rejected by containerd: %v",
				got,
				err,
			)
		}

		if second := convertTaskIdToContainerId(taskId); second != got {
			t.Fatalf(
				"conversion is not deterministic: first %q, second %q",
				got,
				second,
			)
		}
	})
}
