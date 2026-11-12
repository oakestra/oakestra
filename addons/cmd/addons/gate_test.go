package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/oakestra/oakestra/addons/internal/auth"
)

// keyServer serves a public key in jwt_generator's format and counts requests.
type keyServer struct {
	mu    sync.Mutex
	key   *rsa.PublicKey
	calls int
}

func (k *keyServer) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.calls++
	_ = json.NewEncoder(w).Encode(map[string]string{"public_key": pemOf(k.key)})
}

func (k *keyServer) set(key *rsa.PublicKey) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.key = key
}

func (k *keyServer) count() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.calls
}

func pemOf(k *rsa.PublicKey) string {
	der, _ := x509.MarshalPKIXPublicKey(k)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func genKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func signed(t *testing.T, k *rsa.PrivateKey) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": "u", "type": "access", "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString(k)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// setup loads the first key the way main does and moves the gate's clock past the refresh
// interval, so any refresh the gate is allowed to do happens on the next failing token.
func setup(t *testing.T, logger *slog.Logger) (*auth.Gate, http.Handler) {
	t.Helper()
	loader := auth.KeyLoaderFromEnv()
	gate := newGate(loader, logger)
	key, err := loader.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	gate.Now = func() time.Time { return now }
	gate.SetKey(key)
	now = now.Add(time.Hour)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", gate.RequireUser(), func(c *gin.Context) { c.Status(http.StatusOK) })
	return gate, r
}

func call(h http.Handler, tok string) int {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func clearKeyEnv(t *testing.T) {
	for _, k := range []string{"JWT_PUBLIC_KEY", "JWT_PUBLIC_KEY_URL", "JWT_GENERATOR_URL", "JWT_GENERATOR_PORT"} {
		t.Setenv(k, "")
	}
}

func TestRemoteKeyURLIsNotReplacedOnFailure(t *testing.T) {
	clearKeyEnv(t)
	first, attacker := genKey(t), genKey(t)
	srv := &keyServer{key: &first.PublicKey}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	t.Setenv("JWT_PUBLIC_KEY_URL", ts.URL+"/api/v1/marketplace/auth/public-key")

	var logs bytes.Buffer
	_, h := setup(t, slog.New(slog.NewTextHandler(&logs, nil)))
	if !strings.Contains(logs.String(), "JWT_PUBLIC_KEY") || !strings.Contains(logs.String(), "level=WARN") {
		t.Errorf("no pinning recommendation logged: %s", logs.String())
	}

	// An on-path attacker now answers key requests and sends a token signed with their key.
	srv.set(&attacker.PublicKey)
	if code := call(h, signed(t, attacker)); code != 401 {
		t.Fatalf("attacker token: %d", code)
	}
	if code := call(h, signed(t, attacker)); code != 401 || srv.count() != 1 {
		t.Fatalf("key was refetched: code %d, fetches %d", code, srv.count())
	}
	if code := call(h, signed(t, first)); code != 200 {
		t.Fatalf("original key no longer works: %d", code)
	}
}

func TestGeneratorKeyIsRefreshed(t *testing.T) {
	clearKeyEnv(t)
	first, second := genKey(t), genKey(t)
	srv := &keyServer{key: &first.PublicKey}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	host, port, _ := strings.Cut(strings.TrimPrefix(ts.URL, "http://"), ":")
	t.Setenv("JWT_GENERATOR_URL", host)
	t.Setenv("JWT_GENERATOR_PORT", port)

	_, h := setup(t, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	// jwt_generator restarted with a new key pair.
	srv.set(&second.PublicKey)
	if code := call(h, signed(t, second)); code != 200 || srv.count() != 2 {
		t.Fatalf("code %d, fetches %d", code, srv.count())
	}
}

func TestStaticKeyIsNeverFetched(t *testing.T) {
	clearKeyEnv(t)
	k, other := genKey(t), genKey(t)
	t.Setenv("JWT_PUBLIC_KEY", pemOf(&k.PublicKey))
	loader := auth.KeyLoaderFromEnv()
	if loader.Source != auth.KeySourceStatic || loader.RefreshAllowed() {
		t.Fatalf("loader = %+v", loader)
	}
	_, h := setup(t, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if code := call(h, signed(t, other)); code != 401 {
		t.Fatalf("other key: %d", code)
	}
	if code := call(h, signed(t, k)); code != 200 {
		t.Fatalf("static key: %d", code)
	}
}
