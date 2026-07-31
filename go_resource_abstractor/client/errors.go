package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// ErrNotFound is returned when the resource abstractor responds 404 to a
// request that expects a single document (a GetByID call, or a
// GetByName/GetByIP/GetByNameAndNamespace lookup that found no match).
//
// Unlike the Python resource_abstractor_client - which returns None for a
// 404, a connection failure, and any other non-2xx response alike - this
// client keeps "not found" distinct from "the request failed": callers can
// use errors.Is(err, client.ErrNotFound) to detect the former, while any
// other error (including *APIError below) signals the latter.
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
// error handler produces (go_resource_abstractor/api/common.go) when
// possible.
//
// body arrives as bytes rather than a stream because the generated client has
// already read it - it exposes every response both raw and decoded, so
// there is nothing left to consume here.
func newAPIError(resp *http.Response, body []byte) *APIError {
	apiErr := &APIError{Status: resp.StatusCode}

	// Request is set by net/http on every response it returns, but the field
	// is documented as nil for responses a caller constructs by hand - which
	// a stubbed HttpRequestDoer in a test may well do.
	if resp.Request != nil {
		apiErr.Method = resp.Request.Method
		if resp.Request.URL != nil {
			apiErr.Path = resp.Request.URL.Path
		}
	}

	var decoded struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &decoded); err == nil {
		apiErr.Message = decoded.Message
	}
	return apiErr
}
