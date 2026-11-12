// Package api is the marketplace's HTTP interface.
//
// Reads are unauthenticated. The cluster addons managers run on other hosts, fetch definitions
// and the JWT public key from root:11102 and have no user token to present, and the dashboard
// lists the catalog before anyone has decided to install anything. That is also why compose
// publishes the port on the root host.
//
// The cost is that anyone who can reach the port can see which taps are configured (names,
// URLs, sync errors) and every addon they publish, including private taps. Tokens are never
// returned, only has_token, and tap URLs lose their query string. The list endpoints are kept
// cheap to serve (derived items are cached, concurrency is bounded). Operators who don't run
// remote clusters, or want the catalog closed, should firewall the port or bind it to a
// private interface.
//
// Everything that changes state needs an admin token from system_manager.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/oakestra/oakestra/addons/internal/definition"
	"github.com/oakestra/oakestra/addons/internal/marketplace/catalog"
)

const (
	// MaxBodySize applies to every request except imports.
	MaxBodySize = 1 << 20
	// MaxImportSize leaves room for multipart framing around a MaxIndexSize index.
	MaxImportSize = 6 << 20
)

// Config wires the handler.
type Config struct {
	Catalog *catalog.Service
	Gate    *AdminGate
	// CORSOrigin enables CORS for exactly this origin. Empty sends no CORS headers; the
	// dashboard reaches the marketplace through its own nginx proxy, which needs none.
	CORSOrigin string
	Logger     *slog.Logger
}

// maxConcurrentLists bounds the unauthenticated list endpoint, which reads every snapshot
// from the database.
const maxConcurrentLists = 8

type handler struct {
	cat *catalog.Service
	log *slog.Logger
	// listSem is a counting semaphore for listAddons.
	listSem chan struct{}
}

// New returns the marketplace's HTTP handler. The Gin engine is only a router: it is served by
// the caller's http.Server, which owns the timeouts and graceful shutdown.
func New(cfg Config) http.Handler {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if os.Getenv(gin.EnvGinMode) == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	h := &handler{cat: cfg.Catalog, log: cfg.Logger, listSem: make(chan struct{}, maxConcurrentLists)}
	admin := cfg.Gate.RequireAdmin()

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
	r.Use(recoverPanics(cfg.Logger), logRequests(cfg.Logger))
	if cfg.CORSOrigin != "" {
		r.Use(cors(cfg.CORSOrigin))
	}
	// JSON like every other error response, so clients can rely on a {"message"} body.
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

	handle(http.MethodGet, "/", health)
	handle(http.MethodGet, "/api/v1/marketplace/auth/public-key", publicKey(cfg.Gate))

	// The dashboard may send either form.
	for _, p := range []string{"/api/v1/marketplace/addons", "/api/v1/marketplace/addons/"} {
		handle(http.MethodGet, p, h.listAddons)
		handle(http.MethodPost, p, admin, limitBody(MaxBodySize), h.registerLocal)
	}
	handle(http.MethodGet, "/api/v1/marketplace/addons/:source/:name", h.getAddon)
	handle(http.MethodDelete, "/api/v1/marketplace/addons/local/:name", admin, h.deleteLocal)

	for _, p := range []string{"/api/v1/marketplace/sources", "/api/v1/marketplace/sources/"} {
		handle(http.MethodGet, p, h.listSources)
		handle(http.MethodPost, p, admin, limitBody(MaxBodySize), h.addTap)
	}
	handle(http.MethodDelete, "/api/v1/marketplace/sources/:name", admin, h.deleteSource)
	handle(http.MethodPost, "/api/v1/marketplace/sources/:name/sync", admin, h.syncSource)
	handle(http.MethodPost, "/api/v1/marketplace/sources/:name/import", admin, limitBody(MaxImportSize), h.importIndex)

	return r
}

func health(c *gin.Context) {
	w := c.Writer
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "ok")
}

// publicKey republishes the JWT verification key in jwt_generator's GET /key format. Its port
// is not published on the root host, because jwt_generator's /create mints arbitrary tokens
// without authentication, yet the cluster addons managers on other hosts need the key to verify
// admin tokens. They point JWT_PUBLIC_KEY_URL here instead. A public key is not secret, so the
// endpoint needs no auth. It serves whatever key is current, including after a re-fetch.
func publicKey(g *AdminGate) gin.HandlerFunc {
	return func(c *gin.Context) {
		w := c.Writer
		key, ok := g.PublicKeyPEM()
		if !ok {
			w.Header().Set("Retry-After", "5")
			writeError(w, http.StatusServiceUnavailable, "the JWT public key hasn't been loaded yet")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"public_key": key})
	}
}

func (h *handler) listAddons(c *gin.Context) {
	w, r := c.Writer, c.Request
	select {
	case h.listSem <- struct{}{}:
		defer func() { <-h.listSem }()
	case <-r.Context().Done():
		return
	}
	q := r.URL.Query()
	items, err := h.cat.Addons(r.Context(), catalog.Filter{Source: q.Get("source"), Name: q.Get("name")})
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *handler) getAddon(c *gin.Context) {
	w, r := c.Writer, c.Request
	item, err := h.cat.Addon(r.Context(), c.Param("source"), c.Param("name"))
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *handler) registerLocal(c *gin.Context) {
	w, r := c.Writer, c.Request
	a, err := definition.DecodeAddon(r.Body)
	if err != nil {
		h.fail(w, decodeError(err))
		return
	}
	item, err := h.cat.RegisterLocal(r.Context(), a)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *handler) deleteLocal(c *gin.Context) {
	w, r := c.Writer, c.Request
	if err := h.cat.DeleteLocal(r.Context(), c.Param("name")); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) listSources(c *gin.Context) {
	w, r := c.Writer, c.Request
	sources, err := h.cat.Sources(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sources)
}

func (h *handler) addTap(c *gin.Context) {
	w, r := c.Writer, c.Request
	var in catalog.TapInput
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		h.fail(w, decodeError(err))
		return
	}
	src, err := h.cat.AddTap(r.Context(), in)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, src)
}

func (h *handler) deleteSource(c *gin.Context) {
	w, r := c.Writer, c.Request
	if err := h.cat.DeleteSource(r.Context(), c.Param("name")); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) syncSource(c *gin.Context) {
	w, r := c.Writer, c.Request
	name := c.Param("name")
	if err := h.cat.Sync(r.Context(), name); err != nil {
		var (
			rejected *catalog.RejectedError
			fetch    *catalog.FetchError
		)
		switch {
		case errors.As(err, &rejected), errors.As(err, &fetch):
			// The fetch or the index is at fault, not the request. The error is also on
			// the source's last_error.
			writeError(w, http.StatusBadGateway, err.Error())
		default:
			h.fail(w, err)
		}
		return
	}
	h.respondSource(w, r, name)
}

func (h *handler) importIndex(c *gin.Context) {
	w, r := c.Writer, c.Request
	name := c.Param("name")
	body, closeBody, err := importBody(r)
	if err != nil {
		h.fail(w, err)
		return
	}
	defer closeBody()
	if err := h.cat.Import(r.Context(), name, body); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body is too large")
			return
		}
		h.fail(w, err)
		return
	}
	h.respondSource(w, r, name)
}

func (h *handler) respondSource(w http.ResponseWriter, r *http.Request, name string) {
	src, err := h.cat.Source(r.Context(), name)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, src)
}

// importBody accepts either a raw JSON index or a multipart form with the index in a file
// part (named "file", or the first part with a filename). The multipart body is streamed, not
// buffered to disk, so the size limit is the only thing it costs.
func importBody(r *http.Request) (io.Reader, func(), error) {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/form-data" {
		return r.Body, func() {}, nil
	}
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, nil, &catalog.ValidationError{Msg: "invalid multipart body: " + err.Error()}
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return nil, nil, &catalog.ValidationError{Msg: `multipart body has no file part (use the field name "file")`}
		}
		if err != nil {
			return nil, nil, decodeError(err)
		}
		if part.FormName() == "file" || part.FileName() != "" {
			return part, func() { _ = part.Close() }, nil
		}
		_ = part.Close()
	}
}

func decodeError(err error) error {
	var maxErr *http.MaxBytesError
	switch {
	case errors.As(err, &maxErr):
		return errTooLarge
	case errors.Is(err, definition.ErrTooLarge):
		return errTooLarge
	}
	return &catalog.ValidationError{Msg: "invalid request body: " + err.Error()}
}

var errTooLarge = errors.New("request body is too large")

func (h *handler) fail(w http.ResponseWriter, err error) {
	var (
		validation *catalog.ValidationError
		conflict   *catalog.ConflictError
		rejected   *catalog.RejectedError
		resolve    *catalog.ResolveError
	)
	switch {
	case errors.Is(err, errTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, catalog.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, catalog.ErrSyncDisabled):
		writeError(w, http.StatusConflict, err.Error())
	case errors.As(err, &validation):
		body := errorBody{Message: validation.Msg}
		if len(validation.Fields) > 0 {
			body.Errors = validation.Fields
		}
		writeJSON(w, http.StatusUnprocessableEntity, body)
	case errors.As(err, &conflict):
		writeError(w, http.StatusConflict, conflict.Msg)
	case errors.As(err, &rejected):
		writeError(w, http.StatusUnprocessableEntity, rejected.Msg)
	case errors.As(err, &resolve):
		writeError(w, http.StatusUnprocessableEntity, "resolving image digest: "+resolve.Error())
	default:
		h.log.Error("request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func limitBody(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
	}
}

func cors(origin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Add("Vary", "Origin")
		if c.Request.Header.Get("Origin") == origin {
			h.Set("Access-Control-Allow-Origin", origin)
			if c.Request.Method == http.MethodOptions && c.Request.Header.Get("Access-Control-Request-Method") != "" {
				h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				h.Set("Access-Control-Max-Age", "600")
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
		}
		c.Next()
	}
}

// logRequests logs method, path and status only. Headers and bodies are left out because
// they carry tokens, and the query string because tap URLs can carry credentials.
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

// recoverPanics replaces gin.Recovery's default output, which dumps request headers, with a
// log line through the service logger. The response matches fail's internal error.
func recoverPanics(log *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, err any) {
		log.Error("request panicked", "method", c.Request.Method, "path", c.Request.URL.Path, "panic", err)
		writeError(c.Writer, http.StatusInternalServerError, "internal error")
		c.Abort()
	})
}

// routeTable lets the 405 handler report an Allow header like net/http's mux does, which Gin
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
