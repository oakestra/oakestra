// Package api is the REST API of the addons engine. It only records intent (install, update,
// approve, delete); containers are created by the reconcile loop.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/oakestra/oakestra/addons/internal/addons/grant"
	"github.com/oakestra/oakestra/addons/internal/addons/marketplaceclient"
	"github.com/oakestra/oakestra/addons/internal/addons/store"
	"github.com/oakestra/oakestra/addons/internal/auth"
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
	if os.Getenv(gin.EnvGinMode) == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	// Redirects turn a POST into a GET on most clients and drop the body, so a missing or
	// extra slash must be a plain 404 (or an explicit route below) rather than a 301/307.
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false
	// Without this, wrong methods would get the 404 handler instead of 405.
	r.HandleMethodNotAllowed = true
	// Match on the escaped path, so an encoded slash stays inside its segment instead of
	// splitting it in two.
	r.UseRawPath = true
	r.UnescapePathValues = true

	routes := &routeTable{}
	// Global middleware also runs for the 404 and 405 handlers, so every response carries the
	// CORS headers and an OPTIONS preflight is answered whatever the path.
	r.Use(recoverPanics(s.log), logRequests(s.log), s.cors(), limitBody(MaxBodyBytes))
	r.NoRoute(func(c *gin.Context) { writeError(c.Writer, http.StatusNotFound, "not found") })
	r.NoMethod(routes.methodNotAllowed)

	handle := func(method, path string, handlers ...gin.HandlerFunc) {
		routes.add(method, path)
		r.Handle(method, path, handlers...)
		if method == http.MethodGet {
			// Answer HEAD on every GET route, as net/http does.
			routes.add(http.MethodHead, path)
			r.Handle(http.MethodHead, path, handlers...)
		}
	}
	user, admin := cfg.Gate.RequireUser(), cfg.Gate.RequireAdmin()

	handle(http.MethodGet, "/", health)
	// The dashboard may send either form.
	for _, p := range []string{"/api/v1/addons", "/api/v1/addons/"} {
		handle(http.MethodGet, p, user, s.list)
		handle(http.MethodPost, p, admin, s.install)
	}
	handle(http.MethodGet, "/api/v1/addons/:id", user, s.get)
	handle(http.MethodDelete, "/api/v1/addons/:id", admin, s.remove)
	handle(http.MethodPost, "/api/v1/addons/:id/update", admin, s.update)
	handle(http.MethodPost, "/api/v1/addons/:id/approve", admin, s.approve)
	// No auth on purpose, the dashboard calls this without a token.
	for _, p := range []string{"/api/v1/ui-addons", "/api/v1/ui-addons/"} {
		handle(http.MethodGet, p, s.uiAddons)
	}
	return r
}

func health(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(c.Writer, "ok")
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

func limitBody(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
	}
}

// logRequests logs method, path and status only. Headers and bodies are left out because they
// carry tokens, and the query string to stay in line with the marketplace.
func logRequests(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		status := c.Writer.Status()
		if c.Request.URL.Path == "/" && status == http.StatusOK {
			// Health checks would drown everything else.
			return
		}
		log.Info("request", "method", c.Request.Method, "path", c.Request.URL.Path, "status", status, "duration_ms", time.Since(start).Milliseconds())
	}
}

// recoverPanics replaces gin.Recovery's default output, which dumps request headers (and with
// them the bearer token), with a log line through the service logger.
func recoverPanics(log *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, err any) {
		log.Error("handler panicked", "method", c.Request.Method, "path", c.Request.URL.Path, "panic", err)
		writeError(c.Writer, http.StatusInternalServerError, "Internal server error")
		c.Abort()
	})
}

// routeTable lets the 405 handler send an Allow header like net/http's mux does, which Gin
// doesn't provide.
type routeTable struct {
	routes []route
}

type route struct {
	method   string
	segments []string
}

func (t *routeTable) add(method, path string) {
	t.routes = append(t.routes, route{method: method, segments: strings.Split(path, "/")})
}

func (t *routeTable) methodNotAllowed(c *gin.Context) {
	segs := strings.Split(c.Request.URL.Path, "/")
	var allowed []string
	for _, rt := range t.routes {
		if len(rt.segments) != len(segs) {
			continue
		}
		match := true
		for i, s := range rt.segments {
			if strings.HasPrefix(s, ":") {
				match = segs[i] != ""
			} else {
				match = s == segs[i]
			}
			if !match {
				break
			}
		}
		if match && !slices.Contains(allowed, rt.method) {
			allowed = append(allowed, rt.method)
		}
	}
	slices.Sort(allowed)
	c.Header("Allow", strings.Join(allowed, ", "))
	writeError(c.Writer, http.StatusMethodNotAllowed, "method not allowed")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"message": msg})
}

// decodeBody reads a JSON object into v and writes the error response itself on failure.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "Request body is too large")
			return false
		}
		writeError(w, http.StatusBadRequest, "Request body must be a JSON object")
		return false
	}
	return true
}
