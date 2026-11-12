// Package marketplaceclient fetches addon definitions from the marketplace service.
//
// The marketplace also returns precomputed fields (id, qualified_name, required_permissions).
// They are dropped on purpose: decoding goes into definition.Addon, which
// has no place for them, so every security decision has to be recomputed from the definition.
package marketplaceclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/oakestra/oakestra/addons/internal/definition"
)

// Timeout bounds one marketplace request. Installs wait on it, so the server's write timeout
// has to stay above it.
const Timeout = 10 * time.Second

// maxBody is far above any definition. It guards against a misbehaving endpoint.
const maxBody = 1 << 20

var (
	// ErrNotFound means the marketplace doesn't know the addon.
	ErrNotFound = errors.New("addon not found in the marketplace")
	// ErrUnavailable means the marketplace couldn't answer: connection errors, timeouts, error
	// statuses and broken bodies. Callers keep what they have instead of treating the addon as
	// gone.
	ErrUnavailable = errors.New("marketplace is unavailable")
)

// Client talks to the marketplace's HTTP API.
type Client struct {
	base string
	http *http.Client
}

// New returns a client for the marketplace at addr, e.g. "http://marketplace_manager:11102".
func New(addr string) *Client {
	return &Client{base: strings.TrimRight(addr, "/"), http: &http.Client{Timeout: Timeout}}
}

// Get fetches an addon. The result is normalized, and its source and name are checked against
// the request, but it is not validated: callers run definition.Check themselves.
func (c *Client) Get(ctx context.Context, src definition.Source, name string) (*definition.Addon, error) {
	u := fmt.Sprintf("%s/api/v1/marketplace/addons/%s/%s", c.base, url.PathEscape(src.Name), url.PathEscape(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
		return nil, ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%w: unexpected status %s", ErrUnavailable, resp.Status)
	}

	var a definition.Addon
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&a); err != nil {
		return nil, fmt.Errorf("%w: decoding response: %v", ErrUnavailable, err)
	}
	a.Normalize()
	if a.Name != name || a.Source != src {
		// A confused or hostile marketplace could otherwise hand back another addon's
		// definition under the name the admin confirmed.
		return nil, fmt.Errorf("%w: asked for %s/%s but got %s/%s", ErrUnavailable, src.Name, name, a.Source.Name, a.Name)
	}
	return &a, nil
}

// Getter is what consumers need from the client, so tests can substitute a fake.
type Getter interface {
	Get(ctx context.Context, src definition.Source, name string) (*definition.Addon, error)
}

var _ Getter = (*Client)(nil)
