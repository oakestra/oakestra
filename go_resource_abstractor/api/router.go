package api

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"go_resource_abstractor/db"
	"go_resource_abstractor/openapi"
	"go_resource_abstractor/services"
)

// NewRouter builds the HTTP handler for the whole service.
//
// The route table isn't written out here: RegisterHandlersWithOptions is
// generated from openapi/openapi.yaml, and the compiler enforces that *Server
// implements every operation the spec declares (openapi.ServerInterface). To
// add or change an endpoint, edit the spec and re-run `go generate
// ./openapi`.
func NewRouter(store *db.Store, hooks *services.Hooks) http.Handler {
	s := &Server{store: store, hooks: hooks}

	router := gin.New()
	router.Use(gin.Recovery(), requestLogger())
	router.Use(corsMiddleware())

	// Trailing slashes are normalized by stripTrailingSlash below rather than
	// by gin, which would answer with a redirect the Flask service never sent.
	router.RedirectTrailingSlash = false
	router.RedirectFixedPath = false

	openapi.RegisterHandlersWithOptions(router, s, openapi.GinServerOptions{
		ErrorHandler: rejectMalformedParameter,
	})

	return stripTrailingSlash(router)
}

// rejectMalformedParameter answers a request whose path or query parameters
// failed to bind to the types openapi.yaml declares for them.
//
// In practice only :instance_id lands here - the spec's other parameters
// with their own validation contract (?active=, ?instance_number=) are
// declared as strings and parsed inside the handlers instead. If a future
// spec change adds another non-string parameter, give it the same
// string-plus-handler-parsing treatment if it needs anything other than
// this blanket 400.
func rejectMalformedParameter(c *gin.Context, err error, _ int) {
	slog.Debug("rejected request parameter", "path", c.Request.URL.Path, "error", err)
	abortBadRequest(c)
}

// Health implements GET /. It is the liveness probe the Python service
// exposed at the same path.
func (s *Server) Health(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}

// GetSpecJSON implements GET /docs/openapi.json.
func (s *Server) GetSpecJSON(c *gin.Context) {
	body, err := openapi.SpecJSON()
	if err != nil {
		abortInternalError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// GetSpecYAML implements GET /docs/openapi.yaml.
func (s *Server) GetSpecYAML(c *gin.Context) {
	c.Data(http.StatusOK, "application/yaml; charset=utf-8", openapi.SpecYAML)
}

// stripTrailingSlash rewrites "/path/" to "/path" before routing, so every
// route is reachable under both forms - matches Python's
// strict_slashes = False.
//
// Runs outside the gin engine since gin middleware only executes after a
// route is matched, which is too late. Rewrites rather than redirects
// because a 301/308 would make some clients re-issue POST/PUT as GET, or
// drop the body.
func stripTrailingSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := r.URL.Path; len(p) > 1 && strings.HasSuffix(p, "/") {
			trimmed := *r.URL
			trimmed.Path = p[:len(p)-1]

			rewritten := *r
			rewritten.URL = &trimmed
			r = &rewritten
		}
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware reproduces the CORS configuration resource_abstractor.py
// applies to /api/*: open origins, the standard write methods, and the
// Content-Type/Authorization headers, without credentials.
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Header("Access-Control-Expose-Headers", "Content-Type")
			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
		}
		c.Next()
	}
}

// requestLogger emits a debug-level structured log line per request.
func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Debug("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration", time.Since(start),
		)
	}
}

// Server implements every operation openapi.yaml declares; the compiler
// enforces it here as well as at the RegisterHandlersWithOptions call above,
// so a spec change that adds an operation fails the build until it is
// implemented.
var _ openapi.ServerInterface = (*Server)(nil)
