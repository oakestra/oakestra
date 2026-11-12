package auth

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/oakestra/oakestra/addons/internal/httpx"
)

// ginClaimsKey is the gin context key the Gin middleware stores the claims under.
const ginClaimsKey = "oakestra.auth.claims"

// GinClaims returns the claims the gate's middleware stored.
func GinClaims(c *gin.Context) (*Claims, bool) {
	v, ok := c.Get(ginClaimsKey)
	if !ok {
		return nil, false
	}
	claims, ok := v.(*Claims)
	return claims, ok
}

// DefaultKeyRefreshInterval limits how often a failed verification may re-fetch the key.
const DefaultKeyRefreshInterval = 30 * time.Second

// Gate wraps a Verifier for services that load their key asynchronously. It adds two things
// the verifier doesn't do by itself:
//
//   - Until the first key is loaded, protected endpoints answer 503 with Retry-After instead
//     of 401, so clients can tell "try again shortly" from "your token is bad".
//   - jwt_generator makes a new key pair on every restart unless one is configured, which would
//     leave the service rejecting every fresh token until it restarts too. A token that fails
//     verification triggers a re-fetch of the key, at most once per interval so a flood of bad
//     tokens can't turn into a flood of requests against jwt_generator.
type Gate struct {
	verifier *Verifier
	fetch    func(context.Context) (*rsa.PublicKey, error)
	log      *slog.Logger

	// Interval defaults to DefaultKeyRefreshInterval. Set both fields before serving requests.
	Interval time.Duration
	// Now defaults to time.Now and exists so tests can move the clock.
	Now func() time.Time

	loaded atomic.Bool
	// key mirrors the verifier's key so it can be served to other services.
	key atomic.Pointer[rsa.PublicKey]
	// mu serializes refreshes, so concurrent failures wait for one fetch instead of each
	// deciding the interval hasn't passed and verifying against the stale key.
	mu          sync.Mutex
	lastRefresh time.Time
}

// NewGate returns a gate without a key. fetch makes one attempt to load the key (e.g.
// KeyLoader.Fetch); it may be nil when keys are only set with SetKey.
func NewGate(fetch func(context.Context) (*rsa.PublicKey, error), log *slog.Logger) *Gate {
	if log == nil {
		log = slog.Default()
	}
	return &Gate{
		verifier: NewVerifier(nil),
		fetch:    fetch,
		Interval: DefaultKeyRefreshInterval,
		Now:      time.Now,
		log:      log,
	}
}

// SetKey installs a verification key and opens the protected endpoints.
func (g *Gate) SetKey(key *rsa.PublicKey) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.verifier.SetKey(key)
	g.key.Store(key)
	// A key that was just loaded is current, so the next failed token shouldn't refetch it.
	g.lastRefresh = g.Now()
	g.loaded.Store(true)
}

// PublicKeyPEM returns the key currently used for verification in PKIX PEM form, the format
// jwt_generator serves, or false while none is loaded.
func (g *Gate) PublicKeyPEM() (string, bool) {
	key := g.key.Load()
	if key == nil {
		return "", false
	}
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return "", false
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), true
}

// Ready reports whether a key has been loaded.
func (g *Gate) Ready() bool { return g.loaded.Load() }

// RequireUser rejects requests without a valid access token with 401 and stores the claims
// for GinClaims.
func (g *Gate) RequireUser() gin.HandlerFunc {
	return g.require(nil)
}

// RequireAdmin is RequireUser plus a 403 for tokens without AdminRole.
func (g *Gate) RequireAdmin() gin.HandlerFunc {
	return g.require(func(c *Claims) bool { return c.IsAdmin() })
}

func (g *Gate) require(allowed func(*Claims) bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !g.loaded.Load() {
			c.Header("Retry-After", "5")
			httpx.WriteError(c.Writer, http.StatusServiceUnavailable, "authentication is not ready yet, the JWT public key hasn't been loaded")
			c.Abort()
			return
		}
		raw, ok := bearerToken(c.Request)
		if !ok {
			unauthorized(c.Writer, "Missing authentication token")
			c.Abort()
			return
		}
		claims, err := g.verifier.Verify(raw)
		if errors.Is(err, ErrInvalidToken) && g.maybeRefresh(c.Request.Context()) {
			claims, err = g.verifier.Verify(raw)
		}
		switch {
		case errors.Is(err, ErrNotAccess):
			unauthorized(c.Writer, "Only access token allowed")
			c.Abort()
			return
		case err != nil:
			// The parser's error text can echo token details, so keep the response generic.
			unauthorized(c.Writer, "Invalid authentication token")
			c.Abort()
			return
		}
		if allowed != nil && !allowed(claims) {
			// Same message as system_manager's not_authorized.
			httpx.WriteError(c.Writer, http.StatusForbidden, "You have not enough permissions!")
			c.Abort()
			return
		}
		c.Set(ginClaimsKey, claims)
		c.Next()
	}
}

// maybeRefresh re-fetches the key unless that happened within the interval, and reports
// whether a new key was installed.
func (g *Gate) maybeRefresh(ctx context.Context) bool {
	if g.fetch == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.Now()
	if now.Sub(g.lastRefresh) < g.Interval {
		return false
	}
	g.lastRefresh = now
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	key, err := g.fetch(ctx)
	if err != nil {
		g.log.Warn("re-fetching the JWT public key failed", "error", err)
		return false
	}
	g.verifier.SetKey(key)
	g.key.Store(key)
	g.log.Info("re-fetched the JWT public key after a failed verification")
	return true
}
