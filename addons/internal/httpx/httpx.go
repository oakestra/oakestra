// Package httpx is the Gin setup shared by the addons and marketplace APIs: routing flags,
// request logging, panic recovery, 404/405 handling and the {"code", "message"} error body.
package httpx

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Router is a Gin engine that remembers its routes, so the 405 handler can send an Allow header
// like net/http's mux does, which Gin doesn't provide.
type Router struct {
	*gin.Engine
	routes []route
}

type route struct {
	method   string
	segments []string
}

// NewRouter returns an engine with "/" answering health checks. mw runs after panic recovery and
// request logging. Global middleware also runs for the 404 and 405 handlers, so CORS headers
// passed here reach every response.
//
// The engine is only a router: the caller's http.Server owns the timeouts and graceful shutdown.
func NewRouter(log *slog.Logger, mw ...gin.HandlerFunc) *Router {
	if os.Getenv(gin.EnvGinMode) == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := &Router{Engine: gin.New()}
	// Redirects turn a POST into a GET on most clients and drop the body, so a missing or
	// extra slash must be a plain 404 (or an explicit route) rather than a 301/307.
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false
	// Without this, wrong methods would get the 404 handler instead of 405.
	r.HandleMethodNotAllowed = true
	// Match on the escaped path, so an encoded slash stays inside its segment instead of
	// splitting it in two.
	r.UseRawPath = true
	r.UnescapePathValues = true

	r.Use(recoverPanics(log), logRequests(log))
	r.Use(mw...)
	r.NoRoute(func(c *gin.Context) { WriteError(c.Writer, http.StatusNotFound, CodeNotFound, "not found") })
	r.NoMethod(r.methodNotAllowed)
	r.Handle(http.MethodGet, "/", health)
	return r
}

// Handle registers a route and, for GET, the matching HEAD route as net/http does.
func (r *Router) Handle(method, path string, handlers ...gin.HandlerFunc) {
	r.add(method, path, handlers)
	if method == http.MethodGet {
		r.add(http.MethodHead, path, handlers)
	}
}

func (r *Router) add(method, path string, handlers []gin.HandlerFunc) {
	r.routes = append(r.routes, route{method: method, segments: strings.Split(path, "/")})
	r.Engine.Handle(method, path, handlers...)
}

func (r *Router) methodNotAllowed(c *gin.Context) {
	segs := strings.Split(c.Request.URL.Path, "/")
	var allowed []string
	for _, rt := range r.routes {
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
	WriteError(c.Writer, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "method not allowed")
}

func health(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(c.Writer, "ok")
}

// LimitBody caps the request body; reads past n fail with *http.MaxBytesError.
func LimitBody(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
	}
}

// logRequests logs method, path and status only. Headers and bodies are left out because they
// carry tokens, and the query string because tap URLs can carry credentials.
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
		WriteError(c.Writer, http.StatusInternalServerError, CodeInternalError, "internal error")
		c.Abort()
	})
}

// Error codes are part of the API: clients branch on them, because the HTTP status alone can't
// tell the 409s apart. Add new ones instead of changing what an existing one means.
const (
	CodeUnauthorized         = "unauthorized"
	CodeForbidden            = "forbidden"
	CodeAuthUnavailable      = "auth_unavailable"
	CodeNotFound             = "not_found"
	CodeMethodNotAllowed     = "method_not_allowed"
	CodeValidationFailed     = "validation_failed"
	CodePayloadTooLarge      = "payload_too_large"
	CodeConfirmationRequired = "confirmation_required"
	CodeAlreadyInstalled     = "already_installed"
	CodeHostPathConflict     = "host_path_conflict"
	CodeNoUpdateAvailable    = "no_update_available"
	CodeBusy                 = "busy"
	CodeConflict             = "conflict"
	CodeSyncFailed           = "sync_failed"
	CodeMarketplaceDown      = "marketplace_unavailable"
	CodeInternalError        = "internal_error"
)

// ErrorBody is every error response. Errors carries validation details.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Errors  any    `json:"errors,omitempty"`
}

// WriteJSON writes v as the response body.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes {"code": code, "message": msg}.
func WriteError(w http.ResponseWriter, status int, code, msg string) {
	WriteJSON(w, status, ErrorBody{Code: code, Message: msg})
}
