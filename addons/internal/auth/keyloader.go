package auth

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// KeySource says where a KeyLoader gets its key, which decides whether re-fetching it on a
// failed verification is safe.
type KeySource string

const (
	// KeySourceStatic is a key from JWT_PUBLIC_KEY. There is nothing to refresh.
	KeySourceStatic KeySource = "static"
	// KeySourceURL is a key from JWT_PUBLIC_KEY_URL, typically the root marketplace on another
	// host over plain HTTP. Refreshing it would let anyone on the path trigger a refetch (by
	// sending a bad token) and answer it with their own key, so it is loaded once.
	KeySourceURL KeySource = "url"
	// KeySourceGenerator is jwt_generator on the same docker network. It makes a new key pair
	// on every restart, so refreshing is needed and the network path is local.
	KeySourceGenerator KeySource = "generator"
)

// KeyLoader fetches the public key from jwt_generator's GET /key, from another endpoint with
// the same response shape (the root marketplace serves one), or returns a fixed key.
type KeyLoader struct {
	// Source is set by KeyLoaderFromEnv. The zero value allows no refresh.
	Source KeySource
	// URL is the full key endpoint, e.g. "http://jwt_generator:10011/key".
	URL string
	// Static, when set, is returned by Load and Fetch without any network access. Key refreshes
	// then just reinstall the same key.
	Static *rsa.PublicKey
	// staticErr is set when JWT_PUBLIC_KEY was given but doesn't parse. Retrying can't fix a
	// bad environment, so Load fails at once instead of looping.
	staticErr error
	// Client defaults to a client with a 5 s timeout.
	Client *http.Client
	// RetryInterval defaults to 5 s, like system_manager.
	RetryInterval time.Duration
	// Logger defaults to slog.Default().
	Logger *slog.Logger
}

// maxKeyResponse is far above any PEM key; it only guards against a misbehaving endpoint.
const maxKeyResponse = 64 << 10

// KeyLoaderFromEnv builds a loader from the environment, in order of precedence:
//
//   - JWT_PUBLIC_KEY: a PEM public key, used as is. Literal "\n" sequences are accepted
//     because multi-line values are awkward in compose files.
//   - JWT_PUBLIC_KEY_URL: the full URL of an endpoint answering like jwt_generator's GET /key.
//     Services outside the root host use the root marketplace's
//     /api/v1/marketplace/auth/public-key, because jwt_generator itself isn't published.
//   - JWT_GENERATOR_URL and JWT_GENERATOR_PORT, with the same defaults as system_manager
//     (localhost, 10011).
func KeyLoaderFromEnv() *KeyLoader {
	if pemKey := os.Getenv("JWT_PUBLIC_KEY"); strings.TrimSpace(pemKey) != "" {
		if !strings.Contains(pemKey, "\n") {
			pemKey = strings.ReplaceAll(pemKey, "\\n", "\n")
		}
		key, err := ParsePublicKeyPEM([]byte(pemKey))
		if err != nil {
			return &KeyLoader{Source: KeySourceStatic, staticErr: fmt.Errorf("JWT_PUBLIC_KEY: %w", err)}
		}
		return &KeyLoader{Source: KeySourceStatic, Static: key}
	}
	if u := strings.TrimSpace(os.Getenv("JWT_PUBLIC_KEY_URL")); u != "" {
		return &KeyLoader{Source: KeySourceURL, URL: u}
	}
	host := os.Getenv("JWT_GENERATOR_URL")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("JWT_GENERATOR_PORT")
	if port == "" {
		port = "10011"
	}
	return &KeyLoader{Source: KeySourceGenerator, URL: fmt.Sprintf("http://%s:%s/key", host, port)}
}

// RefreshAllowed reports whether the key may be re-fetched after a token fails verification.
// Only jwt_generator qualifies, see KeySource. For the other sources a key rotation needs a
// restart.
func (l *KeyLoader) RefreshAllowed() bool {
	return l.Source == KeySourceGenerator
}

// NewGate returns a gate that re-fetches the key through l only when RefreshAllowed. A key from
// JWT_PUBLIC_KEY_URL usually crosses hosts over plain HTTP, where a refetch that anyone can
// trigger with a bad token would let an on-path attacker swap in their own key; it is loaded
// once and kept until the service restarts.
func (l *KeyLoader) NewGate(log *slog.Logger) *Gate {
	if log == nil {
		log = slog.Default()
	}
	if l.Logger == nil {
		l.Logger = log
	}
	if l.Source == KeySourceURL {
		log.Warn("the JWT public key is fetched from JWT_PUBLIC_KEY_URL without pinning; set JWT_PUBLIC_KEY to the root's public key instead, the URL is only trusted on first fetch and key rotation needs a restart",
			"url", l.URL)
	}
	if !l.RefreshAllowed() {
		return NewGate(nil, log)
	}
	return NewGate(l.Fetch, log)
}

// StartGate is NewGate plus a background Load. The key may arrive after the service is up
// (jwt_generator can start later), and until it does the gated endpoints answer 503.
func (l *KeyLoader) StartGate(ctx context.Context, log *slog.Logger) *Gate {
	gate := l.NewGate(log)
	go func() {
		key, err := l.Load(ctx)
		if err != nil {
			// Shutdown, or an invalid JWT_PUBLIC_KEY, which Load already logged.
			return
		}
		gate.SetKey(key)
		gate.log.Info("JWT public key loaded")
	}()
	return gate
}

// Load fetches the key, retrying until it succeeds or ctx is done. Services call it at startup;
// jwt_generator may still be starting, which is why failures are retried rather than fatal.
func (l *KeyLoader) Load(ctx context.Context) (*rsa.PublicKey, error) {
	interval := l.RetryInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	logger := l.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if l.staticErr != nil {
		// Callers typically drop the error and stay unauthenticated (503), so make it visible.
		logger.Error("invalid static JWT public key", "error", l.staticErr)
		return nil, l.staticErr
	}
	for {
		key, err := l.Fetch(ctx)
		if err == nil {
			return key, nil
		}
		logger.Warn("fetching JWT public key failed, retrying", "url", l.URL, "error", err, "retry_in", interval)
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("fetching JWT public key: %w (last error: %v)", ctx.Err(), err)
		case <-time.After(interval):
		}
	}
}

// Fetch makes a single attempt.
func (l *KeyLoader) Fetch(ctx context.Context) (*rsa.PublicKey, error) {
	if l.staticErr != nil {
		return nil, l.staticErr
	}
	if l.Static != nil {
		return l.Static, nil
	}
	client := l.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}
	var body struct {
		PublicKey string `json:"public_key"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxKeyResponse)).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return ParsePublicKeyPEM([]byte(body.PublicKey))
}

// ParsePublicKeyPEM parses an RSA public key in PKIX ("PUBLIC KEY", what jwt_generator emits)
// or PKCS#1 ("RSA PUBLIC KEY") form.
func ParsePublicKeyPEM(data []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	switch block.Type {
	case "PUBLIC KEY":
		key, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := key.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("expected an RSA key, got %T", key)
		}
		return rsaKey, nil
	case "RSA PUBLIC KEY":
		return x509.ParsePKCS1PublicKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unexpected PEM block %q", block.Type)
	}
}
