package catalog_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/oakestra/oakestra/addons/internal/definition"
	"github.com/oakestra/oakestra/addons/internal/marketplace/catalog"
	"github.com/oakestra/oakestra/addons/internal/marketplace/mptest"
	"github.com/oakestra/oakestra/addons/internal/marketplace/store"
)

// indexServer serves whatever body/status the test sets, with an ETag derived from the body.
type indexServer struct {
	mu     sync.Mutex
	body   []byte
	status int
	srv    *httptest.Server
}

func newIndexServer(t *testing.T) *indexServer {
	s := &indexServer{status: http.StatusOK}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		body, status := s.body, s.status
		s.mu.Unlock()
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		sum := sha256.Sum256(body)
		etag := `"` + hex.EncodeToString(sum[:8]) + `"`
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		_, _ = w.Write(body)
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *indexServer) set(status int, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status, s.body = status, body
}

type fakeResolver struct {
	digests map[string]string
	calls   []string
}

func (f *fakeResolver) Resolve(_ context.Context, image string) (string, error) {
	f.calls = append(f.calls, image)
	if d, ok := f.digests[image]; ok {
		return d, nil
	}
	return "", errors.New("manifest unknown")
}

type env struct {
	cat   *catalog.Service
	store *store.Memory
	srv   *indexServer
	res   *fakeResolver
}

func setup(t *testing.T, officialURL string) *env {
	t.Helper()
	srv := newIndexServer(t)
	st := store.NewMemory()
	res := &fakeResolver{digests: map[string]string{}}
	switch officialURL {
	case "default":
		// httptest serves plain http; the official source may use it because its URL is
		// operator-configured.
		officialURL = "http://official.test/index.json"
	case "off":
		officialURL = ""
	}
	cat := catalog.New(catalog.Config{
		Store: st,
		Fetcher: mptest.Fetcher(srv.srv, mptest.Resolver{
			"tap.test":      {mptest.PublicIP},
			"official.test": {mptest.PublicIP},
		}),
		Resolver:    res,
		OfficialURL: officialURL,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err := cat.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	return &env{cat: cat, store: st, srv: srv, res: res}
}

func (e *env) addTap(t *testing.T) {
	t.Helper()
	if _, err := e.cat.AddTap(context.Background(), catalog.TapInput{
		Name: "mytap", URL: "http://tap.test/index.json", AllowInsecure: true, Token: "tok",
	}); err != nil {
		t.Fatal(err)
	}
}

func (e *env) source(t *testing.T, name string) *catalog.SourceView {
	t.Helper()
	v, err := e.cat.Source(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (e *env) names(t *testing.T, src string) []string {
	t.Helper()
	items, err := e.cat.Addons(context.Background(), catalog.Filter{Source: src})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, it := range items {
		out = append(out, it.Name+"@"+it.Version)
	}
	return out
}

func TestSyncSerialRules(t *testing.T) {
	e := setup(t, "off")
	ctx := context.Background()

	e.srv.set(200, mptest.IndexJSON("mytap", 5, mptest.Addon("a", "1.0.0")))
	e.addTap(t)
	if v := e.source(t, "mytap"); v.Serial != 5 || v.LastError != "" || v.LastSyncedAt == nil || v.AddonCount != 1 {
		t.Fatalf("after first sync: %+v", v)
	}

	tests := []struct {
		name    string
		status  int
		body    []byte
		wantErr string
	}{
		{"lower serial", 200, mptest.IndexJSON("mytap", 4, mptest.Addon("a", "0.9.0")), "lower than the last accepted serial 5"},
		{"same serial, other content", 200, mptest.IndexJSON("mytap", 5, mptest.Addon("a", "6.6.6")), "already accepted with different content"},
		{"server error", 500, nil, "500"},
		{"source mismatch", 200, mptest.IndexJSON("othertap", 9, mptest.Addon("a", "2.0.0")), `doesn't match the source name "mytap"`},
		{"invalid addon", 200, func() []byte {
			bad := mptest.Addon("b", "2.0.0")
			bad.Services[0].Image = "ghcr.io/example/b:latest" // not digest pinned
			return mptest.IndexJSON("mytap", 9, mptest.Addon("a", "2.0.0"), bad)
		}(), "must be pinned by digest"},
		{"undeclared permission", 200, func() []byte {
			bad := mptest.Addon("b", "2.0.0")
			bad.Services[0].Volumes = []string{"/var/run/docker.sock:/var/run/docker.sock"}
			return mptest.IndexJSON("mytap", 9, bad)
		}(), "docker_socket"},
		{"garbage", 200, []byte("<html>"), "decoding index"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e.srv.set(tt.status, tt.body)
			err := e.cat.Sync(ctx, "mytap")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Sync error = %v, want %q", err, tt.wantErr)
			}
			v := e.source(t, "mytap")
			if v.Serial != 5 || !strings.Contains(v.LastError, tt.wantErr) {
				t.Errorf("source after failure: serial %d, last_error %q", v.Serial, v.LastError)
			}
			if got := e.names(t, "mytap"); len(got) != 1 || got[0] != "a@1.0.0" {
				t.Errorf("last good snapshot not kept: %v", got)
			}
		})
	}

	// Same serial with the same content is a no-op success and clears the error.
	e.srv.set(200, mptest.IndexJSON("mytap", 5, mptest.Addon("a", "1.0.0")))
	if err := e.cat.Sync(ctx, "mytap"); err != nil {
		t.Fatalf("refetch of the same index: %v", err)
	}
	if v := e.source(t, "mytap"); v.LastError != "" {
		t.Errorf("error not cleared: %q", v.LastError)
	}
	// Now the ETag matches and the server answers 304.
	if err := e.cat.Sync(ctx, "mytap"); err != nil {
		t.Fatalf("304: %v", err)
	}
	if got := e.names(t, "mytap"); len(got) != 1 {
		t.Errorf("304 lost the snapshot: %v", got)
	}

	e.srv.set(200, mptest.IndexJSON("mytap", 6, mptest.Addon("a", "1.1.0"), mptest.Addon("b", "1.0.0")))
	if err := e.cat.Sync(ctx, "mytap"); err != nil {
		t.Fatal(err)
	}
	if got := e.names(t, "mytap"); strings.Join(got, ",") != "a@1.1.0,b@1.0.0" {
		t.Errorf("after update: %v", got)
	}
}

func TestImportWithOfficialOff(t *testing.T) {
	e := setup(t, "off")
	ctx := context.Background()

	if err := e.cat.Sync(ctx, "official"); !errors.Is(err, catalog.ErrSyncDisabled) {
		t.Fatalf("sync of disabled official: %v", err)
	}
	if v := e.source(t, "official"); v.SyncEnabled || v.Deletable {
		t.Errorf("official source view: %+v", v)
	}
	if err := e.cat.Import(ctx, "official", bytes.NewReader(mptest.IndexJSON("official", 10, mptest.Addon("mon", "1.0.0")))); err != nil {
		t.Fatal(err)
	}
	if got := e.names(t, "official"); len(got) != 1 || got[0] != "mon@1.0.0" {
		t.Errorf("after import: %v", got)
	}
	err := e.cat.Import(ctx, "official", bytes.NewReader(mptest.IndexJSON("official", 9, mptest.Addon("mon", "0.1.0"))))
	var rejected *catalog.RejectedError
	if !errors.As(err, &rejected) {
		t.Errorf("lower serial import: %v", err)
	}
	err = e.cat.Import(ctx, "official", strings.NewReader(`{"schema":1}`+strings.Repeat(" ", definition.MaxIndexSize)))
	if !errors.As(err, &rejected) || !strings.Contains(err.Error(), "size limit") {
		t.Errorf("oversized import: %v", err)
	}
	if err := e.cat.Import(ctx, "local", bytes.NewReader(mptest.IndexJSON("official", 11))); err == nil {
		t.Error("import into local accepted")
	}
}

func TestOfficialSyncAndURLChange(t *testing.T) {
	e := setup(t, "default")
	ctx := context.Background()
	e.srv.set(200, mptest.IndexJSON("official", 50, mptest.Addon("mon", "1.0.0")))
	if err := e.cat.Sync(ctx, "official"); err != nil {
		t.Fatal(err)
	}

	// Same store, new URL: the serial floor resets but the addons stay until the next sync.
	cat := catalog.New(catalog.Config{
		Store:       e.store,
		Fetcher:     mptest.Fetcher(e.srv.srv, mptest.Resolver{"official2.test": {mptest.PublicIP}}),
		OfficialURL: "http://official2.test/index.json",
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err := cat.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if items, _ := cat.Addons(ctx, catalog.Filter{}); len(items) != 1 {
		t.Errorf("addons lost on URL change: %d", len(items))
	}
	e.srv.set(200, mptest.IndexJSON("official", 3, mptest.Addon("mon", "2.0.0")))
	if err := cat.Sync(ctx, "official"); err != nil {
		t.Errorf("sync after URL change: %v", err)
	}
	if err := cat.DeleteSource(ctx, "official"); err == nil {
		t.Error("official source deleted")
	}
}

func TestDeleteTap(t *testing.T) {
	e := setup(t, "off")
	e.srv.set(200, mptest.IndexJSON("mytap", 1, mptest.Addon("a", "1.0.0")))
	e.addTap(t)
	if err := e.cat.DeleteSource(context.Background(), "mytap"); err != nil {
		t.Fatal(err)
	}
	if items, _ := e.cat.Addons(context.Background(), catalog.Filter{}); len(items) != 0 {
		t.Errorf("tap addons still listed: %d", len(items))
	}
	if err := e.cat.DeleteSource(context.Background(), "mytap"); !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
}

func TestAddTapValidation(t *testing.T) {
	e := setup(t, "off")
	for _, in := range []catalog.TapInput{
		{Name: "official", URL: "https://tap.test/"},
		{Name: "Bad_Name", URL: "https://tap.test/"},
		{Name: "ok", URL: "http://tap.test/"},
		{Name: "ok", URL: "file:///etc/passwd"},
		{Name: "ok", URL: "ftp://tap.test/"},
		{Name: "ok", URL: "https://tap.test/", Token: "has space"},
	} {
		var verr *catalog.ValidationError
		if _, err := e.cat.AddTap(context.Background(), in); !errors.As(err, &verr) {
			t.Errorf("%+v: expected a validation error, got %v", in, err)
		}
	}
}

func TestRegisterLocal(t *testing.T) {
	e := setup(t, "off")
	ctx := context.Background()
	digest := "sha256:" + strings.Repeat("cd", 32)
	e.res.digests["nginx:1.27"] = digest

	a := mptest.Addon("web", "1.0.0")
	a.Services[0].Image = "nginx:1.27"
	a.Source = definition.TapSource("sneaky") // must be overridden
	item, err := e.cat.RegisterLocal(ctx, &a)
	if err != nil {
		t.Fatal(err)
	}
	if item.Source != definition.LocalSource || item.QualifiedName != "local/web" || item.ID != "local--web" {
		t.Errorf("unexpected identity: %+v %q %q", item.Source, item.QualifiedName, item.ID)
	}
	if got := item.Services[0].Image; got != "nginx:1.27@"+digest {
		t.Errorf("image not pinned: %q", got)
	}
	stored, err := e.cat.Addon(ctx, "local", "web")
	if err != nil || stored.Services[0].Image != "nginx:1.27@"+digest {
		t.Errorf("stored definition: %+v, %v", stored, err)
	}

	// Pinned images are not resolved again.
	pinned := mptest.Addon("web", "1.1.0")
	e.res.calls = nil
	if _, err := e.cat.RegisterLocal(ctx, &pinned); err != nil {
		t.Fatal(err)
	}
	if len(e.res.calls) != 0 {
		t.Errorf("resolver called for a pinned image: %v", e.res.calls)
	}

	var conflict *catalog.ConflictError
	same := mptest.Addon("web", "1.1.0")
	if _, err := e.cat.RegisterLocal(ctx, &same); !errors.As(err, &conflict) {
		t.Errorf("same version: %v", err)
	}
	older := mptest.Addon("web", "1.0.5")
	if _, err := e.cat.RegisterLocal(ctx, &older); !errors.As(err, &conflict) {
		t.Errorf("older version: %v", err)
	}

	unknown := mptest.Addon("other", "1.0.0")
	unknown.Services[0].Image = "example.test/missing:1"
	var resolveErr *catalog.ResolveError
	if _, err := e.cat.RegisterLocal(ctx, &unknown); !errors.As(err, &resolveErr) {
		t.Errorf("unresolvable image: %v", err)
	}

	risky := mptest.Addon("risky", "1.0.0")
	risky.Services[0].Networks = []string{"host"}
	var verr *catalog.ValidationError
	if _, err := e.cat.RegisterLocal(ctx, &risky); !errors.As(err, &verr) || len(verr.Fields) == 0 {
		t.Errorf("undeclared host network: %v", err)
	}

	if err := e.cat.DeleteLocal(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	if err := e.cat.DeleteLocal(ctx, "web"); !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
}

func TestMergedListOrderAndFilter(t *testing.T) {
	e := setup(t, "off")
	ctx := context.Background()
	if err := e.cat.Import(ctx, "official", bytes.NewReader(mptest.IndexJSON("official", 1, mptest.Addon("zeta", "1.0.0"), mptest.Addon("alpha", "1.0.0")))); err != nil {
		t.Fatal(err)
	}
	e.srv.set(200, mptest.IndexJSON("mytap", 1, mptest.Addon("alpha", "2.0.0")))
	e.addTap(t)
	local := mptest.Addon("alpha", "3.0.0")
	if _, err := e.cat.RegisterLocal(ctx, &local); err != nil {
		t.Fatal(err)
	}

	items, err := e.cat.Addons(ctx, catalog.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range items {
		got = append(got, it.QualifiedName)
	}
	if strings.Join(got, ",") != "alpha,zeta,mytap/alpha,local/alpha" {
		t.Errorf("order: %v", got)
	}
	items, _ = e.cat.Addons(ctx, catalog.Filter{Name: "alpha"})
	if len(items) != 3 {
		t.Errorf("name filter: %d items", len(items))
	}
	items, _ = e.cat.Addons(ctx, catalog.Filter{Name: "mytap/alpha"})
	if len(items) != 1 || items[0].Version != "2.0.0" {
		t.Errorf("qualified name filter: %+v", items)
	}
	items, _ = e.cat.Addons(ctx, catalog.Filter{Source: "local"})
	if len(items) != 1 || items[0].ID != "local--alpha" {
		t.Errorf("source filter: %+v", items)
	}
}

func TestCategoryFilterAndCounts(t *testing.T) {
	e := setup(t, "off")
	ctx := context.Background()
	withCategory := func(name, category string, tags ...string) definition.Addon {
		a := mptest.Addon(name, "1.0.0")
		a.Category = category
		a.Tags = tags
		return a
	}
	if err := e.cat.Import(ctx, "official", bytes.NewReader(mptest.IndexJSON("official", 1,
		withCategory("grafana", "Monitoring", "Dashboards", "metrics"), withCategory("ollama", "ai", "llm", "gpu"),
		mptest.Addon("plain", "1.0.0")))); err != nil {
		t.Fatal(err)
	}
	e.srv.set(200, mptest.IndexJSON("mytap", 1, withCategory("prom", " monitoring ", "metrics")))
	e.addTap(t)
	local := withCategory("llm", "AI", "LLM")
	if _, err := e.cat.RegisterLocal(ctx, &local); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		filter catalog.Filter
		want   string
	}{
		{catalog.Filter{Category: "ai"}, "ollama,local/llm"},
		{catalog.Filter{Category: "AI"}, "ollama,local/llm"},
		{catalog.Filter{Category: "  Monitoring "}, "grafana,mytap/prom"},
		{catalog.Filter{Category: "monitoring", Source: "mytap"}, "mytap/prom"},
		{catalog.Filter{Category: "networking"}, ""},
		{catalog.Filter{Tag: "metrics"}, "grafana,mytap/prom"},
		{catalog.Filter{Tag: " LLM "}, "ollama,local/llm"},
		{catalog.Filter{Tag: "dashboards"}, "grafana"},
		{catalog.Filter{Tag: "metrics", Category: "monitoring", Source: "official"}, "grafana"},
		{catalog.Filter{Tag: "gpu", Category: "monitoring"}, ""},
		{catalog.Filter{Tag: "llm", Name: "llm"}, "local/llm"},
		{catalog.Filter{}, "grafana,ollama,plain,mytap/prom,local/llm"},
	}
	for _, tt := range tests {
		items, err := e.cat.Addons(ctx, tt.filter)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, it := range items {
			got = append(got, it.QualifiedName)
		}
		if strings.Join(got, ",") != tt.want {
			t.Errorf("Addons(%+v) = %v, want %s", tt.filter, got, tt.want)
		}
	}

	render := func(cs []catalog.Category) string {
		var out []string
		for _, c := range cs {
			out = append(out, c.Name+"="+strconv.Itoa(c.Count))
		}
		return strings.Join(out, ",")
	}
	for src, want := range map[string]string{
		"":         "ai=2,monitoring=2",
		"official": "ai=1,monitoring=1",
		"mytap":    "monitoring=1",
		"nope":     "",
	} {
		cs, err := e.cat.Categories(ctx, src)
		if err != nil {
			t.Fatal(err)
		}
		if cs == nil {
			t.Errorf("Categories(%q) = nil, want an empty list", src)
		}
		if got := render(cs); got != want {
			t.Errorf("Categories(%q) = %s, want %s", src, got, want)
		}
	}
}

func TestSourceURLQueryIsRedacted(t *testing.T) {
	e := setup(t, "off")
	ctx := context.Background()
	e.srv.set(500, nil)
	const secret = "sig-do-not-leak"
	v, err := e.cat.AddTap(ctx, catalog.TapInput{
		Name: "mytap", URL: "http://tap.test/index.json?X-Amz-Signature=" + secret, AllowInsecure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if v.URL != "http://tap.test/index.json?<redacted>" {
		t.Errorf("url = %q", v.URL)
	}
	if v.LastError == "" {
		t.Fatal("expected the failed first sync to be recorded")
	}
	all, err := e.cat.Sources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, sv := range all {
		if strings.Contains(sv.URL, secret) || strings.Contains(sv.LastError, secret) {
			t.Errorf("%s leaks the query: url=%q error=%q", sv.Name, sv.URL, sv.LastError)
		}
	}
	// The stored URL must stay intact, or the sync would stop working.
	src, _ := e.store.GetSource(ctx, "mytap")
	if !strings.Contains(src.URL, secret) {
		t.Errorf("stored url lost its query: %q", src.URL)
	}
}

func TestAddonsCacheFollowsSnapshotChanges(t *testing.T) {
	e := setup(t, "off")
	ctx := context.Background()
	if err := e.cat.Import(ctx, "official", bytes.NewReader(mptest.IndexJSON("official", 1, mptest.Addon("a", "1.0.0")))); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if got := strings.Join(e.names(t, "official"), ","); got != "a@1.0.0" {
			t.Fatalf("round %d: %s", i, got)
		}
	}
	if err := e.cat.Import(ctx, "official", bytes.NewReader(mptest.IndexJSON("official", 2, mptest.Addon("a", "1.1.0"), mptest.Addon("b", "1.0.0")))); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(e.names(t, "official"), ","); got != "a@1.1.0,b@1.0.0" {
		t.Errorf("after import: %s", got)
	}
}

func TestAddonsConcurrent(t *testing.T) {
	e := setup(t, "off")
	ctx := context.Background()
	if err := e.cat.Import(ctx, "official", bytes.NewReader(mptest.IndexJSON("official", 1, mptest.Addon("a", "1.0.0")))); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if items, err := e.cat.Addons(ctx, catalog.Filter{}); err != nil || len(items) != 1 {
				t.Errorf("items=%d err=%v", len(items), err)
			}
		}()
	}
	wg.Wait()
}
