// Package grant signs what the admin approved for an installed addon.
//
// The record of an installed addon lives in Mongo. Re-checking the snapshot against the grant
// (as the reconcile loop does) only catches inconsistent edits: someone who can write to the
// database can rewrite the snapshot and the grant together, e.g. add the docker socket to both.
// The signature closes that gap. It is an HMAC-SHA256 with a key that only exists in the
// engine's state directory, so a record is trusted only if this engine wrote it.
//
// What a signature does not prevent: putting back an older signed state of the same record,
// i.e. a version of this addon the admin approved before. That stays within what was approved
// at some point, which is the property the permission model is about.
package grant

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/oakestra/oakestra/addons/internal/addons"
	"github.com/oakestra/oakestra/addons/internal/definition"
)

// KeySize is the length of a grant key in bytes.
const KeySize = 32

// version prefixes every signature, so the encoding can change without old signatures
// silently verifying against a new layout.
const version = "v1"

// domain separates these MACs from anything else the key might ever be used for.
const domain = "oakestra-addon-grant"

var (
	// ErrMissing means the record carries no signature, e.g. it was inserted directly into
	// the database.
	ErrMissing = errors.New("grant signature missing")
	// ErrInvalid means the signature doesn't match the record.
	ErrInvalid = errors.New("grant signature invalid")
)

// Key is a grant signing key. The zero Key signs nothing and verifies nothing.
type Key struct {
	b []byte
}

// NewKey wraps raw key material, which must be KeySize bytes.
func NewKey(b []byte) (Key, error) {
	if len(b) != KeySize {
		return Key{}, fmt.Errorf("grant key must be %d bytes, got %d", KeySize, len(b))
	}
	return Key{b: append([]byte(nil), b...)}, nil
}

// IsZero reports whether the key is unset.
func (k Key) IsZero() bool { return len(k.b) == 0 }

// String keeps the key material out of logs and %v output.
func (k Key) String() string { return "grant.Key(redacted)" }

// GoString does the same for %#v.
func (k Key) GoString() string { return k.String() }

// Sign returns the signature of a record for the given plane. It panics on the zero key,
// which would be a wiring bug: an unsigned install is useless since nothing would run it.
func (k Key) Sign(a addons.Installed, plane string) string {
	if k.IsZero() {
		panic("grant: Sign with an empty key")
	}
	return version + ":" + hex.EncodeToString(k.mac(a, plane))
}

// Verify checks the record's signature. It returns ErrMissing or ErrInvalid.
func (k Key) Verify(a addons.Installed, plane string) error {
	if a.Signature == "" {
		return ErrMissing
	}
	if k.IsZero() {
		return ErrInvalid
	}
	v, sig, ok := strings.Cut(a.Signature, ":")
	if !ok || v != version {
		return ErrInvalid
	}
	got, err := hex.DecodeString(sig)
	if err != nil {
		return ErrInvalid
	}
	if !hmac.Equal(got, k.mac(a, plane)) {
		return ErrInvalid
	}
	return nil
}

func (k Key) mac(a addons.Installed, plane string) []byte {
	m := hmac.New(sha256.New, k.b)
	_, _ = m.Write(Encode(a, plane))
	return m.Sum(nil)
}

// Encode is the canonical byte string that gets signed: every field length-prefixed, so no two
// different records encode the same. It covers the record id (a signed snapshot can't be
// copied onto another record), the plane (nor between the root and the cluster database),
// the addon id, the version, the definition snapshot and the granted permissions.
func Encode(a addons.Installed, plane string) []byte {
	var buf []byte
	field := func(b []byte) {
		buf = binary.BigEndian.AppendUint64(buf, uint64(len(b)))
		buf = append(buf, b...)
	}
	field([]byte(domain))
	field([]byte(version))
	field([]byte(a.ID))
	field([]byte(a.AddonID))
	field([]byte(plane))
	field([]byte(a.Version))
	field(definition.CanonicalJSON(a.Definition))
	field(canonicalPermissions(a.Granted))
	return buf
}

func canonicalPermissions(p definition.Permissions) []byte {
	out, err := json.Marshal(p.Canonical())
	if err != nil {
		panic(err)
	}
	return out
}
