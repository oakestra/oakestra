package grant

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/oakestra/oakestra/addons/internal/addons"
	"github.com/oakestra/oakestra/addons/internal/definition"
)

func key(t *testing.T, fill byte) Key {
	t.Helper()
	k, err := NewKey(bytes.Repeat([]byte{fill}, KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func record() addons.Installed {
	d := definition.Addon{
		Name: "insights", Source: definition.OfficialSource, Version: "1.0.0",
		Permissions: definition.Permissions{HostPorts: []int{8080}},
		Services: []definition.Service{{
			ServiceName: "web", Image: "ghcr.io/x/web@sha256:" + fmt.Sprintf("%064x", 1),
			Ports: map[string]string{"80": "8080"},
		}},
	}
	return addons.Installed{
		ID: "000000000000000000000001", AddonID: d.ID(), Version: d.Version,
		Definition: d, Granted: d.Permissions,
	}
}

func TestSignVerify(t *testing.T) {
	k := key(t, 1)
	a := record()
	a.Signature = k.Sign(a, "root")
	if err := k.Verify(a, "root"); err != nil {
		t.Fatalf("honest record: %v", err)
	}

	// Normalization and canonical permission order don't matter, nil and empty are the same.
	b := a
	b.Definition.Normalize()
	b.Granted.HostPorts = []int{8080, 8080}
	if err := k.Verify(b, "root"); err != nil {
		t.Errorf("normalized record: %v", err)
	}

	for name, mutate := range map[string]func(*addons.Installed){
		"id":       func(x *addons.Installed) { x.ID = "000000000000000000000002" },
		"addon id": func(x *addons.Installed) { x.AddonID = "other" },
		"version":  func(x *addons.Installed) { x.Version = "1.0.1" },
		"snapshot": func(x *addons.Installed) { x.Definition.Services[0].Image = "evil" },
		"grant":    func(x *addons.Installed) { x.Granted.DockerSocket = true },
		"garbage":  func(x *addons.Installed) { x.Signature = "v1:zz" },
		"version prefix": func(x *addons.Installed) {
			x.Signature = "v2" + x.Signature[2:]
		},
	} {
		c := record()
		c.Signature = a.Signature
		mutate(&c)
		if err := k.Verify(c, "root"); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := k.Verify(a, "cluster"); !errors.Is(err, ErrInvalid) {
		t.Errorf("other plane: %v", err)
	}
	if err := key(t, 2).Verify(a, "root"); !errors.Is(err, ErrInvalid) {
		t.Errorf("other key: %v", err)
	}
	if err := (Key{}).Verify(a, "root"); !errors.Is(err, ErrInvalid) {
		t.Errorf("zero key: %v", err)
	}
	a.Signature = ""
	if err := k.Verify(a, "root"); !errors.Is(err, ErrMissing) {
		t.Errorf("unsigned: %v", err)
	}
}

func TestEncodeIsUnambiguous(t *testing.T) {
	a, b := record(), record()
	a.AddonID, a.Version = "ab", "c"
	b.AddonID, b.Version = "a", "bc"
	if bytes.Equal(Encode(a, "root"), Encode(b, "root")) {
		t.Fatal("field boundaries are not encoded")
	}
}

func TestKeyIsRedacted(t *testing.T) {
	k := key(t, 'A')
	for _, s := range []string{fmt.Sprint(k), fmt.Sprintf("%v %+v %#v %s", k, k, k, k)} {
		if bytes.Contains([]byte(s), []byte("AAAA")) || bytes.Contains([]byte(s), []byte("4141")) {
			t.Errorf("key material in %q", s)
		}
	}
	if _, err := NewKey([]byte("short")); err == nil {
		t.Error("short key accepted")
	}
}

func TestEncodeEmbedsCanonicalDefinition(t *testing.T) {
	a := record()
	enc := Encode(a, "root")
	if !bytes.Contains(enc, definition.CanonicalJSON(a.Definition)) {
		t.Fatal("Encode does not carry the canonical definition")
	}
	// Records already in the database carry signatures over these bytes, so they are pinned.
	sum := sha256.Sum256(enc)
	const want = "320f949c6e83f504bab7275079206ddbb8f6439213ffd6bca0ba37fff99735bf"
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("Encode output changed: %s", got)
	}
}
