package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var (
	testKey  = mustKey()
	otherKey = mustKey()
	testNow  = time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
)

func mustKey() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}

func publicPEM(t *testing.T, k *rsa.PublicKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

// tokenClaims mimics what flask_jwt_extended produces for system_manager.
func tokenClaims(roles ...string) jwt.MapClaims {
	return jwt.MapClaims{
		"sub":          "alice",
		"type":         "access",
		"fresh":        false,
		"jti":          "5b6c",
		"iat":          testNow.Add(-time.Minute).Unix(),
		"nbf":          testNow.Add(-time.Minute).Unix(),
		"exp":          testNow.Add(9 * time.Minute).Unix(),
		"user":         "alice",
		"roles":        roles,
		"organization": "64f0c0ffee",
	}
}

func sign(t *testing.T, claims jwt.MapClaims, mutate func(jwt.MapClaims)) string {
	t.Helper()
	if mutate != nil {
		mutate(claims)
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(testKey)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newTestVerifier() *Verifier {
	v := NewVerifier(&testKey.PublicKey)
	v.now = func() time.Time { return testNow }
	return v
}

// newTestGate returns a loaded gate whose verifier runs at testNow.
func newTestGate() *Gate {
	g := NewGate(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	g.verifier.now = func() time.Time { return testNow }
	g.SetKey(&testKey.PublicKey)
	return g
}

func TestMiddleware(t *testing.T) {
	admin := sign(t, tokenClaims("Admin"), nil)
	user := sign(t, tokenClaims("Application_Provider"), nil)

	hsConfusion := func() string {
		// The classic key confusion attack: HS256 with the public key PEM as the secret.
		s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, tokenClaims("Admin")).SignedString(publicPEM(t, &testKey.PublicKey))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}()
	none := func() string {
		s, err := jwt.NewWithClaims(jwt.SigningMethodNone, tokenClaims("Admin")).SignedString(jwt.UnsafeAllowNoneSignatureType)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}()
	otherSigner := func() string {
		s, err := jwt.NewWithClaims(jwt.SigningMethodRS256, tokenClaims("Admin")).SignedString(otherKey)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}()
	ps256 := func() string {
		s, err := jwt.NewWithClaims(jwt.SigningMethodPS256, tokenClaims("Admin")).SignedString(testKey)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}()

	tests := []struct {
		name      string
		header    string
		adminCode int
		userCode  int
	}{
		{"valid admin", "Bearer " + admin, 200, 200},
		{"lowercase scheme", "bearer " + admin, 200, 200},
		{"non-admin", "Bearer " + user, 403, 200},
		{"no roles claim", "Bearer " + sign(t, tokenClaims(), func(c jwt.MapClaims) { delete(c, "roles") }), 403, 200},
		{"missing header", "", 401, 401},
		{"wrong scheme", "Basic " + admin, 401, 401},
		{"bearer without token", "Bearer ", 401, 401},
		{"garbage", "Bearer not.a.jwt", 401, 401},
		{"refresh token", "Bearer " + sign(t, tokenClaims("Admin"), func(c jwt.MapClaims) { c["type"] = "refresh" }), 401, 401},
		{"missing type", "Bearer " + sign(t, tokenClaims("Admin"), func(c jwt.MapClaims) { delete(c, "type") }), 401, 401},
		{"file access token", "Bearer " + sign(t, tokenClaims("Admin"), func(c jwt.MapClaims) { c["file_access_token"] = true }), 401, 401},
		{"file access token as string", "Bearer " + sign(t, tokenClaims("Admin"), func(c jwt.MapClaims) { c["file_access_token"] = "yes" }), 401, 401},
		{"falsy file access token", "Bearer " + sign(t, tokenClaims("Admin"), func(c jwt.MapClaims) { c["file_access_token"] = false }), 200, 200},
		{"expired", "Bearer " + sign(t, tokenClaims("Admin"), func(c jwt.MapClaims) { c["exp"] = testNow.Add(-time.Minute).Unix() }), 401, 401},
		{"expired within leeway", "Bearer " + sign(t, tokenClaims("Admin"), func(c jwt.MapClaims) { c["exp"] = testNow.Add(-10 * time.Second).Unix() }), 200, 200},
		{"no expiry", "Bearer " + sign(t, tokenClaims("Admin"), func(c jwt.MapClaims) { delete(c, "exp") }), 401, 401},
		{"not yet valid", "Bearer " + sign(t, tokenClaims("Admin"), func(c jwt.MapClaims) { c["nbf"] = testNow.Add(time.Hour).Unix() }), 401, 401},
		{"no subject", "Bearer " + sign(t, tokenClaims("Admin"), func(c jwt.MapClaims) { delete(c, "sub") }), 401, 401},
		{"HS256 signed with the public key", "Bearer " + hsConfusion, 401, 401},
		{"alg none", "Bearer " + none, 401, 401},
		{"PS256", "Bearer " + ps256, 401, 401},
		{"signed by another key", "Bearer " + otherSigner, 401, 401},
	}

	gate := newTestGate()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, mw := range []struct {
				name string
				h    gin.HandlerFunc
				want int
			}{
				{"RequireAdmin", gate.RequireAdmin(), tt.adminCode},
				{"RequireUser", gate.RequireUser(), tt.userCode},
			} {
				rec := doGet(ginEngine(mw.h, nil), tt.header)
				want := mw.want
				if want == http.StatusOK {
					want = http.StatusNoContent
				}
				if rec.Code != want {
					t.Errorf("%s: status %d, want %d (%s)", mw.name, rec.Code, want, rec.Body)
				}
				if rec.Code != http.StatusNoContent {
					var body map[string]string
					if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["message"] == "" {
						t.Errorf("%s: error body %q is not {\"message\": ...}", mw.name, rec.Body)
					}
					if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
						t.Errorf("%s: content type %q", mw.name, ct)
					}
				}
			}
		})
	}

	var seen atomic.Pointer[Claims]
	doGet(ginEngine(gate.RequireAdmin(), &seen), "Bearer "+admin)
	c := seen.Load()
	if c == nil || c.Subject != "alice" || c.User != "alice" || c.Organization != "64f0c0ffee" || !c.IsAdmin() {
		t.Errorf("claims not passed through: %+v", c)
	}
}

func TestVerifierWithoutKey(t *testing.T) {
	v := NewVerifier(nil)
	if _, err := v.Verify(sign(t, tokenClaims("Admin"), nil)); err != ErrNoKey {
		t.Errorf("Verify without key = %v, want ErrNoKey", err)
	}
	v.SetKey(&testKey.PublicKey)
	v.now = func() time.Time { return testNow }
	if _, err := v.Verify(sign(t, tokenClaims("Admin"), nil)); err != nil {
		t.Errorf("Verify after SetKey = %v", err)
	}
}

func TestKeyLoader(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/key" {
			http.NotFound(w, r)
			return
		}
		// Fail twice, like a jwt_generator that is still starting.
		if calls.Add(1) <= 2 {
			http.Error(w, "starting", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"public_key": string(publicPEM(t, &testKey.PublicKey))})
	}))
	defer srv.Close()

	l := &KeyLoader{URL: srv.URL + "/key", RetryInterval: 10 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	key, err := l.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !key.Equal(&testKey.PublicKey) {
		t.Error("loaded the wrong key")
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", calls.Load())
	}
}

func TestKeyLoaderGivesUpWithContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"public_key": "not a pem"})
	}))
	defer srv.Close()

	l := &KeyLoader{URL: srv.URL + "/key", RetryInterval: 10 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := l.Load(ctx); err == nil {
		t.Fatal("Load succeeded with a broken key")
	}
}

func TestKeyLoaderFromEnv(t *testing.T) {
	t.Setenv("JWT_GENERATOR_URL", "jwt_generator")
	t.Setenv("JWT_GENERATOR_PORT", "10011")
	if got := KeyLoaderFromEnv().URL; got != "http://jwt_generator:10011/key" {
		t.Errorf("URL = %q", got)
	}
	t.Setenv("JWT_GENERATOR_URL", "")
	t.Setenv("JWT_GENERATOR_PORT", "")
	if got := KeyLoaderFromEnv().URL; got != "http://localhost:10011/key" {
		t.Errorf("default URL = %q", got)
	}
}

func TestParsePublicKeyPEM(t *testing.T) {
	pkcs1 := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&testKey.PublicKey)})
	for name, in := range map[string][]byte{"pkix": publicPEM(t, &testKey.PublicKey), "pkcs1": pkcs1} {
		k, err := ParsePublicKeyPEM(in)
		if err != nil || !k.Equal(&testKey.PublicKey) {
			t.Errorf("%s: %v", name, err)
		}
	}
	priv := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(testKey)})
	for name, in := range map[string][]byte{"empty": nil, "private key": priv, "junk": []byte("-----BEGIN PUBLIC KEY-----\nAAAA\n-----END PUBLIC KEY-----\n")} {
		if _, err := ParsePublicKeyPEM(in); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}

func TestKeyLoaderFromEnvPrecedence(t *testing.T) {
	pemKey := string(publicPEM(t, &testKey.PublicKey))
	t.Setenv("JWT_GENERATOR_URL", "jwt_generator")
	t.Setenv("JWT_GENERATOR_PORT", "10011")
	t.Setenv("JWT_PUBLIC_KEY_URL", "http://root:11102/api/v1/marketplace/auth/public-key")
	t.Setenv("JWT_PUBLIC_KEY", "")
	if got := KeyLoaderFromEnv(); got.URL != "http://root:11102/api/v1/marketplace/auth/public-key" || got.Static != nil {
		t.Errorf("JWT_PUBLIC_KEY_URL: %+v", got)
	}

	for name, value := range map[string]string{
		"multi-line":       pemKey,
		"escaped newlines": strings.ReplaceAll(pemKey, "\n", `\n`),
	} {
		t.Setenv("JWT_PUBLIC_KEY", value)
		l := KeyLoaderFromEnv()
		if l.Static == nil || !l.Static.Equal(&testKey.PublicKey) {
			t.Fatalf("%s: static key not used: %+v", name, l)
		}
		// No network: the URL fields are ignored and there is nothing to dial.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		key, err := l.Load(ctx)
		cancel()
		if err != nil || !key.Equal(&testKey.PublicKey) {
			t.Errorf("%s: Load = %v", name, err)
		}
		if key, err := l.Fetch(context.Background()); err != nil || !key.Equal(&testKey.PublicKey) {
			t.Errorf("%s: Fetch = %v", name, err)
		}
	}

	t.Setenv("JWT_PUBLIC_KEY", "not a key")
	l := KeyLoaderFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	if _, err := l.Load(ctx); err == nil || time.Since(start) > time.Second {
		t.Errorf("invalid static key: err %v after %s, want an immediate error", err, time.Since(start))
	}
	if _, err := l.Fetch(ctx); err == nil {
		t.Error("Fetch with an invalid static key succeeded")
	}
}
