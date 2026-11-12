package api

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/oakestra/oakestra/addons/internal/addons"
	"github.com/oakestra/oakestra/addons/internal/addons/store"
	"github.com/oakestra/oakestra/addons/internal/auth"
)

func TestTrailingSlashVariants(t *testing.T) {
	f := newFixture(t, "root")
	for _, p := range []string{"/api/v1/addons", "/api/v1/addons/"} {
		if code, _ := f.do("GET", p, f.user, nil); code != 200 {
			t.Errorf("GET %s: %d", p, code)
		}
	}
	for _, p := range []string{"/api/v1/ui-addons", "/api/v1/ui-addons/"} {
		if code, _ := f.do("GET", p, "", nil); code != 200 {
			t.Errorf("GET %s: %d", p, code)
		}
	}
	f.mkt.set(def("insights", "1.0.0"))
	a := f.install("insights")
	// A slash where no variant is registered is a 404, never a redirect.
	for _, p := range []string{"/api/v1/addons/" + a.ID + "/", "/api/v1/addons/" + a.ID + "/update/"} {
		code, body := f.do("POST", p, f.admin, "{}")
		if code != 404 || strings.TrimSpace(string(body)) != `{"message":"not found"}` {
			t.Errorf("POST %s: %d %s", p, code, body)
		}
	}
}

func TestNotFoundAndMethodNotAllowedAreJSON(t *testing.T) {
	f := newFixture(t, "root")
	req := httptest.NewRequest("GET", "/nope", nil)
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	if rec.Code != 404 || rec.Header().Get("Content-Type") != "application/json" || strings.TrimSpace(rec.Body.String()) != `{"message":"not found"}` {
		t.Errorf("404: %d %q %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body)
	}
	// CORS headers are on error responses as well.
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("404 lacks CORS headers: %v", rec.Header())
	}

	for path, allow := range map[string]string{
		"/api/v1/addons/0123":         "DELETE, GET, HEAD",
		"/api/v1/addons":              "GET, HEAD, POST",
		"/api/v1/addons/0123/approve": "POST",
	} {
		rec := httptest.NewRecorder()
		f.h.ServeHTTP(rec, httptest.NewRequest("PATCH", path, strings.NewReader(`{"status":"installing"}`)))
		if rec.Code != 405 || rec.Header().Get("Allow") != allow || strings.TrimSpace(rec.Body.String()) != `{"message":"method not allowed"}` {
			t.Errorf("PATCH %s: %d Allow %q %s", path, rec.Code, rec.Header().Get("Allow"), rec.Body)
		}
	}
}

func TestHeadOnGetRoutes(t *testing.T) {
	f := newFixture(t, "root")
	if code, _ := f.do("HEAD", "/", "", nil); code != 200 {
		t.Errorf("HEAD /: %d", code)
	}
	if code, _ := f.do("HEAD", "/api/v1/addons", f.user, nil); code != 200 {
		t.Errorf("HEAD list: %d", code)
	}
	if code, _ := f.do("HEAD", "/api/v1/addons", "", nil); code != 401 {
		t.Errorf("HEAD list without token: %d", code)
	}
}

func TestCORSHeadersAndOrigin(t *testing.T) {
	st := store.NewMemory()
	gate := auth.NewGate(nil, nil)
	gate.SetKey(&testKey.PublicKey)
	h := New(Config{Store: st, Marketplace: &fakeMarket{}, Gate: gate, GrantKey: testGrantKey, Plane: "root", CORSOrigin: "https://dash.example"})
	for _, path := range []string{"/api/v1/addons/x/update", "/nope"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("OPTIONS", path, nil))
		if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "https://dash.example" ||
			rec.Header().Get("Access-Control-Allow-Methods") != "GET, POST, DELETE, OPTIONS" {
			t.Errorf("OPTIONS %s: %d %v", path, rec.Code, rec.Header())
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/ui-addons", nil))
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://dash.example" || rec.Header().Get("Access-Control-Expose-Headers") != "Content-Type" {
		t.Errorf("GET: %v", rec.Header())
	}
}

func TestBodyLimitAppliesToEveryEndpoint(t *testing.T) {
	f := newFixture(t, "root")
	f.mkt.set(def("insights", "1.0.0"))
	a := f.install("insights")
	f.setStatus(a.ID, addons.Failed)
	big := `{"accept_permissions":"` + strings.Repeat("a", MaxBodyBytes) + `"}`
	for _, p := range []string{"/api/v1/addons/" + a.ID + "/update", "/api/v1/addons/" + a.ID + "/approve"} {
		if code, body := f.do("POST", p, f.admin, big); code != 413 {
			t.Errorf("POST %s: %d %s", p, code, body)
		}
	}
}

type panicStore struct{ store.Store }

func (panicStore) List(context.Context, addons.Status) ([]addons.Installed, error) {
	panic("boom")
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

func TestPanicsAndRequestLog(t *testing.T) {
	var buf lockedBuffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	gate := auth.NewGate(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	gate.SetKey(&testKey.PublicKey)
	h := New(Config{Store: panicStore{store.NewMemory()}, Marketplace: &fakeMarket{}, Gate: gate, GrantKey: testGrantKey, Plane: "root", Logger: log})

	tok := token(t, testKey, "Admin")
	req := httptest.NewRequest("GET", "/api/v1/addons?status=active&secret=hunter2", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError || strings.TrimSpace(rec.Body.String()) != `{"message":"Internal server error"}` {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	out := buf.String()
	if !strings.Contains(out, "panicked") || !strings.Contains(out, "path=/api/v1/addons") {
		t.Errorf("log: %s", out)
	}
	if strings.Contains(out, "hunter2") || strings.Contains(out, tok) {
		t.Errorf("query or token leaked into the log: %s", out)
	}
}
