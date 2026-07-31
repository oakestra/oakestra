package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// ErrNotFound is returned when the resource abstractor responds 404, for
// whichever method was called. GetByName/GetByIP/GetByNameAndNamespace also
// return it when the underlying list came back empty, since the abstractor
// never 404s those itself.
//
// Unlike the Python client, which returns None alike for a 404, a connection
// failure, or any other non-2xx response, this client keeps "not found"
// distinct from "the request failed": use errors.Is(err, client.ErrNotFound)
// for the former; any other error signals the latter.
var ErrNotFound = errors.New("resource abstractor: not found")

// APIError is returned for any non-2xx response other than 404. It carries
// the HTTP status code and, when the resource abstractor's error body could
// be decoded, the server-provided message (see the {"message": "..."}
// shape produced by go_resource_abstractor/api/common.go).
type APIError struct {
	Method  string // HTTP method of the failed request, e.g. "GET".
	Path    string // Request path, e.g. "/api/v1/jobs/123".
	Status  int    // HTTP status code returned by the resource abstractor.
	Message string // Server-provided message, empty if the error body couldn't be decoded.
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("resource abstractor: %s %s: %d %s", e.Method, e.Path, e.Status, e.Message)
	}
	return fmt.Sprintf("resource abstractor: %s %s: %d", e.Method, e.Path, e.Status)
}

// newAPIError builds an *APIError from a non-2xx, non-404 response, parsing
// the {"message": "...", "details": {...}} shape the resource abstractor's
// error handler produces when it can.
//
// body arrives as bytes, not a stream, since the generated client already
// read it.
func newAPIError(resp *http.Response, body []byte) *APIError {
	apiErr := &APIError{Status: resp.StatusCode}
	apiErr.Method, apiErr.Path = requestContext(resp)

	var decoded struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &decoded); err == nil {
		apiErr.Message = decoded.Message
	}
	return apiErr
}

// requestContext extracts the method and path of the request that produced
// resp, for error messages that want to name what failed.
func requestContext(resp *http.Response) (method, path string) {
	// Request is set by net/http on every response it returns, but the field
	// is documented as nil for responses a caller constructs by hand - which
	// a stubbed HttpRequestDoer in a test may well do.
	if resp.Request == nil {
		return "", ""
	}
	method = resp.Request.Method
	if resp.Request.URL != nil {
		path = resp.Request.URL.Path
	}
	return method, path
}

// notFoundError wraps ErrNotFound with the method and path of the request
// that got the 404, so errors.Is(err, ErrNotFound) still succeeds while the
// message names what was requested. Returns the bare sentinel when resp has
// no usable request context, rather than a message with two empty fields.
func notFoundError(resp *http.Response) error {
	method, path := requestContext(resp)
	if method == "" && path == "" {
		return ErrNotFound
	}
	return fmt.Errorf("resource abstractor: %s %s: %w", method, path, ErrNotFound)
}
