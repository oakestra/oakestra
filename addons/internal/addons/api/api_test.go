package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/oakestra/oakestra/addons/internal/addons"
	"github.com/oakestra/oakestra/addons/internal/addons/grant"
	"github.com/oakestra/oakestra/addons/internal/addons/marketplaceclient"
	"github.com/oakestra/oakestra/addons/internal/addons/store"
	"github.com/oakestra/oakestra/addons/internal/auth"
	"github.com/oakestra/oakestra/addons/internal/definition"
)

const digest = "@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

var testGrantKey = func() grant.Key {
	k, err := grant.NewKey(bytes.Repeat([]byte{7}, grant.KeySize))
	if err != nil {
		panic(err)
	}
	return k
}()

var testKey = func() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}()

func token(t *testing.T, key *rsa.PrivateKey, roles ...string) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": "u", "type": "access", "roles": roles, "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type fakeMarket struct {
	mu     sync.Mutex
	addons map[string]definition.Addon // by qualified name
	err    error
}

func (f *fakeMarket) Get(_ context.Context, src definition.Source, name string) (*definition.Addon, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	a, ok := f.addons[definition.QualifiedName(src, name)]
	if !ok {
		return nil, marketplaceclient.ErrNotFound
	}
	out := a.Clone()
	return &out, nil
}

func (f *fakeMarket) set(a definition.Addon) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a.Normalize()
	f.addons[a.QualifiedName()] = a
}

func def(name, version string) definition.Addon {
	return definition.Addon{
		Name: name, Source: definition.OfficialSource, Version: version,
		UI:       &definition.UI{Service: "web", SDK: "^1.0.0"},
		Services: []definition.Service{{ServiceName: "web", Image: "ghcr.io/x/web:" + version + digest}},
	}
}

type fixture struct {
	t     *testing.T
	st    *store.Memory
	mkt   *fakeMarket
	h     http.Handler
	admin string
	user  string
}

func newFixture(t *testing.T, plane string) *fixture {
	st := store.NewMemory()
	mkt := &fakeMarket{addons: map[string]definition.Addon{}}
	gate := auth.NewGate(nil, nil)
	gate.SetKey(&testKey.PublicKey)
	return &fixture{
		t: t, st: st, mkt: mkt,
		h:     New(Config{Store: st, Marketplace: mkt, Gate: gate, GrantKey: testGrantKey, Plane: plane}),
		admin: token(t, testKey, "Admin"), user: token(t, testKey, "Application_Provider"),
	}
}

func (f *fixture) do(method, path, tok string, body any) (int, []byte) {
	f.t.Helper()
	var rd *bytes.Reader
	switch b := body.(type) {
	case nil:
		rd = bytes.NewReader(nil)
	case string:
		rd = bytes.NewReader([]byte(b))
	default:
		raw, _ := json.Marshal(b)
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rd)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func decode[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("decoding %s: %v", b, err)
	}
	return v
}

// install installs name and returns the record.
func (f *fixture) install(name string) addonResponse {
	f.t.Helper()
	q := f.mkt.addons[name]
	code, body := f.do("POST", "/api/v1/addons", f.admin, map[string]string{"name": name, "accept_definition": q.Hash()})
	if code != 201 {
		f.t.Fatalf("install = %d %s", code, body)
	}
	return decode[addonResponse](f.t, body)
}

// accept is the body that confirms what the marketplace currently serves for name.
func (f *fixture) accept(name string) map[string]string {
	q := f.mkt.addons[name]
	return map[string]string{"accept_definition": q.Hash()}
}

func (f *fixture) setStatus(id string, s addons.Status) {
	f.t.Helper()
	if _, err := f.st.Update(context.Background(), id, func(a *addons.Installed) error { a.Status = s; return nil }); err != nil {
		f.t.Fatal(err)
	}
}

func TestHealth(t *testing.T) {
	f := newFixture(t, "root")
	if code, body := f.do("GET", "/", "", nil); code != 200 || string(body) != "ok" {
		t.Fatalf("%d %s", code, body)
	}
}

func TestAuthEnforced(t *testing.T) {
	f := newFixture(t, "root")
	f.mkt.set(def("insights", "1.0.0"))
	a := f.install("insights")
	other, _ := rsa.GenerateKey(rand.Reader, 2048)

	admin := []struct{ method, path string }{
		{"POST", "/api/v1/addons"},
		{"POST", "/api/v1/addons/" + a.ID + "/update"},
		{"DELETE", "/api/v1/addons/" + a.ID},
	}
	for _, c := range admin {
		if code, _ := f.do(c.method, c.path, "", "{}"); code != 401 {
			t.Errorf("%s %s without token = %d", c.method, c.path, code)
		}
		if code, _ := f.do(c.method, c.path, token(t, other, "Admin"), "{}"); code != 401 {
			t.Errorf("%s %s with a forged token = %d", c.method, c.path, code)
		}
		if code, _ := f.do(c.method, c.path, f.user, "{}"); code != 403 {
			t.Errorf("%s %s as non-admin = %d", c.method, c.path, code)
		}
	}
	for _, p := range []string{"/api/v1/addons", "/api/v1/addons/" + a.ID} {
		if code, _ := f.do("GET", p, "", nil); code != 401 {
			t.Errorf("GET %s without token = %d", p, code)
		}
		if code, _ := f.do("GET", p, f.user, nil); code != 200 {
			t.Errorf("GET %s as user = %d", p, code)
		}
	}
	if code, _ := f.do("GET", "/api/v1/ui-addons", "", nil); code != 200 {
		t.Errorf("ui-addons needs no token, got %d", code)
	}
}

func TestNoPatchEndpoint(t *testing.T) {
	f := newFixture(t, "root")
	f.mkt.set(def("insights", "1.0.0"))
	a := f.install("insights")
	if code, _ := f.do("PATCH", "/api/v1/addons/"+a.ID, f.admin, `{"status":"installing"}`); code != http.StatusMethodNotAllowed {
		t.Fatalf("PATCH = %d", code)
	}
}

func TestUnavailableUntilKeyLoaded(t *testing.T) {
	gate := auth.NewGate(nil, nil)
	h := New(Config{Store: store.NewMemory(), Marketplace: &fakeMarket{}, Gate: gate, GrantKey: testGrantKey, Plane: "root"})
	for _, tok := range []string{"", token(t, testKey, "Admin")} {
		req := httptest.NewRequest("POST", "/api/v1/addons", strings.NewReader("{}"))
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 503 {
			t.Errorf("before key load: %d", rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/ui-addons", nil))
	if rec.Code != 200 {
		t.Errorf("ui-addons before key load: %d", rec.Code)
	}
}

type countingFetcher struct {
	key   *rsa.PublicKey
	calls int
}

func (c *countingFetcher) Fetch(context.Context) (*rsa.PublicKey, error) {
	c.calls++
	return c.key, nil
}

func TestKeyRefreshOnVerifyFailureIsRateLimited(t *testing.T) {
	newKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	fetcher := &countingFetcher{key: &newKey.PublicKey}
	now := time.Now()
	gate := auth.NewGate(fetcher.Fetch, nil)
	gate.Now = func() time.Time { return now }
	gate.SetKey(&testKey.PublicKey) // the old key, jwt_generator has restarted since
	h := New(Config{Store: store.NewMemory(), Marketplace: &fakeMarket{}, Gate: gate, GrantKey: testGrantKey, Plane: "root"})

	call := func(tok string) int {
		req := httptest.NewRequest("GET", "/api/v1/addons", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	tok := token(t, newKey, "Admin")

	// Right after loading the key, a failing token must not trigger a fetch.
	if code := call(tok); code != 401 || fetcher.calls != 0 {
		t.Fatalf("immediate: code %d, fetches %d", code, fetcher.calls)
	}
	now = now.Add(auth.DefaultKeyRefreshInterval + time.Second)
	if code := call(tok); code != 200 || fetcher.calls != 1 {
		t.Fatalf("after interval: code %d, fetches %d", code, fetcher.calls)
	}
	// Garbage tokens within the interval don't fetch again.
	for range 5 {
		call("garbage")
	}
	if fetcher.calls != 1 {
		t.Fatalf("fetches = %d", fetcher.calls)
	}
}

func TestInstallFlow(t *testing.T) {
	f := newFixture(t, "root")
	d := def("insights", "1.0.0")
	d.Permissions = definition.Permissions{HostPorts: []int{8080}}
	d.Services[0].Ports = map[string]string{"80": "8080"}
	f.mkt.set(d)
	pHash := d.Permissions.Canonical().Hash()
	hash := d.Hash()

	// Without the hash, the admin gets the definition and permissions to look at.
	code, body := f.do("POST", "/api/v1/addons", f.admin, map[string]string{"name": "insights"})
	if code != 409 {
		t.Fatalf("no hash: %d %s", code, body)
	}
	resp := decode[struct {
		Message     string                 `json:"message"`
		Definition  definition.Addon       `json:"definition"`
		Hash        string                 `json:"definition_hash"`
		Permissions definition.Permissions `json:"permissions"`
	}](t, body)
	if resp.Hash != hash || len(resp.Permissions.HostPorts) != 1 || resp.Permissions.HostPorts[0] != 8080 || resp.Message == "" {
		t.Fatalf("response = %+v", resp)
	}
	if resp.Definition.Hash() != resp.Hash || resp.Definition.Services[0].Image != d.Services[0].Image {
		t.Fatalf("definition does not match its hash: %+v", resp.Definition)
	}
	if strings.Contains(string(body), "permissions_hash") {
		t.Fatalf("body still has permissions_hash: %s", body)
	}

	// A hash of something else is refused the same way, and so is the hash of only the
	// permissions.
	for _, wrong := range []string{definition.Permissions{}.Hash(), pHash} {
		code, _ = f.do("POST", "/api/v1/addons", f.admin, map[string]string{"name": "insights", "accept_definition": wrong})
		if code != 409 {
			t.Fatalf("wrong hash: %d", code)
		}
	}
	if items, _ := f.st.List(context.Background(), ""); len(items) != 0 {
		t.Fatal("stored without confirmation")
	}

	code, body = f.do("POST", "/api/v1/addons/", f.admin, map[string]string{"name": "oakestra/insights", "accept_definition": resp.Hash})
	if code != 201 {
		t.Fatalf("install: %d %s", code, body)
	}
	a := decode[addonResponse](t, body)
	if a.Status != addons.Installing || a.AddonID != "insights" || a.QualifiedName != "insights" || a.Version != "1.0.0" ||
		len(a.ID) != 24 || a.Permissions.HostPorts[0] != 8080 || a.UpdateAvailable || a.Source != definition.OfficialSource {
		t.Fatalf("addon = %+v", a)
	}

	// Duplicate, also without confirming anything.
	if code, _ = f.do("POST", "/api/v1/addons", f.admin, map[string]string{"name": "insights"}); code != 409 {
		t.Fatalf("duplicate: %d", code)
	}
	stored, _ := f.st.Get(context.Background(), a.ID)
	if stored.Granted.Hash() != pHash || stored.Definition.Hash() != hash || stored.Definition.Services[0].Image != d.Services[0].Image {
		t.Error("snapshot or grant differ from what was confirmed")
	}
}

func TestInstallErrors(t *testing.T) {
	f := newFixture(t, "root")

	bad := def("evil", "1.0.0")
	bad.Services[0].Volumes = []string{"/:/host"}
	f.mkt.set(bad)
	unpinned := def("loose", "1.0.0")
	unpinned.Services[0].Image = "ghcr.io/x/web:latest"
	f.mkt.set(unpinned)

	for _, tc := range []struct {
		name string
		body any
		want int
	}{
		{"unknown", map[string]string{"name": "nothing"}, 404},
		{"invalid name", map[string]string{"name": "Bad Name"}, 400},
		{"empty body", "", 400},
		{"not an object", "[]", 400},
		// The marketplace says nothing about permissions here, the engine works them out itself.
		{"undeclared host mount", map[string]string{"name": "evil", "accept_definition": def("x", "1.0.0").Permissions.Hash()}, 422},
		{"unpinned image", map[string]string{"name": "loose"}, 422},
		{"oversize body", `{"name":"` + strings.Repeat("a", MaxBodyBytes) + `"}`, 413},
	} {
		if code, body := f.do("POST", "/api/v1/addons", f.admin, tc.body); code != tc.want {
			t.Errorf("%s: %d %s, want %d", tc.name, code, body, tc.want)
		}
	}

	f.mkt.err = marketplaceclient.ErrUnavailable
	if code, _ := f.do("POST", "/api/v1/addons", f.admin, map[string]string{"name": "evil"}); code != 502 {
		t.Errorf("marketplace down: %d", code)
	}
	if items, _ := f.st.List(context.Background(), ""); len(items) != 0 {
		t.Error("something was stored")
	}
}

func TestInstallRechecksNotTheMarketplacesWord(t *testing.T) {
	// The marketplace could claim anything about permissions; only the definition counts. This
	// one declares nothing but publishes a port, so the hash the admin sees must be for the
	// declared (empty) set and the install must fail the policy.
	f := newFixture(t, "root")
	d := def("sneaky", "1.0.0")
	d.Services[0].Ports = map[string]string{"80": "80"}
	f.mkt.set(d)
	code, body := f.do("POST", "/api/v1/addons", f.admin, map[string]string{"name": "sneaky", "accept_definition": d.Hash()})
	if code != 422 || !strings.Contains(string(body), "host_ports") {
		t.Fatalf("%d %s", code, body)
	}
}

func TestUpdateFlows(t *testing.T) {
	f := newFixture(t, "root")
	f.mkt.set(def("insights", "1.0.0"))
	a := f.install("insights")
	path := "/api/v1/addons/" + a.ID + "/update"

	// Not while installing.
	if code, _ := f.do("POST", path, f.admin, f.accept("insights")); code != 409 {
		t.Fatalf("while installing: %d", code)
	}
	f.setStatus(a.ID, addons.Active)

	// Same version.
	if code, body := f.do("POST", path, f.admin, f.accept("insights")); code != 409 || !strings.Contains(string(body), "newer") {
		t.Fatalf("same version: %d %s", code, body)
	}

	// A newer version with the same permissions still needs the hash of its definition.
	f.mkt.set(def("insights", "1.1.0"))
	code, body := f.do("POST", path, f.admin, f.accept("insights"))
	if code != 200 {
		t.Fatalf("update: %d %s", code, body)
	}
	got := decode[addonResponse](t, body)
	if got.Status != addons.Updating || got.Version != "1.1.0" || !strings.Contains(got.Services[0].Image, ":1.1.0") {
		t.Fatalf("after update: %+v", got)
	}
	// A second update while updating is refused.
	if code, _ = f.do("POST", path, f.admin, f.accept("insights")); code != 409 {
		t.Fatalf("while updating: %d", code)
	}
	f.setStatus(a.ID, addons.Active)

	// Permission growth is flagged in the consent response.
	grown := def("insights", "1.2.0")
	grown.Permissions = definition.Permissions{HostNetwork: true}
	grown.Services[0].Networks = []string{"host"}
	grown.UI = nil
	f.mkt.set(grown)
	code, body = f.do("POST", path, f.admin, "{}")
	if code != 409 {
		t.Fatalf("growth without hash: %d %s", code, body)
	}
	resp := decode[struct {
		Definition      definition.Addon       `json:"definition"`
		Permissions     definition.Permissions `json:"permissions"`
		Hash            string                 `json:"definition_hash"`
		PreviousVersion string                 `json:"previous_version"`
		Previous        definition.Permissions `json:"previous_permissions"`
		Grow            bool                   `json:"permissions_grow"`
	}](t, body)
	if !resp.Permissions.HostNetwork || resp.Previous.HostNetwork || !resp.Grow || resp.PreviousVersion != "1.1.0" ||
		resp.Hash != f.accept("insights")["accept_definition"] || resp.Definition.Hash() != resp.Hash {
		t.Fatalf("response = %+v", resp)
	}
	if cur, _ := f.st.Get(context.Background(), a.ID); cur.Version != "1.1.0" || cur.Status != addons.Active {
		t.Fatal("refused update changed the record")
	}
	if code, _ = f.do("POST", path, f.admin, map[string]string{"accept_definition": definition.Permissions{}.Hash()}); code != 409 {
		t.Fatalf("growth with the old hash: %d", code)
	}
	if code, body = f.do("POST", path, f.admin, map[string]string{"accept_definition": resp.Hash}); code != 200 {
		t.Fatalf("growth with hash: %d %s", code, body)
	}
	cur, _ := f.st.Get(context.Background(), a.ID)
	if cur.Version != "1.2.0" || !cur.Granted.HostNetwork || cur.Status != addons.Updating {
		t.Fatalf("record = %+v", cur)
	}
}

func TestUpdateShrinkingPermissionsStillNeedsHash(t *testing.T) {
	f := newFixture(t, "root")
	d := def("insights", "1.0.0")
	d.Permissions = definition.Permissions{HostPorts: []int{8080}}
	d.Services[0].Ports = map[string]string{"80": "8080"}
	f.mkt.set(d)
	a := f.install("insights")
	f.setStatus(a.ID, addons.Active)
	f.mkt.set(def("insights", "1.0.1"))
	if code, body := f.do("POST", "/api/v1/addons/"+a.ID+"/update", f.admin, f.accept("insights")); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if cur, _ := f.st.Get(context.Background(), a.ID); len(cur.Granted.HostPorts) != 0 {
		t.Errorf("granted = %+v, should have shrunk", cur.Granted)
	}
}

func TestUpdateUnknownAndMarketplaceErrors(t *testing.T) {
	f := newFixture(t, "root")
	if code, _ := f.do("POST", "/api/v1/addons/nope/update", f.admin, "{}"); code != 404 {
		t.Errorf("unknown id: %d", code)
	}
	f.mkt.set(def("insights", "1.0.0"))
	a := f.install("insights")
	f.setStatus(a.ID, addons.Active)
	f.mkt.err = marketplaceclient.ErrUnavailable
	if code, _ := f.do("POST", "/api/v1/addons/"+a.ID+"/update", f.admin, f.accept("insights")); code != 502 {
		t.Errorf("marketplace down: %d", code)
	}
	f.mkt.err = nil
	delete(f.mkt.addons, "insights")
	if code, _ := f.do("POST", "/api/v1/addons/"+a.ID+"/update", f.admin, "{}"); code != 404 {
		t.Errorf("delisted: %d", code)
	}
}

func TestDeleteSemantics(t *testing.T) {
	f := newFixture(t, "root")
	f.mkt.set(def("insights", "1.0.0"))
	a := f.install("insights")
	f.setStatus(a.ID, addons.Active)

	code, body := f.do("DELETE", "/api/v1/addons/"+a.ID, f.admin, nil)
	if code != 200 || decode[addonResponse](t, body).Status != addons.Disabling {
		t.Fatalf("first delete: %d %s", code, body)
	}
	if code, _ = f.do("DELETE", "/api/v1/addons/"+a.ID, f.admin, nil); code != 200 {
		t.Fatalf("delete while disabling: %d", code)
	}
	f.setStatus(a.ID, addons.Disabled)
	if code, _ = f.do("DELETE", "/api/v1/addons/"+a.ID, f.admin, nil); code != 204 {
		t.Fatalf("delete disabled: %d", code)
	}
	if code, _ = f.do("GET", "/api/v1/addons/"+a.ID, f.user, nil); code != 404 {
		t.Fatalf("get deleted: %d", code)
	}
	if code, _ = f.do("DELETE", "/api/v1/addons/"+a.ID, f.admin, nil); code != 404 {
		t.Fatalf("delete twice: %d", code)
	}
}

func TestListFilterAndUpdateAvailable(t *testing.T) {
	f := newFixture(t, "root")
	withMeta := def("insights", "1.0.0")
	withMeta.DisplayName = "Insights"
	withMeta.Category = "observability"
	withMeta.Tags = []string{"metrics"}
	withMeta.Icon = "https://cdn.example.com/insights.svg"
	withMeta.Metadata = map[string]string{"example.com/id": "7"}
	f.mkt.set(withMeta)
	f.mkt.set(def("other", "1.0.0"))
	a := f.install("insights")
	b := f.install("other")
	f.setStatus(a.ID, addons.Active)
	_, _ = f.st.Update(context.Background(), a.ID, func(x *addons.Installed) error { x.LatestVersion = "1.0.1"; return nil })
	_, _ = f.st.Update(context.Background(), b.ID, func(x *addons.Installed) error { x.LatestVersion = "1.0.0"; return nil })

	code, body := f.do("GET", "/api/v1/addons?status=active", f.user, nil)
	list := decode[[]addonResponse](t, body)
	if code != 200 || len(list) != 1 || list[0].ID != a.ID || !list[0].UpdateAvailable || list[0].LatestVersion != "1.0.1" {
		t.Fatalf("%d %s", code, body)
	}
	if m := list[0]; m.DisplayName != "Insights" || m.Category != "observability" || len(m.Tags) != 1 || m.Icon == "" || m.Metadata["example.com/id"] != "7" {
		t.Errorf("display fields missing: %s", body)
	}
	_, body = f.do("GET", "/api/v1/addons", f.user, nil)
	all := decode[[]addonResponse](t, body)
	if len(all) != 2 || all[1].UpdateAvailable {
		t.Fatalf("%s", body)
	}
	if code, _ = f.do("GET", "/api/v1/addons?status=bogus", f.user, nil); code != 400 {
		t.Fatalf("bad status: %d", code)
	}
	// "_id" is the key the dashboard reads.
	var raw []map[string]any
	_ = json.Unmarshal(body, &raw)
	for _, k := range []string{"_id", "qualified_name", "addon_id", "source", "version", "status", "status_details", "permissions", "created_at", "updated_at", "update_available", "latest_version"} {
		if _, ok := raw[0][k]; !ok {
			t.Errorf("response lacks %q", k)
		}
	}
	if string(decode[map[string]json.RawMessage](t, mustJSON(raw[0]))["status_details"]) != "{}" {
		t.Error("status_details should serialize as {}")
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func TestUIAddons(t *testing.T) {
	f := newFixture(t, "root")
	tagged := def("insights", "1.4.2")
	tagged.DisplayName = "Insights"
	tagged.Icon = "https://cdn.example.com/insights.svg"
	tagged.Category = "observability"
	f.mkt.set(tagged)
	f.mkt.set(def("inactive", "1.0.0"))
	noUI := def("plain", "1.0.0")
	noUI.UI = nil
	f.mkt.set(noUI)
	tap := def("tool", "2.0.0")
	tap.Source = definition.TapSource("acme")
	tap.Services[0].Image = "ghcr.io/x/web" + digest
	f.mkt.set(tap)

	ids := map[string]string{}
	for _, n := range []string{"insights", "inactive", "plain", "acme/tool"} {
		ids[n] = f.install(n).ID
	}
	for _, n := range []string{"insights", "plain", "acme/tool"} {
		f.setStatus(ids[n], addons.Active)
	}

	code, body := f.do("GET", "/api/v1/ui-addons/", "", nil)
	if code != 200 {
		t.Fatal(code)
	}
	want := `[{"name":"insights","version":"1.4.2","sdk":"^1.0.0","display_name":"Insights","icon":"https://cdn.example.com/insights.svg"},` +
		`{"name":"acme--tool","version":"2.0.0","sdk":"^1.0.0"}]`
	if strings.TrimSpace(string(body)) != want {
		t.Fatalf("ui-addons = %s\nwant %s", body, want)
	}
}

func TestUIAddonsEmptyAndCluster(t *testing.T) {
	f := newFixture(t, "root")
	if _, body := f.do("GET", "/api/v1/ui-addons", "", nil); strings.TrimSpace(string(body)) != "[]" {
		t.Fatalf("empty = %s", body)
	}
	c := newFixture(t, "cluster")
	c.mkt.set(def("insights", "1.0.0"))
	a := c.install("insights")
	c.setStatus(a.ID, addons.Active)
	if _, body := c.do("GET", "/api/v1/ui-addons", "", nil); strings.TrimSpace(string(body)) != "[]" {
		t.Fatalf("cluster plane served UI addons: %s", body)
	}
}

func TestCORSPreflight(t *testing.T) {
	f := newFixture(t, "root")
	req := httptest.NewRequest("OPTIONS", "/api/v1/addons", nil)
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "*" || !strings.Contains(rec.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
}
