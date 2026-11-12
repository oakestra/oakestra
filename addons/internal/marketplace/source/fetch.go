// Package source fetches catalog indexes from the network or the local file system.
//
// Adding a source makes the marketplace fetch an admin-supplied URL from inside the control
// plane, which is a server-side request forgery primitive. The fetcher therefore only speaks
// https (http only when the source allows it), caps time and size, follows redirects only
// within the same scheme and host, ignores proxy settings, and refuses to connect to private or
// reserved addresses unless the source explicitly allows them.
package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/oakestra/oakestra/addons/internal/definition"
)

const (
	// DefaultTimeout bounds a whole fetch, including redirects and reading the body.
	DefaultTimeout = 10 * time.Second
	// MaxRedirects is how many same-host redirects a fetch follows.
	MaxRedirects = 3
)

// Request describes one fetch.
type Request struct {
	URL string
	// Token is sent as a bearer token. It is never included in errors.
	Token string
	// ETag of the last good snapshot, sent as If-None-Match.
	ETag          string
	AllowInsecure bool
	AllowPrivate  bool
	// AllowFile permits file:// URLs. Only the official source, whose URL comes from the
	// environment, sets it; an API-added tap must never read the marketplace's file system.
	AllowFile bool
}

// Result is a successful fetch.
type Result struct {
	// NotModified is set when the server answered 304; Index is nil then.
	NotModified bool
	Index       *definition.Index
	ETag        string
}

// Fetcher fetches indexes. The zero value is not usable; use NewFetcher.
type Fetcher struct {
	resolver Resolver
	dial     DialFunc
	timeout  time.Duration
}

// Option configures a Fetcher.
type Option func(*Fetcher)

// WithResolver replaces the DNS resolver, for tests.
func WithResolver(r Resolver) Option { return func(f *Fetcher) { f.resolver = r } }

// WithDial replaces the function that connects to an already checked address, for tests.
func WithDial(d DialFunc) Option { return func(f *Fetcher) { f.dial = d } }

// WithTimeout replaces DefaultTimeout.
func WithTimeout(d time.Duration) Option { return func(f *Fetcher) { f.timeout = d } }

// NewFetcher returns a fetcher using the system resolver.
func NewFetcher(opts ...Option) *Fetcher {
	d := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	f := &Fetcher{resolver: net.DefaultResolver, dial: d.DialContext, timeout: DefaultTimeout}
	for _, o := range opts {
		o(f)
	}
	return f
}

// CheckURL validates a source URL without fetching it. allowFile is only true for the
// official source.
func CheckURL(raw string, allowInsecure, allowFile bool) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("invalid URL")
	}
	if u.User != nil {
		// Credentials in the URL would show up in the source list and in error messages. The
		// token field is the write-only alternative.
		return nil, errors.New("URL must not contain credentials, use the token field")
	}
	if u.Fragment != "" {
		return nil, errors.New("URL must not contain a fragment")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !allowInsecure {
			return nil, errors.New("URL must use https (set allow_insecure to permit http)")
		}
	case "file":
		if !allowFile {
			return nil, errors.New("file URLs are only allowed for the official source")
		}
		if u.Host != "" && u.Host != "localhost" {
			return nil, errors.New("file URL must be file:///absolute/path")
		}
		if u.Path == "" {
			return nil, errors.New("file URL has no path")
		}
		return u, nil
	default:
		return nil, fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
	if u.Hostname() == "" {
		return nil, errors.New("URL has no host")
	}
	return u, nil
}

// RedactURL returns raw without its query string, which may carry a presigned signature or a
// token. A "?<redacted>" marker shows that a query was there. Use it for anything that is
// shown to users or logged.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "invalid URL"
	}
	out := url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path, RawPath: u.RawPath}
	res := out.String()
	if u.RawQuery != "" || u.ForceQuery {
		res += "?<redacted>"
	}
	return res
}

// Fetch retrieves and decodes an index. It doesn't validate the addons or compare serials;
// the caller does that against its stored state.
func (f *Fetcher) Fetch(ctx context.Context, req Request) (*Result, error) {
	u, err := CheckURL(req.URL, req.AllowInsecure, req.AllowFile)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "file" {
		return fetchFile(u.Path)
	}

	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	transport := newTransport(f.resolver, f.dial, req.AllowPrivate, f.timeout)
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   f.timeout,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) > MaxRedirects {
				return fmt.Errorf("stopped after %d redirects", MaxRedirects)
			}
			first := via[0].URL
			if next.URL.Scheme != first.Scheme || next.URL.Host != first.Host {
				return fmt.Errorf("redirect to %s://%s is not allowed, only redirects within %s://%s", next.URL.Scheme, next.URL.Host, first.Scheme, first.Host)
			}
			return nil
		},
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", "oakestra-marketplace")
	if req.Token != "" {
		// net/http keeps this header on same-host redirects only, and CheckRedirect refuses
		// any other redirect, so the token never leaves the configured host.
		httpReq.Header.Set("Authorization", "Bearer "+req.Token)
	}
	if req.ETag != "" {
		httpReq.Header.Set("If-None-Match", req.ETag)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, describeTransportError(err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotModified:
		return &Result{NotModified: true, ETag: req.ETag}, nil
	default:
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}
	if resp.ContentLength > definition.MaxIndexSize {
		return nil, fmt.Errorf("index is %d bytes, the limit is %d", resp.ContentLength, definition.MaxIndexSize)
	}
	idx, err := definition.DecodeIndex(io.LimitReader(resp.Body, definition.MaxIndexSize+1))
	if err != nil {
		return nil, describeDecodeError(err)
	}
	return &Result{Index: idx, ETag: resp.Header.Get("ETag")}, nil
}

func fetchFile(path string) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening index file: %w", err)
	}
	defer f.Close()
	idx, err := definition.DecodeIndex(io.LimitReader(f, definition.MaxIndexSize+1))
	if err != nil {
		return nil, describeDecodeError(err)
	}
	return &Result{Index: idx}, nil
}

// Decode reads an index from an upload (air-gapped import) with the same limits as a fetch.
func Decode(r io.Reader) (*definition.Index, error) {
	idx, err := definition.DecodeIndex(io.LimitReader(r, definition.MaxIndexSize+1))
	if err != nil {
		return nil, describeDecodeError(err)
	}
	return idx, nil
}

func describeDecodeError(err error) error {
	if errors.Is(err, definition.ErrTooLarge) {
		return fmt.Errorf("index exceeds the size limit of %d bytes", definition.MaxIndexSize)
	}
	return fmt.Errorf("decoding index: %w", err)
}

// describeTransportError unwraps *url.Error so the message doesn't repeat the full URL, which
// may carry query parameters the admin considers sensitive.
func describeTransportError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("fetching index: %w", ue.Err)
	}
	return fmt.Errorf("fetching index: %w", err)
}
