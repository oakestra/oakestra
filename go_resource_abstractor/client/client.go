// Package client is a Go HTTP client for the resource abstractor
// (go_resource_abstractor and its Python predecessor, resource-abstractor -
// they serve the same wire-compatible /api/v1 REST API). It plays the same
// role for Go services that libraries/resource_abstractor_client plays for
// Python services: a thin wrapper around the applications, resources
// (candidates), and jobs endpoints, so consumers don't hand-roll HTTP calls
// against the abstractor.
//
// Requests, models and parameter encoding all come from the service's
// OpenAPI spec: the openapi subpackage is generated from
// go_resource_abstractor/openapi/openapi.yaml, the same file the server's
// routes and handler interface are generated from. This package is the
// ergonomic layer on top - environment-based construction, the
// ErrNotFound/APIError split, filter options in place of pointer-filled
// parameter structs, and the handful of lookups the Python client has
// consumers for.
//
// Calling code needs this package alone: the document types are re-exported
// here as aliases (see models.go), so openapi need not be imported to name a
// Job or a Resource. Use Client.OpenAPI to reach an endpoint this package
// doesn't wrap.
//
// It is a standalone Go module (see go.mod) nested inside
// go_resource_abstractor/ purely for co-location with the server it talks
// to; a consumer pulls it in with a replace directive pointing at this
// directory. See README.md for the full recipe.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/oakestra/oakestra/go_resource_abstractor/client/openapi"
)

// defaultTimeout bounds every request made through a Client that wasn't
// constructed with WithHTTPClient or WithTimeout. The Python client had no
// timeout at all (a hung abstractor hangs the caller); we add a sane
// default rather than reproduce that.
const defaultTimeout = 10 * time.Second

// Client is a resource abstractor HTTP client. Construct one with New or
// NewFromEnv; the Apps, Resources, and Jobs fields group methods by
// resource, mirroring the three modules of the Python client
// (app_operations, candidate_operations, job_operations).
type Client struct {
	api *openapi.ClientWithResponses

	Apps      *AppsClient
	Resources *ResourcesClient
	Jobs      *JobsClient
}

// Option configures a Client at construction time. Options are resolved
// into a clientConfig before any *http.Client is touched, so - unlike an
// Option that mutated the Client (or a caller-supplied *http.Client)
// directly as each option ran - WithHTTPClient and WithTimeout combine the
// same way regardless of the order they're passed in.
type Option func(*clientConfig)

type clientConfig struct {
	httpClient *http.Client
	timeout    time.Duration
}

// WithHTTPClient overrides the *http.Client used for every request,
// e.g. to install a transport with custom TLS settings or tracing. The
// supplied client is copied, not adopted, so combining it with WithTimeout
// never mutates the caller's own client.
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

	// Copy rather than mutate: a *http.Client passed via WithHTTPClient is
	// owned by the caller and may be shared with other code (or already in
	// use by another goroutine), so writing Timeout onto it would change -
	// and race with - requests that have nothing to do with this Client.
	httpClient := &http.Client{Timeout: defaultTimeout}
	if cfg.httpClient != nil {
		clone := *cfg.httpClient
		httpClient = &clone
	}
	if cfg.timeout > 0 {
		httpClient.Timeout = cfg.timeout
	}

	// The generated constructor's only error path is a failing ClientOption,
	// and WithHTTPClient never fails - so New keeps its error-free signature
	// rather than pushing an impossible error onto every caller.
	api, err := openapi.NewClientWithResponses(baseURL, openapi.WithHTTPClient(httpClient))
	if err != nil {
		panic(fmt.Sprintf("client: constructing generated client for %q: %v", baseURL, err))
	}

	c := &Client{api: api}
	c.Apps = &AppsClient{c: c}
	c.Resources = &ResourcesClient{c: c}
	c.Jobs = &JobsClient{c: c}
	return c
}

// NewFromEnv builds a Client from the same environment variables the
// Python client reads: RESOURCE_ABSTRACTOR_URL (host, e.g.
// "root_resource_abstractor" or "cluster_resource_abstractor") and
// RESOURCE_ABSTRACTOR_PORT (11011 for root, 11012 for cluster; see
// go_resource_abstractor/README.md). Unlike the Python client - which
// silently builds "http://None:None" when the variables are unset -
// NewFromEnv returns an error so misconfiguration fails fast at startup.
func NewFromEnv(opts ...Option) (*Client, error) {
	host := os.Getenv("RESOURCE_ABSTRACTOR_URL")
	port := os.Getenv("RESOURCE_ABSTRACTOR_PORT")
	if host == "" || port == "" {
		return nil, fmt.Errorf("client: RESOURCE_ABSTRACTOR_URL and RESOURCE_ABSTRACTOR_PORT must both be set")
	}
	return New(fmt.Sprintf("http://%s:%s", host, port), opts...), nil
}

// OpenAPI exposes the generated client underneath, which covers every
// operation in the spec - including /hooks and /custom-resources, which the
// Apps/Resources/Jobs facades deliberately don't wrap because no consumer
// needs them yet. Its methods report failures as a response to inspect
// rather than through the ErrNotFound/APIError split documented on this
// package.
func (c *Client) OpenAPI() *openapi.ClientWithResponses { return c.api }

// Health reports whether the service answers its liveness probe.
func (c *Client) Health(ctx context.Context) error {
	return done(c.api.Health(ctx))
}

// The helpers below adapt the generated operations - each of which returns
// the bare (*http.Response, error) pair - to this package's error contract.
// Every facade method is one call to one of them.
//
// They deliberately consume the low-level generated methods rather than
// their *WithResponse counterparts, which decode the body for every status
// the spec documents. That decode is the difficulty: it fails the call
// outright when a documented status arrives carrying something other than
// the documented shape - an empty 404 from a proxy, say - and the error it
// returns carries no response, so the status is gone and ErrNotFound can no
// longer be reported. Reading the body here keeps the status authoritative,
// and costs only the json.Unmarshal below, since the models it decodes into
// are generated all the same.

// read maps a response's status onto this package's error contract - 404 to
// ErrNotFound, any other non-2xx to *APIError - and returns the body for the
// caller to decode. err is the transport-level failure the generated client
// reports (DNS, connection refused, a cancelled context), which stays
// distinguishable from both.
func read(resp *http.Response, err error) ([]byte, error) {
	if err != nil {
		return nil, fmt.Errorf("client: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("client: reading response body: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, newAPIError(resp, body)
	}
	return body, nil
}

// decode reads a response and unmarshals its body into T. A success with an
// empty body (a 204, which delete endpoints answer with) yields the zero
// value rather than an error.
func decode[T any](resp *http.Response, err error) (T, error) {
	var out T

	body, err := read(resp, err)
	if err != nil || len(body) == 0 {
		return out, err
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return out, fmt.Errorf("client: decoding response body: %w", err)
	}
	return out, nil
}

// doc decodes a single-document response.
func doc[T any](resp *http.Response, err error) (*T, error) {
	out, err := decode[T](resp, err)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// list decodes a list-returning response.
func list[T any](resp *http.Response, err error) ([]T, error) {
	return decode[[]T](resp, err)
}

// firstOf returns the first element of a list-returning response, or
// ErrNotFound if it came back empty - the Go equivalent of the Python
// client's `result[0] if result else None` pattern used by
// GetByName/GetByIP/GetByNameAndNamespace.
func firstOf[T any](resp *http.Response, err error) (*T, error) {
	items, err := list[T](resp, err)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return &items[0], nil
}

// done discards the body of a response whose content the caller ignores,
// keeping only the status check.
func done(resp *http.Response, err error) error {
	_, err = read(resp, err)
	return err
}
