package api

import (
	"context"
	"strings"
	"testing"

	"github.com/oakestra/oakestra/addons/internal/addons"
	"github.com/oakestra/oakestra/addons/internal/addons/reconcile"
	"github.com/oakestra/oakestra/addons/internal/addons/runtime"
	"github.com/oakestra/oakestra/addons/internal/definition"
)

func TestInstallAndUpdateAreSigned(t *testing.T) {
	f := newFixture(t, "root")
	f.mkt.set(def("insights", "1.0.0"))
	a := f.install("insights")
	rec, _ := f.st.Get(context.Background(), a.ID)
	if err := testGrantKey.Verify(rec, "root"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := testGrantKey.Verify(rec, "cluster"); err == nil {
		t.Fatal("signature is not bound to the plane")
	}

	f.setStatus(a.ID, addons.Active)
	f.mkt.set(def("insights", "1.1.0"))
	if code, body := f.do("POST", "/api/v1/addons/"+a.ID+"/update", f.admin, "{}"); code != 200 {
		t.Fatalf("update: %d %s", code, body)
	}
	rec, _ = f.st.Get(context.Background(), a.ID)
	if err := testGrantKey.Verify(rec, "root"); err != nil || rec.Version != "1.1.0" {
		t.Fatalf("update: %v %s", err, rec.Version)
	}
}

// The local registration path end to end: the marketplace serves a local addon, the API
// installs it and the reconcile loop runs it. Checking with ModeIndex anywhere on that path
// would reject the local source.
func TestInstallLocalAddonEndToEnd(t *testing.T) {
	f := newFixture(t, "root")
	d := def("mytool", "0.1.0")
	d.Source = definition.LocalSource
	f.mkt.set(d)
	a := f.install("local/mytool")
	if a.Source != definition.LocalSource || a.AddonID != "local--mytool" {
		t.Fatalf("addon = %+v", a)
	}

	rt := runtime.NewFake()
	r := &reconcile.Reconciler{Store: f.st, Runtime: rt, Plane: "root", Instance: strings.Repeat("a", 32), GrantKey: testGrantKey}
	r.ReconcileOnce(context.Background())
	got, _ := f.st.Get(context.Background(), a.ID)
	if got.Status != addons.Active {
		t.Fatalf("status = %s %+v", got.Status, got.StatusDetails)
	}
	if _, ok := rt.Containers["root_addon-local--mytool--web"]; !ok {
		t.Errorf("containers = %v", rt.Names())
	}
}

func withHostPath(name, version, path string) definition.Addon {
	d := def(name, version)
	d.UI = nil
	d.Services[0].Volumes = []string{path + ":/data"}
	d.Permissions = definition.Permissions{HostPaths: []string{path}}
	return d
}

func TestInstallRefusesOverlappingHostPaths(t *testing.T) {
	f := newFixture(t, "root")
	f.mkt.set(withHostPath("writer", "1.0.0", "/srv/shared"))
	f.mkt.set(withHostPath("nested", "1.0.0", "/srv/shared/app"))
	f.mkt.set(withHostPath("apart", "1.0.0", "/srv/other"))
	w := f.install("writer")

	n := f.mkt.addons["nested"]
	code, body := f.do("POST", "/api/v1/addons", f.admin, map[string]string{"name": "nested", "accept_permissions": n.Permissions.Canonical().Hash()})
	if code != 409 || !strings.Contains(string(body), "overlaps") || !strings.Contains(string(body), "writer") {
		t.Fatalf("overlap: %d %s", code, body)
	}
	f.install("apart")

	// A disabled addon doesn't count any more.
	f.setStatus(w.ID, addons.Disabled)
	f.install("nested")
}

func TestUpdateRefusesPathBelowOldReadWritePath(t *testing.T) {
	f := newFixture(t, "root")
	f.mkt.set(withHostPath("tool", "1.0.0", "/srv/tool"))
	a := f.install("tool")
	f.setStatus(a.ID, addons.Active)

	next := withHostPath("tool", "1.1.0", "/srv/tool/conf")
	f.mkt.set(next)
	hash := next.Permissions.Canonical().Hash()
	code, body := f.do("POST", "/api/v1/addons/"+a.ID+"/update", f.admin, map[string]string{"accept_permissions": hash})
	if code != 409 || !strings.Contains(string(body), "previous version") {
		t.Fatalf("%d %s", code, body)
	}
	if cur, _ := f.st.Get(context.Background(), a.ID); cur.Version != "1.0.0" {
		t.Fatal("refused update changed the record")
	}

	// The same path again is just the data directory carried over.
	f.mkt.set(withHostPath("tool", "1.2.0", "/srv/tool"))
	if code, body := f.do("POST", "/api/v1/addons/"+a.ID+"/update", f.admin, "{}"); code != 200 {
		t.Fatalf("same path: %d %s", code, body)
	}
}

func TestUpdateRefusesOverlapWithOtherAddon(t *testing.T) {
	f := newFixture(t, "root")
	f.mkt.set(withHostPath("writer", "1.0.0", "/srv/shared"))
	f.mkt.set(def("tool", "1.0.0"))
	f.install("writer")
	a := f.install("tool")
	f.setStatus(a.ID, addons.Active)
	next := withHostPath("tool", "1.1.0", "/srv/shared/x")
	f.mkt.set(next)
	code, body := f.do("POST", "/api/v1/addons/"+a.ID+"/update", f.admin, map[string]string{"accept_permissions": next.Permissions.Canonical().Hash()})
	if code != 409 || !strings.Contains(string(body), "writer") {
		t.Fatalf("%d %s", code, body)
	}
}

func TestApproveResignsAfterTampering(t *testing.T) {
	f := newFixture(t, "root")
	d := def("insights", "1.0.0")
	f.mkt.set(d)
	a := f.install("insights")
	path := "/api/v1/addons/" + a.ID + "/approve"

	// Someone rewrote the record in the database; the loop failed it.
	_, _ = f.st.Update(context.Background(), a.ID, func(x *addons.Installed) error {
		x.Definition.Services[0].Volumes = []string{"/var/run/docker.sock:/s"}
		x.Definition.Permissions = definition.Permissions{DockerSocket: true, HostRoot: true, HostPaths: []string{"/var/run/docker.sock"}}
		x.Granted = x.Definition.Permissions
		x.Status = addons.Failed
		return nil
	})
	hash := d.Permissions.Canonical().Hash()
	code, body := f.do("POST", path, f.admin, map[string]string{"accept_permissions": hash})
	if code != 409 || !strings.Contains(string(body), "differs") {
		t.Fatalf("tampered snapshot approved: %d %s", code, body)
	}

	// Put the honest snapshot back but without a valid signature, e.g. after the state
	// directory was lost.
	_, _ = f.st.Update(context.Background(), a.ID, func(x *addons.Installed) error {
		dd := f.mkt.addons["insights"]
		dd.Permissions = dd.Permissions.Canonical()
		x.Definition, x.Granted, x.Signature = dd, dd.Permissions, "v1:00"
		return nil
	})
	code, body = f.do("POST", path, f.admin, "{}")
	if code != 409 || !strings.Contains(string(body), "permissions_hash") {
		t.Fatalf("no hash: %d %s", code, body)
	}
	if code, _ := f.do("POST", path, f.user, map[string]string{"accept_permissions": hash}); code != 403 {
		t.Fatalf("non-admin: %d", code)
	}
	code, body = f.do("POST", path, f.admin, map[string]string{"accept_permissions": hash})
	if code != 200 {
		t.Fatalf("approve: %d %s", code, body)
	}
	got := decode[addonResponse](t, body)
	if got.Status != addons.Installing {
		t.Fatalf("status = %s", got.Status)
	}
	rec, _ := f.st.Get(context.Background(), a.ID)
	if err := testGrantKey.Verify(rec, "root"); err != nil {
		t.Fatalf("not re-signed: %v", err)
	}

	// Not while installing, and not for a version the marketplace no longer serves.
	if code, _ := f.do("POST", path, f.admin, map[string]string{"accept_permissions": hash}); code != 409 {
		t.Fatalf("while installing: %d", code)
	}
	f.setStatus(a.ID, addons.Active)
	f.mkt.set(def("insights", "1.1.0"))
	if code, body := f.do("POST", path, f.admin, map[string]string{"accept_permissions": hash}); code != 409 || !strings.Contains(string(body), "update") {
		t.Fatalf("other version: %d %s", code, body)
	}
	if code, _ := f.do("POST", "/api/v1/addons/000000000000000000000099/approve", f.admin, "{}"); code != 404 {
		t.Fatalf("unknown: %d", code)
	}
}

func TestLogsOnlyForAdmins(t *testing.T) {
	f := newFixture(t, "root")
	f.mkt.set(def("insights", "1.0.0"))
	a := f.install("insights")
	_, _ = f.st.Update(context.Background(), a.ID, func(x *addons.Installed) error {
		x.Status, x.Logs = addons.PartiallyActive, "DB_PASSWORD=hunter2"
		x.StatusDetails = addons.StatusDetails{FailedServices: []string{"web"}}
		return nil
	})
	for _, p := range []string{"/api/v1/addons", "/api/v1/addons/" + a.ID} {
		if _, body := f.do("GET", p, f.user, nil); strings.Contains(string(body), "hunter2") || strings.Contains(string(body), `"logs"`) {
			t.Errorf("GET %s as user leaked logs: %s", p, body)
		}
		if _, body := f.do("GET", p, f.admin, nil); !strings.Contains(string(body), "hunter2") {
			t.Errorf("GET %s as admin has no logs: %s", p, body)
		}
	}
}
