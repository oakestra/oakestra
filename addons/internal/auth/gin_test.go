package auth

import (
	"context"
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
)

func ginEngine(mw gin.HandlerFunc, seen *atomic.Pointer[Claims]) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	e := gin.New()
	e.GET("/", mw, func(c *gin.Context) {
		if cl, ok := GinClaims(c); ok {
			if cc, ok := ClaimsFromContext(c.Request.Context()); ok && cc == cl && seen != nil {
				seen.Store(cl)
			}
		}
		c.Status(http.StatusNoContent)
	})
	return e
}

func doGet(h http.Handler, authz string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// The Gin middleware must answer exactly like the net/http one, since the addons API and the
// marketplace are documented to behave the same.
func TestGinMiddlewareMatchesNetHTTP(t *testing.T) {
	v := newTestVerifier()
	admin := sign(t, tokenClaims("Admin"), nil)
	user := sign(t, tokenClaims("Application_Provider"), nil)
	refresh := sign(t, tokenClaims("Admin"), func(c jwt.MapClaims) { c["type"] = "refresh" })
	expired := sign(t, tokenClaims("Admin"), func(c jwt.MapClaims) { c["exp"] = testNow.Add(-time.Hour).Unix() })
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

	for _, authz := range []string{"", "Basic abc", "Bearer ", "Bearer garbage", "Bearer " + expired, "Bearer " + refresh, "Bearer " + user, "Bearer " + admin} {
		for name, pair := range map[string][2]any{
			"user":  {v.RequireUser(ok), v.GinRequireUser()},
			"admin": {v.RequireAdmin(ok), v.GinRequireAdmin()},
		} {
			want := doGet(pair[0].(http.Handler), authz)
			got := doGet(ginEngine(pair[1].(gin.HandlerFunc), nil), authz)
			if got.Code != want.Code || got.Body.String() != want.Body.String() ||
				got.Header().Get("WWW-Authenticate") != want.Header().Get("WWW-Authenticate") ||
				got.Header().Get("Content-Type") != want.Header().Get("Content-Type") {
				t.Errorf("%s with %q: gin %d %q %v, net/http %d %q %v", name, authz,
					got.Code, got.Body, got.Header(), want.Code, want.Body, want.Header())
			}
		}
	}
}

func TestGinClaimsReachHandler(t *testing.T) {
	var seen atomic.Pointer[Claims]
	rec := doGet(ginEngine(newTestVerifier().GinRequireAdmin(), &seen), "Bearer "+sign(t, tokenClaims("Admin"), nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d", rec.Code)
	}
	if c := seen.Load(); c == nil || c.Subject != "alice" {
		t.Errorf("claims not visible through both gin context and request context: %+v", c)
	}
}

func TestGinGate(t *testing.T) {
	var fetches atomic.Int32
	gate := NewGate(func(context.Context) (*rsa.PublicKey, error) {
		fetches.Add(1)
		return &testKey.PublicKey, nil
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	clock := testNow
	gate.Now = func() time.Time { return clock }
	// The gate's verifier uses wall-clock time, so mint tokens that are valid now.
	now := time.Now()
	tok := func(roles ...string) string {
		return sign(t, tokenClaims(roles...), func(c jwt.MapClaims) {
			c["iat"], c["nbf"], c["exp"] = now.Unix(), now.Unix(), now.Add(time.Hour).Unix()
		})
	}
	e := ginEngine(gate.RequireAdmin(), nil)

	rec := doGet(e, "Bearer "+tok("Admin"))
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") != "5" {
		t.Errorf("before a key is loaded: %d %v", rec.Code, rec.Header())
	}
	if gate.Ready() {
		t.Error("ready without a key")
	}

	gate.SetKey(&otherKey.PublicKey)
	if !gate.Ready() {
		t.Error("not ready after SetKey")
	}
	if _, ok := gate.PublicKeyPEM(); !ok {
		t.Error("no PEM after SetKey")
	}
	// A just-loaded key is current, so a bad token must not refetch.
	if rec := doGet(e, "Bearer "+tok("Admin")); rec.Code != http.StatusUnauthorized || fetches.Load() != 0 {
		t.Errorf("right after SetKey: %d, fetches %d", rec.Code, fetches.Load())
	}
	clock = clock.Add(DefaultKeyRefreshInterval)
	if rec := doGet(e, "Bearer "+tok("Admin")); rec.Code != http.StatusNoContent || fetches.Load() != 1 {
		t.Errorf("after rotation: %d, fetches %d", rec.Code, fetches.Load())
	}
	for range 5 {
		doGet(e, "Bearer garbage")
	}
	if fetches.Load() != 1 {
		t.Errorf("refetched %d times inside the interval", fetches.Load())
	}
	if rec := doGet(e, "Bearer "+tok("Application_Provider")); rec.Code != http.StatusForbidden {
		t.Errorf("non-admin: %d", rec.Code)
	}
	if rec := doGet(ginEngine(gate.RequireUser(), nil), "Bearer "+tok("Application_Provider")); rec.Code != http.StatusNoContent {
		t.Errorf("RequireUser with a plain user: %d", rec.Code)
	}
}
