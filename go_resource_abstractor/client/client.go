// Package client is a Go HTTP client for the resource abstractor
// (go_resource_abstractor and its Python predecessor, resource-abstractor,
// which serve the same /api/v1 REST API). It plays the same role for Go
// services that libraries/resource_abstractor_client plays for Python
// services: a thin wrapper around the applications, resources (candidates)
// and jobs endpoints, so consumers don't hand-roll HTTP calls.
//
// Requests, models and parameter encoding come from the service's OpenAPI
// spec: the openapi subpackage is generated from
// go_resource_abstractor/openapi/openapi.yaml, the same file the server's
// routes and handlers are generated from.
//
// Document types are re-exported here as aliases (see models.go), so
// calling code only needs to import this package, not openapi. Use
// Client.OpenAPI to reach an endpoint the facade doesn't wrap.
//
// See README.md for why this lives in its own Go module and how a consumer
// pulls it in.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
// NewFromEnv; the Apps, Resources, and Jobs fields group methods by
// resource, mirroring the three modules of the Python client
// (app_operations, candidate_operations, job_operations).
type Client struct {
	api *openapi.ClientWithResponses

	Apps      *AppsService
	Resources *ResourcesService
	Jobs      *JobsService
}

// Option configures a Client at construction time. Options are order
// independent: WithHTTPClient and WithTimeout combine the same way
// regardless of which is passed first.
type Option func(*clientConfig)

// clientConfig collects the options passed to New before any *http.Client is
// touched, which is what keeps WithHTTPClient and WithTimeout order
// independent no matter which one runs first.
type clientConfig struct {
	httpClient *http.Client
	timeout    time.Duration
}

// WithHTTPClient overrides the *http.Client used for every request, e.g. to
// install a transport with custom TLS settings or tracing. The supplied
// client is copied, not adopted, so combining it with WithTimeout never
// mutates the caller's own client.
//
// This takes a concrete *http.Client, so a caller with a custom
// openapi.HttpRequestDoer implementation should construct the generated
// client directly instead of going through New.
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

	// Copy rather than mutate: the caller's *http.Client may be shared with
	// other code, so writing Timeout onto it directly could change - and
	// race with - requests that have nothing to do with this Client.
	httpClient := &http.Client{Timeout: defaultTimeout}
	if cfg.httpClient != nil {
		clone := *cfg.httpClient
		httpClient = &clone
	}
	if cfg.timeout > 0 {
		httpClient.Timeout = cfg.timeout
	}

	// openapi.NewClientWithResponses can only fail via a bad ClientOption,
	// and WithHTTPClient's never does, so the error is discarded rather than
	// pushed onto every caller of New.
	api, _ := openapi.NewClientWithResponses(baseURL, openapi.WithHTTPClient(httpClient))

	c := &Client{api: api}
	c.Apps = &AppsService{c: c}
	c.Resources = &ResourcesService{c: c}
	c.Jobs = &JobsService{c: c}
	return c
}

// NewFromEnv builds a Client from the same environment variables the Python
// client reads: RESOURCE_ABSTRACTOR_URL (a bare host, e.g.
// "root_resource_abstractor", or a full "http://"/"https://" URL used
// as-is) and RESOURCE_ABSTRACTOR_PORT (11011 for root, 11012 for cluster;
// see go_resource_abstractor/README.md). Unlike the Python client, which
// silently builds "http://None:None" when the variables are unset,
// NewFromEnv returns an error so misconfiguration fails fast at startup.
func NewFromEnv(opts ...Option) (*Client, error) {
	host := os.Getenv("RESOURCE_ABSTRACTOR_URL")
	port := os.Getenv("RESOURCE_ABSTRACTOR_PORT")
	if host == "" || port == "" {
		return nil, fmt.Errorf("resource abstractor: RESOURCE_ABSTRACTOR_URL and RESOURCE_ABSTRACTOR_PORT must both be set")
	}
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "http://" + host
	}
	// A value copied out of a browser or a compose file may carry a trailing
	// slash, which the port would otherwise be appended after.
	return New(fmt.Sprintf("%s:%s", strings.TrimSuffix(host, "/"), port), opts...), nil
}

// OpenAPI exposes the generated client underneath, which covers every
// operation in the spec, including /hooks and /custom-resources that the
// Apps/Resources/Jobs facades don't wrap. Its methods report failures as a
// response to inspect rather than through the ErrNotFound/APIError split
// used elsewhere in this package.
func (c *Client) OpenAPI() *openapi.ClientWithResponses { return c.api }

// Health reports whether the service answers its liveness probe.
func (c *Client) Health(ctx context.Context) error {
	return done(c.api.Health(ctx))
}

// The helpers below adapt the generated operations - each of which returns
// the bare (*http.Response, error) pair - to this package's error contract.
// Every facade method is one call to one of them.
//
// They use the low-level generated methods rather than the *WithResponse
// ones on purpose: those decode the body for every status the spec
// documents, and fail outright when a documented status shows up with an
// unexpected shape (an empty 404 from a proxy, say) - at which point the
// status itself is gone and ErrNotFound can't be reported. Reading the body
// ourselves keeps the status authoritative.

// errorFor maps a non-2xx response onto this package's error contract - 404
// to ErrNotFound, anything else to *APIError - reading the body first since
// newAPIError needs it to parse the message.
func errorFor(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("resource abstractor: reading response body: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return notFoundError(resp)
	}
	return newAPIError(resp, body)
}

// read returns a 2xx response's body for the caller to decode, or an error
// mapped by errorFor. err is the transport-level failure the generated
// client reports (DNS, connection refused, a cancelled context), which stays
// distinguishable from both.
func read(resp *http.Response, err error) ([]byte, error) {
	if err != nil {
		return nil, fmt.Errorf("resource abstractor: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errorFor(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("resource abstractor: reading response body: %w", err)
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
		return out, fmt.Errorf("resource abstractor: decoding response body: %w", err)
	}
	return out, nil
}

// doc decodes a single-document response. A success with an empty body (a
// 204) yields a pointer to the zero value rather than nil, so a caller
// cannot distinguish that from a document the service genuinely returned
// empty.
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
// keeping only the status check. Unlike read, a successful body is streamed
// to io.Discard rather than buffered - Delete and Health never look at it,
// and a deleted job's body can be sizeable (it echoes the job with its full
// instance_list and history).
func done(resp *http.Response, err error) error {
	if err != nil {
		return fmt.Errorf("resource abstractor: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errorFor(resp)
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// collapseParams applies filters, in order, to a zero-valued P and returns
// it - the shared implementation behind appParams, resourceParams and
// jobParams, one per resource because each closes over a different
// generated *Params type.
func collapseParams[P any, F ~func(*P)](filters []F) *P {
	var params P
	for _, filter := range filters {
		filter(&params)
	}
	return &params
}
