package statedir

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/oakestra/oakestra/addons/internal/addons"
)

func TestGrantKeyCreatedOnceWithPrivateModes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(dir); err != nil {
		t.Fatal(err)
	}
	k1, err := LoadOrCreateGrantKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	k2, err := LoadOrCreateGrantKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := addons.Installed{ID: "x", AddonID: "y"}
	a.Signature = k1.Sign(a, "root")
	if k2.Verify(a, "root") != nil {
		t.Error("second start loaded a different key")
	}

	st, err := os.Stat(filepath.Join(dir, GrantKeyFile))
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() != 32 || st.Mode().Perm() != 0o600 {
		t.Errorf("key file: size %d mode %v", st.Size(), st.Mode().Perm())
	}
	if st, _ := os.Stat(dir); st.Mode().Perm() != 0o700 {
		t.Errorf("dir mode %v", st.Mode().Perm())
	}
}

func TestGrantKeyRefusesBadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, GrantKeyFile)
	if err := os.WriteFile(path, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateGrantKey(dir); err == nil {
		t.Fatal("short key accepted")
	}
	if b, _ := os.ReadFile(path); string(b) != "short" {
		t.Error("existing file was replaced")
	}

	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		if err := os.WriteFile(path, make([]byte, 32), 0o000); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o000); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadOrCreateGrantKey(dir); err == nil {
			t.Fatal("unreadable key accepted")
		}
	}
}

func TestInstanceID(t *testing.T) {
	dir := t.TempDir()
	a, err := LoadOrCreateInstanceID(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadOrCreateInstanceID(dir)
	if err != nil || a != b || len(a) != 32 {
		t.Fatalf("%q %q %v", a, b, err)
	}
	other, _ := LoadOrCreateInstanceID(t.TempDir())
	if other == a {
		t.Error("instance ids are not random")
	}
	if err := os.WriteFile(filepath.Join(dir, InstanceIDFile), []byte("../../etc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateInstanceID(dir); err == nil {
		t.Error("malformed instance id accepted")
	}
}
