// Package client is a Go HTTP client for the resource abstractor
// (go_resource_abstractor and its Python predecessor, resource-abstractor -
// they serve the same wire-compatible /api/v1 REST API). It plays the same
// role for Go services that libraries/resource_abstractor_client plays for
// Python services: a thin, dependency-free wrapper around the applications,
// resources (candidates), and jobs endpoints, so consumers don't hand-roll
// HTTP calls against the abstractor.
//
// It is a standalone Go module (see go.mod) nested inside
// go_resource_abstractor/ purely for co-location with the server it talks
// to - it shares no code or types with the server, which is itself
// schema-agnostic (documents flow through as bson.M/map[string]any). A
// consumer pulls it in with a replace directive pointing at this directory;
// see README.md for the full recipe.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Document is a schema-agnostic JSON object, matching the resource
// abstractor's own request/response shape: every endpoint accepts and
// returns plain JSON documents rather than a fixed struct, so the client
// mirrors that instead of inventing per-resource Go types that would drift
// from the server's actual (schema-validated, but not statically typed)
// contract.
type Document = map[string]any

// defaultTimeout bounds every request made through a Client that wasn't
// constructed with WithHTTPClient or WithTimeout. The Python client had no
// timeout at all (a hung abstractor hangs the caller); we add a sane
// default rather than reproduce that.
const defaultTimeout = 10 * time.Second

// apiPrefix is prepended to every request path. The resource abstractor
// mounts all routes under /api/v1 (see go_resource_abstractor/api/router.go).
const apiPrefix = "/api/v1"

// Client is a resource abstractor HTTP client. Construct one with New or
// NewFromEnv; the Apps, Resources, and Jobs fields group methods by
// resource, mirroring the three modules of the Python client
// (app_operations, candidate_operations, job_operations).
type Client struct {
	baseURL    string
	httpClient *http.Client

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
// prefix or a trailing slash - both are handled internally.
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

	c := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
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

// do issues a request to path (relative to apiPrefix) and decodes a 2xx
// JSON response into out. body is JSON-marshaled as the request body when
// non-nil; query parameters with a non-empty value are appended to the URL.
// out may be nil to discard the response body (used by Delete methods).
//
// A 404 response maps to ErrNotFound; any other non-2xx response maps to
// *APIError. Both are distinguishable from transport-level failures (DNS,
// connection refused, context deadline), which are returned as-is/wrapped.
func (c *Client) do(ctx context.Context, method, path string, query map[string]string, body, out any) error {
	fullURL := c.baseURL + apiPrefix + path

	if len(query) > 0 {
		values := url.Values{}
		for k, v := range query {
			if v != "" {
				values.Set(k, v)
			}
		}
		if encoded := values.Encode(); encoded != "" {
			fullURL += "?" + encoded
		}
	}

	var bodyReader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("client: encoding request body for %s %s: %w", method, path, err)
		}
		bodyReader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return fmt.Errorf("client: building request for %s %s: %w", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("client: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return newAPIError(method, path, resp)
	}

	if out == nil {
		// Drain rather than just closing, so the underlying connection can
		// be reused - e.g. every Delete method ignores the response body,
		// but the server does send one for most delete endpoints.
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		if err == io.EOF {
			// A 2xx with an empty body (e.g. 204 No Content) leaves out
			// untouched rather than erroring.
			return nil
		}
		return fmt.Errorf("client: decoding response for %s %s: %w", method, path, err)
	}
	return nil
}

// newAPIError builds an *APIError from a non-2xx, non-404 response,
// parsing the {"message": "...", "details": {...}} shape the resource
// abstractor's error handler produces (go_resource_abstractor/api/common.go)
// when possible.
func newAPIError(method, path string, resp *http.Response) *APIError {
	apiErr := &APIError{Method: method, Path: path, Status: resp.StatusCode}

	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err == nil {
		apiErr.Message = body.Message
	}
	return apiErr
}

// first returns the first element of docs, or ErrNotFound if docs is empty -
// the Go equivalent of the Python client's `result[0] if result else None`
// pattern used by GetByName/GetByIP/GetByNameAndNamespace.
func first(docs []Document) (Document, error) {
	if len(docs) == 0 {
		return nil, ErrNotFound
	}
	return docs[0], nil
}

// doList issues a GET-shaped request expecting a list of documents back.
// Used by every resource's List method.
func (c *Client) doList(ctx context.Context, method, path string, query map[string]string) ([]Document, error) {
	var docs []Document
	if err := c.do(ctx, method, path, query, nil, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

// doFirst issues a list-returning request and returns its first result, or
// ErrNotFound if the result is empty. Used by GetByName/GetByIP/
// GetByNameAndNamespace-style lookups that filter a list endpoint down to a
// single expected match.
func (c *Client) doFirst(ctx context.Context, method, path string, query map[string]string) (Document, error) {
	docs, err := c.doList(ctx, method, path, query)
	if err != nil {
		return nil, err
	}
	return first(docs)
}

// doDoc issues a request expecting a single document back.
func (c *Client) doDoc(ctx context.Context, method, path string, query map[string]string, body any) (Document, error) {
	var doc Document
	if err := c.do(ctx, method, path, query, body, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// withKey returns a shallow copy of m with key set to value, leaving the
// caller's map untouched. m may be nil.
func withKey[M ~map[string]V, V any](m M, key string, value V) M {
	out := make(M, len(m)+1)
	maps.Copy(out, m)
	out[key] = value
	return out
}
