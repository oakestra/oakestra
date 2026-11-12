package api_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/oakestra/oakestra/addons/internal/auth"
	"github.com/oakestra/oakestra/addons/internal/marketplace/api"
	"github.com/oakestra/oakestra/addons/internal/marketplace/catalog"
	"github.com/oakestra/oakestra/addons/internal/marketplace/mptest"
	"github.com/oakestra/oakestra/addons/internal/marketplace/store"
)

const tapToken = "tap-secret-token-do-not-leak"

var (
	signingKey = mustKey()
)

func mustKey() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}

func token(t *testing.T, key *rsa.PrivateKey, roles ...string) string {
	t.Helper()
	now := time.Now()
	s, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": "alice", "type": "access", "roles": roles,
		"iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(10 * time.Minute).Unix(),
	}).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type fixture struct {
	h     http.Handler
	gate  *auth.Gate
	admin string
	user  string
}

type nopResolver struct{}

func (nopResolver) Resolve(context.Context, string) (string, error) {
	return "sha256:" + strings.Repeat("ef", 32), nil
}

func newFixture(t *testing.T, cors string) *fixture {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+tapToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write(mptest.IndexJSON("mytap", 2, mptest.Addon("logs", "1.2.0")))
	}))
	t.Cleanup(srv.Close)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cat := catalog.New(catalog.Config{
		Store:    store.NewMemory(),
		Fetcher:  mptest.Fetcher(srv, mptest.Resolver{"tap.test": {mptest.PublicIP}}),
		Resolver: nopResolver{},
		Logger:   log,
	})
	if err := cat.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := cat.Import(context.Background(), "official", bytes.NewReader(mptest.IndexJSON("official", 1, mptest.Addon("monitoring", "1.0.0")))); err != nil {
		t.Fatal(err)
	}
	gate := auth.NewGate(nil, log)
	gate.SetKey(&signingKey.PublicKey)
	return &fixture{
		h:     api.New(api.Config{Catalog: cat, Gate: gate, CORSOrigin: cors, Logger: log}),
		gate:  gate,
		admin: token(t, signingKey, "Admin"),
		user:  token(t, signingKey, "Application_Provider"),
	}
}

func (f *fixture) do(t *testing.T, method, path, tok string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	return rec
}

func (f *fixture) addTap(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"name":"mytap","url":"http://tap.test/index.json","token":"` + tapToken + `","allow_insecure":true}`
	rec := f.do(t, "POST", "/api/v1/marketplace/sources", f.admin, strings.NewReader(body), "application/json")
	if rec.Code != http.StatusCreated {
		t.Fatalf("add tap: %d %s", rec.Code, rec.Body)
	}
	return rec
}

var adminEndpoints = []struct{ method, path string }{
	{"POST", "/api/v1/marketplace/addons"},
	{"DELETE", "/api/v1/marketplace/addons/local/web"},
	{"POST", "/api/v1/marketplace/sources"},
	{"DELETE", "/api/v1/marketplace/sources/mytap"},
	{"POST", "/api/v1/marketplace/sources/official/sync"},
	{"POST", "/api/v1/marketplace/sources/official/import"},
}

func TestAdminEndpointsNeedAdmin(t *testing.T) {
	f := newFixture(t, "")
	for _, ep := range adminEndpoints {
		for _, tc := range []struct {
			name string
			tok  string
			want int
		}{
			{"no token", "", http.StatusUnauthorized},
			{"non-admin", f.user, http.StatusForbidden},
			{"foreign key", token(t, mustKey(), "Admin"), http.StatusUnauthorized},
		} {
			rec := f.do(t, ep.method, ep.path, tc.tok, strings.NewReader("{}"), "application/json")
			if rec.Code != tc.want {
				t.Errorf("%s %s (%s): %d, want %d", ep.method, ep.path, tc.name, rec.Code, tc.want)
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["message"] == "" {
				t.Errorf("%s %s (%s): error body %q", ep.method, ep.path, tc.name, rec.Body)
			}
		}
	}
}

func TestAdminEndpointsUnavailableWithoutKey(t *testing.T) {
	f := newFixture(t, "")
	gate := auth.NewGate(nil, nil)
	f.h = api.New(api.Config{Catalog: nil, Gate: gate})
	for _, ep := range adminEndpoints {
		rec := f.do(t, ep.method, ep.path, f.admin, nil, "")
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s: %d, want 503", ep.method, ep.path, rec.Code)
		}
	}
}

func TestTokensNeverReturned(t *testing.T) {
	f := newFixture(t, "")
	created := f.addTap(t)
	var src catalog.SourceView
	if err := json.Unmarshal(created.Body.Bytes(), &src); err != nil {
		t.Fatal(err)
	}
	if !src.HasToken || src.Serial != 2 || src.LastError != "" {
		t.Errorf("created tap: %+v", src)
	}
	for _, rec := range []*httptest.ResponseRecorder{
		created,
		f.do(t, "GET", "/api/v1/marketplace/sources", "", nil, ""),
		f.do(t, "GET", "/api/v1/marketplace/addons", "", nil, ""),
		f.do(t, "GET", "/api/v1/marketplace/addons/mytap/logs", "", nil, ""),
		f.do(t, "POST", "/api/v1/marketplace/sources/mytap/sync", f.admin, nil, ""),
	} {
		if rec.Code >= 300 {
			t.Errorf("unexpected status %d: %s", rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body.String(), tapToken) {
			t.Errorf("token leaked: %s", rec.Body)
		}
		if strings.Contains(rec.Body.String(), `"token"`) {
			t.Errorf("token field present: %s", rec.Body)
		}
	}
}

func TestMergedListAndAddonContract(t *testing.T) {
	f := newFixture(t, "")
	f.addTap(t)
	local := `{"name":"web","version":"0.1.0","services":[{"service_name":"web","image":"nginx:1.27","networks":[],"volumes":[],"ports":{"80":"8080"}}],"permissions":{"host_ports":[8080]}}`
	rec := f.do(t, "POST", "/api/v1/marketplace/addons", f.admin, strings.NewReader(local), "application/json")
	if rec.Code != http.StatusCreated {
		t.Fatalf("register local: %d %s", rec.Code, rec.Body)
	}

	rec = f.do(t, "GET", "/api/v1/marketplace/addons", "", nil, "")
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, it := range list {
		names = append(names, it["qualified_name"].(string))
	}
	if strings.Join(names, ",") != "monitoring,mytap/logs,local/web" {
		t.Errorf("merged list: %v", names)
	}
	rec = f.do(t, "GET", "/api/v1/marketplace/addons/?source=mytap", "", nil, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 {
		t.Errorf("filtered list: %s", rec.Body)
	}

	rec = f.do(t, "GET", "/api/v1/marketplace/addons/local/web", "", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get one: %d %s", rec.Code, rec.Body)
	}
	var item map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"name", "version", "source", "qualified_name", "id", "required_permissions", "permissions_hash", "permissions", "services", "volumes", "networks"} {
		if _, ok := item[k]; !ok {
			t.Errorf("addon is missing %q: %s", k, rec.Body)
		}
	}
	if src := item["source"].(map[string]any); src["kind"] != "local" || src["name"] != "local" {
		t.Errorf("source: %v", src)
	}
	if item["id"] != "local--web" || !strings.HasPrefix(item["permissions_hash"].(string), "sha256:") {
		t.Errorf("derived fields: %v %v", item["id"], item["permissions_hash"])
	}
	img := item["services"].([]any)[0].(map[string]any)["image"].(string)
	if !strings.HasPrefix(img, "nginx:1.27@sha256:") {
		t.Errorf("image not pinned: %s", img)
	}
	if ports := item["required_permissions"].(map[string]any)["host_ports"].([]any); len(ports) != 1 || ports[0] != float64(8080) {
		t.Errorf("required_permissions: %v", item["required_permissions"])
	}

	if rec := f.do(t, "GET", "/api/v1/marketplace/addons/mytap/nope", "", nil, ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing addon: %d", rec.Code)
	}
	if rec := f.do(t, "GET", "/api/v1/marketplace/addons/notatap/logs", "", nil, ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing source: %d", rec.Code)
	}
	if rec := f.do(t, "DELETE", "/api/v1/marketplace/addons/local/web", f.admin, nil, ""); rec.Code != http.StatusNoContent {
		t.Errorf("delete local: %d %s", rec.Code, rec.Body)
	}
}

func TestRegisterLocalErrors(t *testing.T) {
	f := newFixture(t, "")
	cases := []struct {
		name string
		body string
		want int
	}{
		{"unknown field", `{"name":"web","privileged":true}`, http.StatusUnprocessableEntity},
		{"policy violation", `{"name":"web","version":"1.0.0","services":[{"service_name":"web","image":"nginx","networks":["host"]}]}`, http.StatusUnprocessableEntity},
		{"too large", `{"name":"` + strings.Repeat("a", 2<<20) + `"}`, http.StatusRequestEntityTooLarge},
	}
	for _, tc := range cases {
		rec := f.do(t, "POST", "/api/v1/marketplace/addons", f.admin, strings.NewReader(tc.body), "application/json")
		if rec.Code != tc.want {
			t.Errorf("%s: %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body)
		}
	}
}

func TestSourceManagement(t *testing.T) {
	f := newFixture(t, "")
	f.addTap(t)
	dup := f.do(t, "POST", "/api/v1/marketplace/sources", f.admin, strings.NewReader(`{"name":"mytap","url":"https://tap.test/"}`), "application/json")
	if dup.Code != http.StatusConflict {
		t.Errorf("duplicate tap: %d", dup.Code)
	}
	if rec := f.do(t, "DELETE", "/api/v1/marketplace/sources/official", f.admin, nil, ""); rec.Code != http.StatusConflict {
		t.Errorf("delete official: %d", rec.Code)
	}
	if rec := f.do(t, "POST", "/api/v1/marketplace/sources/official/sync", f.admin, nil, ""); rec.Code != http.StatusConflict {
		t.Errorf("sync with official off: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, "DELETE", "/api/v1/marketplace/sources/mytap", f.admin, nil, ""); rec.Code != http.StatusNoContent {
		t.Errorf("delete tap: %d", rec.Code)
	}
	if rec := f.do(t, "POST", "/api/v1/marketplace/sources/mytap/sync", f.admin, nil, ""); rec.Code != http.StatusNotFound {
		t.Errorf("sync deleted tap: %d", rec.Code)
	}
}

func TestImport(t *testing.T) {
	f := newFixture(t, "")

	rec := f.do(t, "POST", "/api/v1/marketplace/sources/official/import", f.admin, bytes.NewReader(mptest.IndexJSON("official", 2, mptest.Addon("monitoring", "1.1.0"))), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("raw import: %d %s", rec.Code, rec.Body)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("comment", "ignored")
	fw, _ := mw.CreateFormFile("file", "index.json")
	_, _ = fw.Write(mptest.IndexJSON("official", 3, mptest.Addon("monitoring", "1.2.0")))
	_ = mw.Close()
	rec = f.do(t, "POST", "/api/v1/marketplace/sources/official/import", f.admin, &buf, mw.FormDataContentType())
	if rec.Code != http.StatusOK {
		t.Fatalf("multipart import: %d %s", rec.Code, rec.Body)
	}
	var src catalog.SourceView
	_ = json.Unmarshal(rec.Body.Bytes(), &src)
	if src.Serial != 3 {
		t.Errorf("serial after import: %d", src.Serial)
	}

	rec = f.do(t, "POST", "/api/v1/marketplace/sources/official/import", f.admin, bytes.NewReader(mptest.IndexJSON("official", 1)), "application/json")
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "lower") {
		t.Errorf("rollback import: %d %s", rec.Code, rec.Body)
	}
	rec = f.do(t, "POST", "/api/v1/marketplace/sources/official/import", f.admin, strings.NewReader(strings.Repeat(" ", 7<<20)), "application/json")
	if rec.Code != http.StatusRequestEntityTooLarge && rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("oversized import: %d %s", rec.Code, rec.Body)
	}
}

func TestHealthAndCORS(t *testing.T) {
	f := newFixture(t, "")
	if rec := f.do(t, "GET", "/", "", nil, ""); rec.Code != http.StatusOK {
		t.Errorf("health: %d", rec.Code)
	}
	req := httptest.NewRequest("GET", "/api/v1/marketplace/addons", nil)
	req.Header.Set("Origin", "https://evil.test")
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("CORS header sent without configuration")
	}

	f = newFixture(t, "https://dash.test")
	for origin, want := range map[string]string{"https://dash.test": "https://dash.test", "https://evil.test": ""} {
		req := httptest.NewRequest("OPTIONS", "/api/v1/marketplace/sources", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "POST")
		rec := httptest.NewRecorder()
		f.h.ServeHTTP(rec, req)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != want {
			t.Errorf("origin %s: allow-origin %q, want %q", origin, got, want)
		}
	}
}

// gatedStore counts how many list calls are inside ListSnapshotHeaders at once and holds them until
// released.
type gatedStore struct {
	store.Store
	cur, peak atomic.Int32
	release   chan struct{}
}

func (g *gatedStore) ListSnapshotHeaders(ctx context.Context) ([]store.Snapshot, error) {
	n := g.cur.Add(1)
	defer g.cur.Add(-1)
	for {
		p := g.peak.Load()
		if n <= p || g.peak.CompareAndSwap(p, n) {
			break
		}
	}
	<-g.release
	return g.Store.ListSnapshotHeaders(ctx)
}

func TestListAddonsConcurrencyIsBounded(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	gs := &gatedStore{Store: store.NewMemory(), release: make(chan struct{})}
	cat := catalog.New(catalog.Config{Store: gs, Fetcher: mptest.Fetcher(httptest.NewServer(http.NotFoundHandler()), mptest.Resolver{}), Resolver: nopResolver{}, Logger: log})
	if err := cat.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	gate := auth.NewGate(nil, log)
	h := api.New(api.Config{Catalog: cat, Gate: gate, Logger: log})

	const requests = 40
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/marketplace/addons", nil))
			if rec.Code != http.StatusOK {
				t.Errorf("status %d", rec.Code)
			}
		}()
	}
	// Let the requests pile up against the limit before letting any through.
	time.Sleep(200 * time.Millisecond)
	close(gs.release)
	wg.Wait()
	if p := gs.peak.Load(); p > 8 || p == 0 {
		t.Errorf("peak concurrent list calls = %d, want 1..8", p)
	}
}

func TestSourcesListRedactsQuery(t *testing.T) {
	f := newFixture(t, "")
	body := `{"name":"signed","url":"https://tap.test/index.json?token=hunter2"}`
	rec := f.do(t, "POST", "/api/v1/marketplace/sources", f.admin, strings.NewReader(body), "application/json")
	if rec.Code != http.StatusCreated {
		t.Fatalf("add tap: %d %s", rec.Code, rec.Body)
	}
	rec = f.do(t, "GET", "/api/v1/marketplace/sources", "", nil, "")
	var views []struct{ Name, URL string }
	if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	var got string
	for _, v := range views {
		if v.Name == "signed" {
			got = v.URL
		}
	}
	if strings.Contains(rec.Body.String(), "hunter2") || got != "https://tap.test/index.json?<redacted>" {
		t.Errorf("sources: %s", rec.Body)
	}
}
