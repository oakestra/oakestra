package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/oakestra/oakestra/addons/internal/addons"
	"github.com/oakestra/oakestra/addons/internal/definition"
)

type consentBody struct {
	Definition      definition.Addon       `json:"definition"`
	Hash            string                 `json:"definition_hash"`
	Permissions     definition.Permissions `json:"permissions"`
	PreviousVersion *string                `json:"previous_version"`
	Grow            *bool                  `json:"permissions_grow"`
}

func (f *fixture) storedCount() int {
	f.t.Helper()
	items, err := f.st.List(context.Background(), "")
	if err != nil {
		f.t.Fatal(err)
	}
	return len(items)
}

func TestInstallConsentIsBoundToTheDefinition(t *testing.T) {
	f := newFixture(t, "root")
	f.mkt.set(def("insights", "1.0.0"))

	code, body := f.do("POST", "/api/v1/addons", f.admin, map[string]string{"name": "insights"})
	if code != 409 {
		t.Fatalf("%d %s", code, body)
	}
	seen := decode[consentBody](t, body)
	if seen.PreviousVersion != nil || seen.Grow != nil {
		t.Errorf("install body carries update fields: %s", body)
	}
	// A UI shows "definition" and sends back "definition_hash", so the two have to be the same
	// bytes, not just equivalent after decoding.
	var raw struct {
		Definition json.RawMessage `json:"definition"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw.Definition)
	if want := "sha256:" + hex.EncodeToString(sum[:]); seen.Hash != want {
		t.Errorf("definition_hash %s doesn't hash the shown definition (%s)", seen.Hash, want)
	}

	// The catalog changes after the dialog: another image, identical permissions.
	swapped := def("insights", "1.0.0")
	swapped.Services[0].Image = "ghcr.io/evil/web:1.0.0@sha256:" + strings.Repeat("ab", 32)
	f.mkt.set(swapped)

	code, body = f.do("POST", "/api/v1/addons", f.admin, map[string]string{"name": "insights", "accept_definition": seen.Hash})
	if code != 409 {
		t.Fatalf("stale hash: %d %s", code, body)
	}
	now := decode[consentBody](t, body)
	if now.Hash == seen.Hash || now.Hash != f.accept("insights")["accept_definition"] || now.Definition.Services[0].Image != swapped.Services[0].Image {
		t.Fatalf("new body = %+v", now)
	}
	if n := f.storedCount(); n != 0 {
		t.Fatalf("%d records stored with a stale hash", n)
	}
}

func TestUpdateWithSamePermissionsStillNeedsConsent(t *testing.T) {
	f := newFixture(t, "root")
	f.mkt.set(def("insights", "1.0.0"))
	a := f.install("insights")
	f.setStatus(a.ID, addons.Active)
	path := "/api/v1/addons/" + a.ID + "/update"

	next := def("insights", "1.1.0")
	next.Services[0].Image = "ghcr.io/other/web:1.1.0@sha256:" + strings.Repeat("cd", 32)
	f.mkt.set(next)

	for name, body := range map[string]any{
		"empty object": "{}",
		"wrong hash":   map[string]string{"accept_definition": def("insights", "1.0.0").Permissions.Hash()},
		"current hash": map[string]string{"accept_definition": f.mkt.addons["insights"].Permissions.Hash()},
	} {
		code, resp := f.do("POST", path, f.admin, body)
		if code != 409 {
			t.Fatalf("%s: %d %s", name, code, resp)
		}
		got := decode[consentBody](t, resp)
		if got.Grow == nil || *got.Grow || got.PreviousVersion == nil || *got.PreviousVersion != "1.0.0" || got.Hash != f.accept("insights")["accept_definition"] {
			t.Fatalf("%s: body = %s", name, resp)
		}
		if cur, _ := f.st.Get(context.Background(), a.ID); cur.Version != "1.0.0" || cur.Status != addons.Active {
			t.Fatalf("%s: refused update changed the record", name)
		}
	}

	if code, body := f.do("POST", path, f.admin, f.accept("insights")); code != 200 {
		t.Fatalf("with the hash: %d %s", code, body)
	}
	if cur, _ := f.st.Get(context.Background(), a.ID); cur.Version != "1.1.0" || cur.Definition.Services[0].Image != next.Services[0].Image {
		t.Fatalf("record = %+v", cur)
	}
}

func TestOldAcceptFieldsDontConfirm(t *testing.T) {
	f := newFixture(t, "root")
	d := def("insights", "1.0.0")
	f.mkt.set(d)

	if code, resp := f.do("POST", "/api/v1/addons", f.admin, map[string]string{"name": "insights", "accept_definition": d.Permissions.Canonical().Hash()}); code != 409 {
		t.Errorf("permissions hash as definition hash: %d %s", code, resp)
	}
	for name, body := range map[string]map[string]string{
		"old field name":                  {"name": "insights", "accept_permissions": d.Hash()},
		"old field with permissions hash": {"name": "insights", "accept_permissions": d.Permissions.Canonical().Hash()},
	} {
		if code, resp := f.do("POST", "/api/v1/addons", f.admin, body); code != 400 || !strings.Contains(string(resp), "accept_permissions") {
			t.Errorf("%s: %d %s", name, code, resp)
		}
	}
	if n := f.storedCount(); n != 0 {
		t.Fatalf("%d records stored", n)
	}
}

func TestApproveNeedsTheDefinitionHash(t *testing.T) {
	f := newFixture(t, "root")
	d := def("insights", "1.0.0")
	f.mkt.set(d)
	a := f.install("insights")
	path := "/api/v1/addons/" + a.ID + "/approve"
	_, _ = f.st.Update(context.Background(), a.ID, func(x *addons.Installed) error {
		x.Signature, x.Status = "v1:00", addons.Failed
		return nil
	})

	code, body := f.do("POST", path, f.admin, map[string]string{"accept_definition": d.Permissions.Canonical().Hash()})
	if code != 409 || !strings.Contains(string(body), "definition_hash") {
		t.Fatalf("permissions hash: %d %s", code, body)
	}
	if rec, _ := f.st.Get(context.Background(), a.ID); rec.Status != addons.Failed || testGrantKey.Verify(rec, "root") == nil {
		t.Fatal("refused approval changed the record")
	}
	if code, body := f.do("POST", path, f.admin, f.accept("insights")); code != 200 {
		t.Fatalf("definition hash: %d %s", code, body)
	}
}
