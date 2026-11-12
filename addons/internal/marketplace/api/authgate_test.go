package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/oakestra/oakestra/addons/internal/auth"
)

func TestKeyRefreshAfterRotation(t *testing.T) {
	oldKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	newKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var fetches atomic.Int32
	gate := NewAdminGate(func(context.Context) (*rsa.PublicKey, error) {
		fetches.Add(1)
		return &newKey.PublicKey, nil
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	clock := time.Now()
	gate.Now = func() time.Time { return clock }
	gate.SetKey(&oldKey.PublicKey)

	gin.SetMode(gin.ReleaseMode)
	h := gin.New()
	h.POST("/", gate.RequireAdmin(), func(c *gin.Context) { c.Status(http.StatusOK) })
	tok := func() string {
		now := time.Now()
		s, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"sub": "alice", "type": "access", "roles": []string{"Admin"},
			"iat": now.Unix(), "exp": now.Add(time.Minute).Unix(),
		}).SignedString(newKey)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}()
	call := func(tok string) int {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	// Right after a load the key counts as current, so nothing is refetched.
	if code := call(tok); code != http.StatusUnauthorized || fetches.Load() != 0 {
		t.Fatalf("right after SetKey: code %d, fetches %d", code, fetches.Load())
	}
	clock = clock.Add(DefaultKeyRefreshInterval)
	if code := call(tok); code != http.StatusOK || fetches.Load() != 1 {
		t.Fatalf("after rotation: code %d, fetches %d", code, fetches.Load())
	}
	// Garbage tokens can't make it hammer jwt_generator.
	for range 5 {
		call("garbage")
	}
	if fetches.Load() != 1 {
		t.Errorf("refetched %d times within the interval", fetches.Load())
	}
	clock = clock.Add(DefaultKeyRefreshInterval)
	call("garbage")
	if fetches.Load() != 2 {
		t.Errorf("no refetch after the interval: %d", fetches.Load())
	}
}

func TestPublicKeyEndpoint(t *testing.T) {
	first, _ := rsa.GenerateKey(rand.Reader, 2048)
	second, _ := rsa.GenerateKey(rand.Reader, 2048)
	gate := NewAdminGate(func(context.Context) (*rsa.PublicKey, error) { return &second.PublicKey, nil },
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	clock := time.Now()
	gate.Now = func() time.Time { return clock }
	h := New(Config{Gate: gate, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/marketplace/auth/public-key")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("before load: %d", resp.StatusCode)
	}

	// auth.KeyLoader reads the endpoint, which is how remote addons managers use it.
	load := func() *rsa.PublicKey {
		t.Helper()
		key, err := (&auth.KeyLoader{URL: srv.URL + "/api/v1/marketplace/auth/public-key"}).Fetch(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	gate.SetKey(&first.PublicKey)
	if !load().Equal(&first.PublicKey) {
		t.Error("served key differs from the loaded one")
	}

	// A failed verification after the interval re-fetches; the endpoint follows.
	clock = clock.Add(DefaultKeyRefreshInterval)
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/marketplace/sources/x", nil)
	req.Header.Set("Authorization", "Bearer garbage")
	if resp, err := http.DefaultClient.Do(req); err == nil {
		_ = resp.Body.Close()
	}
	if !load().Equal(&second.PublicKey) {
		t.Error("endpoint still serves the old key after a refresh")
	}
}
