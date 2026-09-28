// Package client is a Go HTTP client for the resource abstractor
// (go_resource_abstractor and its Python predecessor, resource-abstractor,
// which serve the same /api/v1 REST API). It plays the role for Go services
// that libraries/resource_abstractor_client plays for Python: a thin
// wrapper around the applications, resources (candidates), jobs and hooks
// endpoints, so consumers don't hand-roll HTTP calls.
//
// Requests, models and parameter encoding come from the openapi subpackage,
// generated from the same openapi.yaml the server is generated from. Document
// types are re-exported here as aliases (see models.go), so calling code only
// needs to import this package.
//
// See README.md for why this lives in its own Go module.
package client

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/oakestra/oakestra/go_resource_abstractor/client/openapi"
)

// defaultTimeout bounds every request made through a Client that wasn't
// constructed with WithHTTPClient or WithTimeout. The Python client had no
// timeout at all, so a hung abstractor hung the caller; we picked a sane
// default instead of reproducing that.
const defaultTimeout = 10 * time.Second

// Client is a resource abstractor HTTP client. Construct one with New or
// NewFromEnv; the Apps, Resources, Jobs and Hooks fields group methods by
// resource, mirroring the Python client's app/candidate/job operations
// modules (Hooks has no Python counterpart).
type Client struct {
	// api is narrowed to openapi.ClientInterface rather than the concrete
	// *openapi.Client it's built from (see New), so OpenAPI can't hand out
	// the *WithResponse methods either - api never held them.
	api openapi.ClientInterface

	Apps      *AppsService
	Resources *ResourcesService
	Jobs      *JobsService
	Hooks     *HooksService
}

// Option configures a Client at construction time. Options are order
// independent: WithHTTPClient and WithTimeout combine the same way
// regardless of which is passed first.
type Option func(*clientConfig)

// clientConfig collects the options passed to New before any *http.Client is
// touched, keeping WithHTTPClient and WithTimeout order independent.
type clientConfig struct {
	httpClient *http.Client
	timeout    time.Duration
}

// WithHTTPClient overrides the *http.Client used for every request, e.g. to
// install a transport with custom TLS settings or tracing. The client is
// copied, not adopted, so combining it with WithTimeout never mutates the
// caller's own client.
//
// This is also the seam for testing: install a fake http.RoundTripper on the
// client passed here to intercept every request without touching the
// network, while keeping the rest of the facade intact.
func WithHTTPClient(h *http.Client) Option {
	return func(cfg *clientConfig) {
		cfg.httpClient = h
	}
}

// WithTimeout sets the per-request timeout, applied to whichever
// *http.Client the Client ends up using (its own default one, or one
// supplied via WithHTTPClient).
func WithTimeout(d time.Duration) Option {
	return func(cfg *clientConfig) {
		cfg.timeout = d
	}
}

// New builds a Client targeting baseURL (e.g. "http://root_resource_abstractor:11011"
// or "http://localhost:11011"). baseURL should not include the /api/v1
// prefix - the spec's paths carry it, and the generated client resolves them
// against baseURL.
func New(baseURL string, opts ...Option) *Client {
	var cfg clientConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	// Copy rather than mutate: the caller's *http.Client may be shared
	// elsewhere, and writing Timeout directly could race with unrelated
	// requests using it.
	httpClient := &http.Client{Timeout: defaultTimeout}
	if cfg.httpClient != nil {
		clone := *cfg.httpClient
		httpClient = &clone
	}
	if cfg.timeout > 0 {
		httpClient.Timeout = cfg.timeout
	}

	// The only way NewClient fails is a bad ClientOption, and WithHTTPClient's
	// never is - so the error is discarded here.
	api, _ := openapi.NewClient(baseURL, openapi.WithHTTPClient(httpClient))

	c := &Client{api: api}
	c.Apps = &AppsService{c: c}
	c.Resources = &ResourcesService{c: c}
	c.Jobs = &JobsService{c: c}
	c.Hooks = &HooksService{c: c}
	return c
}

// NewFromEnv builds a Client from the same environment variables the Python
// client reads: RESOURCE_ABSTRACTOR_URL (a bare host or a full "http://"/
// "https://" URL) and RESOURCE_ABSTRACTOR_PORT (11011 for root, 11012 for
// cluster; see go_resource_abstractor/README.md). Unlike the Python client,
// which silently builds "http://None:None" when unset, this errors so
// misconfiguration fails fast at startup.
func NewFromEnv(opts ...Option) (*Client, error) {
	host := os.Getenv("RESOURCE_ABSTRACTOR_URL")
	port := os.Getenv("RESOURCE_ABSTRACTOR_PORT")
	if host == "" || port == "" {
		return nil, fmt.Errorf("resource abstractor: RESOURCE_ABSTRACTOR_URL and RESOURCE_ABSTRACTOR_PORT must both be set")
	}
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "http://" + host
	}
	// Strip a trailing slash a compose file or browser bar might carry,
	// which the port would otherwise land after.
	return New(fmt.Sprintf("%s:%s", strings.TrimSuffix(host, "/"), port), opts...), nil
}

// OpenAPI exposes the generated client underneath, narrowed to
// openapi.ClientInterface so a call through it still carries this package's
// error contract via Decode/Done instead of a raw response to switch on by
// hand. The *WithResponse variants are deliberately out of reach, so this
// escape hatch can't bypass that contract.
//
// What's left to reach through it is custom resources
// (/api/v1/custom-resources and its instances), now that hooks have their
// own facade (see HooksService).
func (c *Client) OpenAPI() openapi.ClientInterface { return c.api }

// Health reports whether the service answers its liveness probe.
func (c *Client) Health(ctx context.Context) error {
	return Done(c.api.Health(ctx))
}

// collapseParams applies filters, in order, to a zero-valued P and returns
// it - the shared implementation behind the List methods of Apps, Resources
// and Jobs. Generic over both P and F so a JobFilter can't be passed to
// Resources.List.
func collapseParams[P any, F ~func(*P)](filters []F) *P {
	var params P
	for _, filter := range filters {
		filter(&params)
	}
	return &params
}
