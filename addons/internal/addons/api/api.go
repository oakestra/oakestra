// Package api is the REST API of the addons engine. It only records intent (install, update,
// approve, delete); containers are created by the reconcile loop.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/oakestra/oakestra/addons/internal/addons/grant"
	"github.com/oakestra/oakestra/addons/internal/addons/marketplaceclient"
	"github.com/oakestra/oakestra/addons/internal/addons/store"
	"github.com/oakestra/oakestra/addons/internal/auth"
	"github.com/oakestra/oakestra/addons/internal/httpx"
)

// MaxBodyBytes caps request bodies. The largest legitimate body is a name and a hash.
const MaxBodyBytes = 64 << 10

// Config wires the API.
type Config struct {
	Store       store.Store
	Marketplace marketplaceclient.Getter
	// Gate guards the user and admin endpoints; see auth.Gate for the 503-until-loaded and
	// key refresh behavior.
	Gate *auth.Gate
	// GrantKey signs every grant the API stores; the reconcile loop refuses unsigned ones.
	GrantKey grant.Key
	// Plane is "root" or "cluster". Only root serves UI addons.
	Plane string
	// CORSOrigin is the Access-Control-Allow-Origin value, "*" if empty. Tokens travel in a
	// header, not in cookies, so a wildcard doesn't expose credentials.
	CORSOrigin string
	Logger     *slog.Logger
}

type server struct {
	Config
	log *slog.Logger
	now func() time.Time
}

// New returns the HTTP handler. The Gin engine is only a router: it is served by the caller's
// http.Server, which owns the timeouts and graceful shutdown.
func New(cfg Config) http.Handler {
	if cfg.GrantKey.IsZero() {
		panic("api: GrantKey is required")
	}
	if cfg.Gate == nil {
		panic("api: Gate is required")
	}
	s := &server{Config: cfg, log: cfg.Logger, now: func() time.Time { return time.Now().UTC() }}
	if s.log == nil {
		s.log = slog.Default()
	}
	// CORS is global so the 404 and 405 responses carry it too, and an OPTIONS preflight is
	// answered whatever the path.
	r := httpx.NewRouter(s.log, s.cors(), httpx.LimitBody(MaxBodyBytes))
	user, admin := cfg.Gate.RequireUser(), cfg.Gate.RequireAdmin()

	// The dashboard may send either form.
	for _, p := range []string{"/api/v1/addons", "/api/v1/addons/"} {
		r.Handle(http.MethodGet, p, user, s.list)
		r.Handle(http.MethodPost, p, admin, s.install)
	}
	r.Handle(http.MethodGet, "/api/v1/addons/:id", user, s.get)
	r.Handle(http.MethodDelete, "/api/v1/addons/:id", admin, s.remove)
	r.Handle(http.MethodPost, "/api/v1/addons/:id/update", admin, s.update)
	r.Handle(http.MethodPost, "/api/v1/addons/:id/approve", admin, s.approve)
	// No auth on purpose, the dashboard calls this without a token.
	for _, p := range []string{"/api/v1/ui-addons", "/api/v1/ui-addons/"} {
		r.Handle(http.MethodGet, p, s.uiAddons)
	}
	return r
}

func (s *server) cors() gin.HandlerFunc {
	origin := s.CORSOrigin
	if origin == "" {
		origin = "*"
	}
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Expose-Headers", "Content-Type")
		if c.Request.Method == http.MethodOptions {
			h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// decodeBody reads a JSON object into v and writes the error response itself on failure.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	// An outdated or misspelled field would otherwise look like a body that confirmed nothing.
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, httpx.CodePayloadTooLarge, "Request body is too large")
			return false
		}
		// encoding/json has no typed error for this one.
		if field, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
			httpx.WriteError(w, http.StatusBadRequest, httpx.CodeValidationFailed, "Unknown field "+field+" in request body")
			return false
		}
		httpx.WriteError(w, http.StatusBadRequest, httpx.CodeValidationFailed, "Request body must be a JSON object")
		return false
	}
	return true
}
