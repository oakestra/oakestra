package client

import (
	"errors"
	"fmt"
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
	Path    string // Request path relative to /api/v1, e.g. "/jobs/123".
	Status  int    // HTTP status code returned by the resource abstractor.
	Message string // Server-provided message, empty if the error body couldn't be decoded.
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("resource abstractor: %s %s: %d %s", e.Method, e.Path, e.Status, e.Message)
	}
	return fmt.Sprintf("resource abstractor: %s %s: %d", e.Method, e.Path, e.Status)
}
