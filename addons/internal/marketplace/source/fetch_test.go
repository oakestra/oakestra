package source_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/oakestra/oakestra/addons/internal/definition"
	"github.com/oakestra/oakestra/addons/internal/marketplace/mptest"
	"github.com/oakestra/oakestra/addons/internal/marketplace/source"
)

var public = mptest.Resolver{
	"tap.test":   {mptest.PublicIP},
	"other.test": {mptest.PublicIP},
}

func serveIndex(body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write(body)
	}
}

func TestFetchSucceedsAndSendsHeaders(t *testing.T) {
	var gotAuth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(r.Header.Get("Authorization"))
		serveIndex(mptest.IndexJSON("mytap", 3, mptest.Addon("a", "1.0.0")))(w, r)
	}))
	defer srv.Close()

	res, err := mptest.Fetcher(srv, public).Fetch(context.Background(), source.Request{
		URL: "http://tap.test/index.json", Token: "secret", AllowInsecure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Index.Serial != 3 || len(res.Index.Addons) != 1 || res.ETag != `"v1"` {
		t.Errorf("unexpected result %+v", res)
	}
	if got := gotAuth.Load(); got != "Bearer secret" {
		t.Errorf("Authorization = %q", got)
	}
}

func TestFetchNotModified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		serveIndex(mptest.IndexJSON("mytap", 1))(w, r)
	}))
	defer srv.Close()

	res, err := mptest.Fetcher(srv, public).Fetch(context.Background(), source.Request{
		URL: "http://tap.test/", ETag: `"v1"`, AllowInsecure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.NotModified || res.Index != nil {
		t.Errorf("expected not modified, got %+v", res)
	}
}

func TestFetchRejectsOversizedBody(t *testing.T) {
	big := []byte(`{"schema":1,"source":"mytap","serial":1,"generated_at":"2026-01-01T00:00:00Z","addons":[],"signatures":[],"x":"` +
		strings.Repeat("a", definition.MaxIndexSize) + `"}`)
	for _, chunked := range []bool{false, true} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if chunked {
				// Without a Content-Length the limit has to hold while reading.
				w.(http.Flusher).Flush()
			}
			_, _ = w.Write(big)
		}))
		_, err := mptest.Fetcher(srv, public).Fetch(context.Background(), source.Request{URL: "http://tap.test/", AllowInsecure: true})
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), "size limit") {
			t.Errorf("chunked=%v: expected size error, got %v", chunked, err)
		}
	}
}

func TestFetchRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/same":
			http.Redirect(w, r, "/index.json", http.StatusFound)
		case "/other":
			http.Redirect(w, r, "http://other.test/index.json", http.StatusFound)
		case "/scheme":
			http.Redirect(w, r, "https://tap.test/index.json", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/index.json":
			serveIndex(mptest.IndexJSON("mytap", 1))(w, r)
		}
	}))
	defer srv.Close()
	f := mptest.Fetcher(srv, public)

	if _, err := f.Fetch(context.Background(), source.Request{URL: "http://tap.test/same", AllowInsecure: true}); err != nil {
		t.Errorf("same-host redirect: %v", err)
	}
	for _, path := range []string{"/other", "/scheme", "/loop"} {
		_, err := f.Fetch(context.Background(), source.Request{URL: "http://tap.test" + path, AllowInsecure: true})
		if err == nil {
			t.Errorf("%s: redirect was followed", path)
		}
	}
}

func TestFetchBlocksPrivateAddresses(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		serveIndex(mptest.IndexJSON("mytap", 1))(w, r)
	}))
	defer srv.Close()
	hosts := mptest.Resolver{
		"loopback.test": {netip.MustParseAddr("127.0.0.1")},
		"metadata.test": {netip.MustParseAddr("169.254.169.254")},
		"lan.test":      {netip.MustParseAddr("192.168.1.10")},
		// One bad address taints the whole name.
		"mixed.test": {mptest.PublicIP, netip.MustParseAddr("10.0.0.1")},
	}
	f := mptest.Fetcher(srv, hosts)
	for host := range hosts {
		_, err := f.Fetch(context.Background(), source.Request{URL: "http://" + host + "/", AllowInsecure: true})
		if !errors.Is(err, source.ErrBlockedAddress) {
			t.Errorf("%s: expected ErrBlockedAddress, got %v", host, err)
		}
	}
	// IP literals skip the resolver but not the check.
	_, err := f.Fetch(context.Background(), source.Request{URL: "http://" + srv.Listener.Addr().String() + "/", AllowInsecure: true})
	if !errors.Is(err, source.ErrBlockedAddress) {
		t.Errorf("IP literal: expected ErrBlockedAddress, got %v", err)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("server was reached %d times", n)
	}

	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	res, err := f.Fetch(context.Background(), source.Request{URL: "http://loopback.test:" + port + "/", AllowInsecure: true, AllowPrivate: true})
	if err != nil || res.Index == nil {
		t.Errorf("allow_private: %v", err)
	}
}

func TestFetchStatusAndSchemeErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	f := mptest.Fetcher(srv, public)

	if _, err := f.Fetch(context.Background(), source.Request{URL: "http://tap.test/", AllowInsecure: true}); err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("500: %v", err)
	}
	if _, err := f.Fetch(context.Background(), source.Request{URL: "http://tap.test/"}); err == nil || !strings.Contains(err.Error(), "https") {
		t.Errorf("http without allow_insecure: %v", err)
	}
	if _, err := f.Fetch(context.Background(), source.Request{URL: "https://user:pw@tap.test/"}); err == nil {
		t.Error("credentials in URL accepted")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "index.json")
	if err := os.WriteFile(path, mptest.IndexJSON("official", 7), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Fetch(context.Background(), source.Request{URL: "file://" + path}); err == nil {
		t.Error("file URL accepted without AllowFile")
	}
	res, err := f.Fetch(context.Background(), source.Request{URL: "file://" + path, AllowFile: true})
	if err != nil || res.Index.Serial != 7 {
		t.Errorf("file URL: %v", err)
	}
}
