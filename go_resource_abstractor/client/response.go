package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// The helpers below adapt the generated operations - each returning a bare
// (*http.Response, error) - to this package's error contract. Every facade
// method is one call to one of them.
//
// They use the low-level generated methods rather than the *WithResponse
// ones, which decode the body for every documented status and fail outright
// on an unexpected shape (an empty 404 from a proxy, say), losing the status
// itself and with it the ability to report ErrNotFound.

// errorFor maps a non-2xx response onto ErrNotFound (404) or *APIError,
// reading the body first since newAPIError needs it to parse the message.
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
// client reports (DNS, connection refused, a cancelled context).
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

// Decode reads a response from the generated client and unmarshals its body
// into T, mapping failures onto ErrNotFound, *APIError, or a transport error.
// A success with an empty body (a 204) yields the zero value.
//
// Exported for Client.OpenAPI: a caller reaching past the facades for a
// custom resource has no facade method to call this for it. A list is
// expressed through the type parameter rather than a separate function, so
// one generic covers both single documents and lists:
//
//	hooks, err := client.Decode[[]client.Hook](c.OpenAPI().ListHooks(ctx))
func Decode[T any](resp *http.Response, err error) (T, error) {
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
// 204) yields a pointer to the zero value, not nil.
func doc[T any](resp *http.Response, err error) (*T, error) {
	out, err := Decode[T](resp, err)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// list decodes a list-returning response.
func list[T any](resp *http.Response, err error) ([]T, error) {
	return Decode[[]T](resp, err)
}

// firstOf returns the first element of a list-returning response, or
// ErrNotFound if it came back empty - the Go equivalent of the Python
// client's `result[0] if result else None` pattern.
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

// Done discards the body of a response whose content the caller ignores,
// keeping only the status check. Unlike read, a successful body streams to
// io.Discard rather than being buffered - a deleted job's body can be
// sizeable (it echoes the job with its full instance_list and history).
// Exported for Client.OpenAPI, same reason as Decode.
func Done(resp *http.Response, err error) error {
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
