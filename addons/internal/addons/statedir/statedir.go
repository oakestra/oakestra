// Package statedir manages the engine's private state directory (ADDONS_STATE_DIR): the grant
// signing key and the instance id. Neither may live in Mongo, since the point of both is to be
// out of reach of whoever can write to the database or share the docker daemon.
package statedir

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/oakestra/oakestra/addons/internal/addons/grant"
)

// DefaultDir is used when ADDONS_STATE_DIR is unset.
const DefaultDir = "/var/lib/oakestra-addons"

const (
	// GrantKeyFile holds grant.KeySize random bytes.
	GrantKeyFile = "grant.key"
	// InstanceIDFile holds the instance id as lowercase hex.
	InstanceIDFile = "instance.id"

	instanceIDBytes = 16
)

var instanceIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Ensure creates dir if it is missing and restricts it to mode 0700. An existing directory is
// tightened too: a docker VOLUME or a bind mount usually starts out as 0755.
func Ensure(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("restricting state directory: %w", err)
	}
	return nil
}

// LoadOrCreateGrantKey returns the grant key in dir, creating it on first start. An existing
// file that can't be read or has the wrong length is an error rather than a reason to make a
// new key: a new key would quietly invalidate every approval, and a truncated one points at
// something being wrong with the volume.
func LoadOrCreateGrantKey(dir string) (grant.Key, error) {
	path := filepath.Join(dir, GrantKeyFile)
	raw, err := loadOrCreate(path, func() ([]byte, error) {
		b := make([]byte, grant.KeySize)
		_, err := rand.Read(b)
		return b, err
	})
	if err != nil {
		return grant.Key{}, err
	}
	key, err := grant.NewKey(raw)
	if err != nil {
		return grant.Key{}, fmt.Errorf("%s: %w; refusing to start (restore the file, or delete it and re-approve every addon)", path, err)
	}
	return key, nil
}

// LoadOrCreateInstanceID returns the instance id in dir, creating it on first start. It tells
// apart two engines of the same plane sharing one docker daemon.
func LoadOrCreateInstanceID(dir string) (string, error) {
	path := filepath.Join(dir, InstanceIDFile)
	raw, err := loadOrCreate(path, func() ([]byte, error) {
		b := make([]byte, instanceIDBytes)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		return []byte(hex.EncodeToString(b) + "\n"), nil
	})
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(raw))
	if !instanceIDPattern.MatchString(id) {
		return "", fmt.Errorf("%s does not hold a valid instance id; refusing to start", path)
	}
	return id, nil
}

// loadOrCreate reads path, or creates it with O_EXCL and mode 0600 if it doesn't exist. O_EXCL
// makes two engines starting at once on the same directory agree on one file.
func loadOrCreate(path string, generate func() ([]byte, error)) ([]byte, error) {
	if b, err := readFile(path); !errors.Is(err, fs.ErrNotExist) {
		return b, err
	}
	content, err := generate()
	if err != nil {
		return nil, fmt.Errorf("generating %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return readFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("creating %s: %w", path, err)
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("writing %s: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("writing %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("writing %s: %w", path, err)
	}
	return content, nil
}

func readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		return nil, fmt.Errorf("%s exists but can't be read: %w; refusing to start", path, err)
	}
	defer func() { _ = f.Close() }()
	// Both files are tiny; the limit only guards against pointing the dir at something odd.
	b, err := io.ReadAll(io.LimitReader(f, 4096))
	if err != nil {
		return nil, fmt.Errorf("%s exists but can't be read: %w; refusing to start", path, err)
	}
	return b, nil
}
