package api

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"go_resource_abstractor/db"
	"go_resource_abstractor/services"
)

// NewRouter builds the gin engine and registers every route, mirroring the
// blueprints registered by resource_abstractor.py: resources, applications,
// jobs, hooks, custom-resources, plus the GET / health check.
//
// The Swagger UI / OpenAPI spec the Python service serves at /api/docs and
// /docs/openapi.json is intentionally not reproduced here: no consumer
// (scheduler, system_manager, cluster_manager) depends on it.
func NewRouter(store *db.Store, hooks *services.Hooks) *gin.Engine {
	s := &Server{store: store, hooks: hooks}

	router := gin.New()
	router.Use(gin.Recovery(), requestLogger())
	router.Use(corsMiddleware())

	// The Python service sets app.url_map.strict_slashes = False, so both
	// "/path" and "/path/" must resolve identically. gin's own trailing-
	// slash redirect only applies to GET, so instead every collection
	// route below is registered explicitly under both forms.
	router.RedirectTrailingSlash = false
	router.RedirectFixedPath = false

	router.GET("/", health)

	v1 := router.Group("/api/v1")
	s.registerResourceRoutes(v1)
	s.registerApplicationRoutes(v1)
	s.registerJobRoutes(v1)
	s.registerHookRoutes(v1)
	s.registerCustomResourceRoutes(v1)

	return router
}

func health(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}

// bothSlashes registers handlers for a collection route under both its bare
// and trailing-slash forms (e.g. "/resources" and "/resources/"), the
// equivalent of Flask's strict_slashes=False for that route.
func bothSlashes(group *gin.RouterGroup, method string, handlers ...gin.HandlerFunc) {
	group.Handle(method, "", handlers...)
	group.Handle(method, "/", handlers...)
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
