package api_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/oakestra/oakestra/addons/internal/marketplace/api"
	"github.com/oakestra/oakestra/addons/internal/marketplace/catalog"
	"github.com/oakestra/oakestra/addons/internal/marketplace/mptest"
	"github.com/oakestra/oakestra/addons/internal/marketplace/store"
)

func TestTrailingSlashVariants(t *testing.T) {
	f := newFixture(t, "")
	for _, p := range []string{"/api/v1/marketplace/addons", "/api/v1/marketplace/addons/", "/api/v1/marketplace/sources", "/api/v1/marketplace/sources/"} {
		if rec := f.do(t, "GET", p, "", nil, ""); rec.Code != http.StatusOK {
			t.Errorf("GET %s: %d", p, rec.Code)
		}
	}
	// A slash where no variant is registered is a 404, never a redirect.
	for _, p := range []string{"/api/v1/marketplace/sources/official/", "/api/v1/marketplace/addons/official/monitoring/"} {
		if rec := f.do(t, "GET", p, "", nil, ""); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: %d, want 404", p, rec.Code)
		}
	}
}

func TestPostTrailingSlashIsNotRedirected(t *testing.T) {
	f := newFixture(t, "")
	body := `{"name":"mytap","url":"http://tap.test/index.json","token":"` + tapToken + `","allow_insecure":true}`
	rec := f.do(t, "POST", "/api/v1/marketplace/sources/", f.admin, strings.NewReader(body), "application/json")
	if rec.Code != http.StatusCreated || rec.Header().Get("Location") != "" {
		t.Errorf("POST with trailing slash: %d, Location %q", rec.Code, rec.Header().Get("Location"))
	}
	// Unregistered shapes must not be "fixed" by a redirect either.
	rec = f.do(t, "POST", "/api/v1/marketplace//sources", f.admin, nil, "")
	if rec.Code >= 300 && rec.Code < 400 {
		t.Errorf("redirected: %d", rec.Code)
	}
}

func TestMethodNotAllowedAndNotFound(t *testing.T) {
	f := newFixture(t, "")
	rec := f.do(t, "PUT", "/api/v1/marketplace/addons", f.admin, nil, "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT: %d", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != "GET, HEAD, POST" {
		t.Errorf("Allow %q", got)
	}
	if rec.Header().Get("Content-Type") != "application/json" || strings.TrimSpace(rec.Body.String()) != `{"message":"method not allowed"}` {
		t.Errorf("405 body: %q %s", rec.Header().Get("Content-Type"), rec.Body)
	}
	if rec := f.do(t, "DELETE", "/api/v1/marketplace/addons/local/web", "", nil, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("registered DELETE without a token: %d", rec.Code)
	}
	if rec := f.do(t, "GET", "/nope", "", nil, ""); rec.Code != http.StatusNotFound ||
		rec.Header().Get("Content-Type") != "application/json" || strings.TrimSpace(rec.Body.String()) != `{"message":"not found"}` {
		t.Errorf("unknown route: %d %q %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body)
	}
	if rec := f.do(t, "HEAD", "/", "", nil, ""); rec.Code != http.StatusOK {
		t.Errorf("HEAD on a GET route: %d", rec.Code)
	}
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func TestRequestLogOmitsQueryString(t *testing.T) {
	var buf lockedBuffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	cat := catalog.New(catalog.Config{Store: store.NewMemory(), Fetcher: mptest.Fetcher(httptest.NewServer(http.NotFoundHandler()), mptest.Resolver{}), Resolver: nopResolver{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	gate := api.NewAdminGate(nil, log)
	h := api.New(api.Config{Catalog: cat, Gate: gate, Logger: log})

	req := httptest.NewRequest("GET", "/api/v1/marketplace/sources?token=hunter2&url=https://x.test/?k=secret", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)
	out := buf.String()
	if !strings.Contains(out, "path=/api/v1/marketplace/sources") || !strings.Contains(out, "status=") || !strings.Contains(out, "method=GET") {
		t.Errorf("log line incomplete: %s", out)
	}
	if strings.Contains(out, "hunter2") || strings.Contains(out, "secret") || strings.Contains(out, "?") {
		t.Errorf("query string leaked into the log: %s", out)
	}
}
