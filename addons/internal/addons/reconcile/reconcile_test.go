package reconcile

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oakestra/oakestra/addons/internal/addons"
	"github.com/oakestra/oakestra/addons/internal/addons/grant"
	"github.com/oakestra/oakestra/addons/internal/addons/runtime"
	"github.com/oakestra/oakestra/addons/internal/addons/store"
	"github.com/oakestra/oakestra/addons/internal/definition"
)

const digest = "@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func img(name string) string { return "ghcr.io/x/" + name + digest }

func baseDef() definition.Addon {
	d := definition.Addon{
		Name: "insights", Source: definition.OfficialSource, Version: "1.0.0",
		UI: &definition.UI{Service: "web", SDK: "^1.0.0"},
		Services: []definition.Service{
			{ServiceName: "web", Image: img("web"), Volumes: []string{"data:/data"}, Networks: []string{"backend"}},
			{ServiceName: "worker", Image: img("worker")},
		},
		Volumes:  []definition.Volume{{Name: "data"}},
		Networks: []definition.Network{{Name: "backend"}},
	}
	d.Normalize()
	return d
}

type env struct {
	t  *testing.T
	st *store.Memory
	rt *runtime.Fake
	r  *Reconciler
}

const instance = "0123456789abcdef0123456789abcdef"

func testKey(t *testing.T, fill byte) grant.Key {
	t.Helper()
	k, err := grant.NewKey(bytes.Repeat([]byte{fill}, grant.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func newEnv(t *testing.T, plane string) *env {
	rt := runtime.NewFake()
	st := store.NewMemory()
	return &env{t: t, st: st, rt: rt, r: &Reconciler{Store: st, Runtime: rt, Plane: plane, Instance: instance, GrantKey: testKey(t, 1)}}
}

// install stores an honest, signed record for def in the given status, like the API does.
func (e *env) install(def definition.Addon, status addons.Status) addons.Installed {
	e.t.Helper()
	perms := def.Permissions.Canonical()
	def.Permissions = perms
	rec := addons.Installed{
		ID: store.NewID(), AddonID: def.ID(), QualifiedName: def.QualifiedName(), Source: def.Source, Version: def.Version,
		Definition: def, Granted: perms, Status: status,
	}
	rec.Signature = e.r.GrantKey.Sign(rec, e.r.Plane)
	a, err := e.st.Insert(context.Background(), rec)
	if err != nil {
		e.t.Fatal(err)
	}
	return a
}

// edit changes a stored record and signs the result, i.e. a change the API made or approved.
func (e *env) edit(id string, fn func(*addons.Installed)) {
	e.t.Helper()
	_, err := e.st.Update(context.Background(), id, func(x *addons.Installed) error {
		fn(x)
		x.Signature = e.r.GrantKey.Sign(*x, e.r.Plane)
		return nil
	})
	if err != nil {
		e.t.Fatal(err)
	}
}

// own are the labels the engine under test puts on its objects.
func own(extra map[string]string) map[string]string {
	l := map[string]string{runtime.LabelPlane: "root", runtime.LabelInstance: instance}
	for k, v := range extra {
		l[k] = v
	}
	return l
}

func (e *env) get(id string) addons.Installed {
	e.t.Helper()
	a, err := e.st.Get(context.Background(), id)
	if err != nil {
		e.t.Fatal(err)
	}
	return a
}

func (e *env) reconcile() { e.r.ReconcileOnce(context.Background()) }

func TestInstallCreatesNamespacedObjects(t *testing.T) {
	e := newEnv(t, "root")
	e.rt.Networks[definition.UINetwork] = runtime.NetworkSpec{Name: definition.UINetwork}
	a := e.install(baseDef(), addons.Installing)

	e.reconcile()

	got := e.get(a.ID)
	if got.Status != addons.Active {
		t.Fatalf("status = %s (%+v)", got.Status, got.StatusDetails)
	}
	names := e.rt.Names()
	want := []string{"root_addon-insights--web", "root_addon-insights--worker"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("containers = %v, want %v", names, want)
	}

	web := e.rt.Containers["root_addon-insights--web"].Spec
	if web.Labels[runtime.LabelAddonID] != a.ID || web.Labels[runtime.LabelAddonKey] != "insights" || web.Labels[runtime.LabelPlane] != "root" ||
		web.Labels[runtime.LabelInstance] != instance {
		t.Errorf("labels = %v", web.Labels)
	}
	if len(web.Mounts) != 1 || web.Mounts[0].Source != "root_addon-insights_data" || web.Mounts[0].Bind {
		t.Errorf("named volume was not rewritten: %+v", web.Mounts)
	}
	if _, ok := e.rt.Volumes["root_addon-insights_data"]; !ok {
		t.Errorf("volume not created under the namespaced name: %v", e.rt.Volumes)
	}
	if _, ok := e.rt.Networks["root_addon-insights_backend"]; !ok {
		t.Errorf("declared network not created under the namespaced name: %v", e.rt.Networks)
	}
	if e.rt.Networks["root_addon-insights_backend"].Labels[runtime.LabelAddonID] != a.ID {
		t.Error("network lacks the owner label")
	}

	// The UI service joins its own network plus the UI network with the alias.
	var sawUI bool
	for _, n := range web.Networks {
		if n.Name == definition.UINetwork {
			sawUI = true
			if len(n.Aliases) != 1 || n.Aliases[0] != "addonui-insights" {
				t.Errorf("aliases = %v", n.Aliases)
			}
		}
		if n.Name == definition.CoreNetwork {
			t.Error("web must not join the core network")
		}
	}
	if !sawUI || len(web.Networks) != 2 || web.Networks[0].Name != "root_addon-insights_backend" {
		t.Errorf("web networks = %+v", web.Networks)
	}

	// A service without networks gets the addon-private default, never the core network or the
	// UI network.
	worker := e.rt.Containers["root_addon-insights--worker"].Spec
	if len(worker.Networks) != 1 || worker.Networks[0].Name != "root_addon-insights_default" {
		t.Errorf("worker networks = %+v", worker.Networks)
	}
}

func TestUIAliasOnlyOnRootPlane(t *testing.T) {
	e := newEnv(t, "cluster")
	a := e.install(baseDef(), addons.Installing)
	e.reconcile()

	if got := e.get(a.ID); got.Status != addons.Active {
		t.Fatalf("status = %s", got.Status)
	}
	if _, ok := e.rt.Networks[definition.UINetwork]; ok {
		t.Error("cluster plane created the UI network")
	}
	for _, c := range e.rt.Containers {
		for _, n := range c.Spec.Networks {
			if n.Name == definition.UINetwork || len(n.Aliases) > 0 {
				t.Errorf("%s joined %+v on the cluster plane", c.Name, n)
			}
		}
	}
	if _, ok := e.rt.Containers["cluster_addon-insights--web"]; !ok {
		t.Errorf("containers = %v", e.rt.Names())
	}
}

func TestCreatesUINetworkIfMissing(t *testing.T) {
	e := newEnv(t, "root")
	e.install(baseDef(), addons.Installing)
	e.reconcile()
	if n, ok := e.rt.Networks[definition.UINetwork]; !ok || !n.Shared {
		t.Errorf("UI network = %+v, %v", n, ok)
	}
}

func TestCoreNetworkOnlyWhenListedAndPermitted(t *testing.T) {
	e := newEnv(t, "root")
	e.rt.Networks[definition.CoreNetwork] = runtime.NetworkSpec{Name: definition.CoreNetwork}
	def := baseDef()
	def.UI = nil
	def.Permissions.CoreNetwork = true
	def.Services[1].Networks = []string{definition.CoreNetwork}
	a := e.install(def, addons.Installing)
	e.reconcile()

	if got := e.get(a.ID); got.Status != addons.Active {
		t.Fatalf("status = %s %+v", got.Status, got.StatusDetails)
	}
	worker := e.rt.Containers["root_addon-insights--worker"].Spec
	if len(worker.Networks) != 1 || worker.Networks[0].Name != "oakestra" {
		t.Errorf("worker networks = %+v", worker.Networks)
	}
	if _, ok := e.rt.Networks["root_addon-insights_default"]; ok {
		t.Error("created a default network nobody uses")
	}
}

func TestHostileSnapshotCreatesNothing(t *testing.T) {
	sock := func(d *definition.Addon) {
		d.Services[0].Volumes = []string{"/var/run/docker.sock:/var/run/docker.sock"}
	}
	hostNet := func(d *definition.Addon) { d.Services[0].Networks = []string{"host"} }
	hostPort := func(d *definition.Addon) { d.Services[0].Ports = map[string]string{"80": "8080"} }
	rootBind := func(d *definition.Addon) { d.Services[0].Volumes = []string{"/:/host"} }
	tag := func(d *definition.Addon) { d.Services[0].Image = "ghcr.io/x/web:latest" }
	label := func(d *definition.Addon) { d.Services[0].Labels = map[string]string{"oak.addon.id": "other"} }
	bindVol := func(d *definition.Addon) {
		d.Volumes[0].DriverOpts = map[string]string{"type": "none", "o": "bind", "device": "/etc"}
	}
	rename := func(d *definition.Addon) { d.Name = "system-manager" }
	version := func(d *definition.Addon) { d.Version = "9.9.9" }

	for name, tamper := range map[string]func(*definition.Addon){
		"docker socket": sock, "host network": hostNet, "undeclared host port": hostPort,
		"root bind": rootBind, "unpinned image": tag, "reserved label": label,
		"bind volume": bindVol, "renamed addon": rename, "changed version": version,
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, "root")
			a := e.install(baseDef(), addons.Installing)
			// Signed, so this exercises the policy re-check behind the signature: it is what
			// still holds if the key leaks or an older engine signed something now forbidden.
			e.edit(a.ID, func(x *addons.Installed) { tamper(&x.Definition) })
			e.reconcile()

			got := e.get(a.ID)
			if got.Status != addons.Failed || got.StatusDetails.Message == "" {
				t.Fatalf("status = %s %+v", got.Status, got.StatusDetails)
			}
			if len(e.rt.Containers)+len(e.rt.Networks)+len(e.rt.Volumes)+len(e.rt.Pulled) != 0 {
				t.Errorf("runtime was touched: %v %v %v %v", e.rt.Names(), e.rt.Networks, e.rt.Volumes, e.rt.Pulled)
			}
		})
	}
}

func TestPermissionsChangedWithoutGrantFails(t *testing.T) {
	// The snapshot declares the docker socket and covers it, but the admin never granted it.
	e := newEnv(t, "root")
	a := e.install(baseDef(), addons.Installing)
	e.edit(a.ID, func(x *addons.Installed) {
		x.Definition.Services[1].Volumes = []string{"/var/run/docker.sock:/s"}
		x.Definition.Permissions.DockerSocket = true
		x.Definition.Permissions.HostPaths = []string{"/var/run/docker.sock"}
	})
	e.reconcile()

	got := e.get(a.ID)
	if got.Status != addons.Failed || !strings.Contains(got.StatusDetails.Message, "granted") {
		t.Fatalf("status = %s %+v", got.Status, got.StatusDetails)
	}
	if len(e.rt.Containers) != 0 {
		t.Errorf("containers = %v", e.rt.Names())
	}
}

func TestGrantedMustCoverSnapshot(t *testing.T) {
	// Both are edited to agree on the hash but the grant is narrower than what is used: not
	// reachable through Hash equality, so cover it via a bind the grant doesn't include.
	e := newEnv(t, "root")
	def := baseDef()
	def.Services[1].Volumes = []string{"/var/log:/logs:ro"}
	def.Permissions.HostPaths = []string{"/var/log:ro"}
	a := e.install(def, addons.Installing)
	e.reconcile()
	if got := e.get(a.ID); got.Status != addons.Active {
		t.Fatalf("honest install: status = %s %+v", got.Status, got.StatusDetails)
	}
	m := e.rt.Containers["root_addon-insights--worker"].Spec.Mounts
	if len(m) != 1 || !m[0].Bind || m[0].Source != "/var/log" || !m[0].ReadOnly {
		t.Errorf("mounts = %+v", m)
	}
}

func TestForeignContainerIsNotClobbered(t *testing.T) {
	e := newEnv(t, "root")
	core := e.rt.AddContainer("root_addon-insights--web", map[string]string{"com.docker.compose.service": "system_manager"}, "running")
	a := e.install(baseDef(), addons.Installing)
	e.reconcile()

	got := e.get(a.ID)
	if got.Status != addons.Failed {
		t.Fatalf("status = %s", got.Status)
	}
	if c := e.rt.Containers["root_addon-insights--web"]; c == nil || c.ID != core.ID {
		t.Fatal("the foreign container was removed or replaced")
	}
	if len(e.rt.Created) != 0 || len(e.rt.Networks) != 0 {
		t.Errorf("created %d containers, networks %v", len(e.rt.Created), e.rt.Networks)
	}
}

func TestContainerOfAnotherAddonIsNotClobbered(t *testing.T) {
	e := newEnv(t, "root")
	otherDef := baseDef()
	otherDef.Name = "other"
	other := e.install(otherDef, addons.Active)
	e.rt.AddContainer("root_addon-insights--web", map[string]string{runtime.LabelAddonID: other.ID, runtime.LabelPlane: "root"}, "running")
	a := e.install(baseDef(), addons.Installing)
	e.reconcile()
	if got := e.get(a.ID); got.Status != addons.Failed {
		t.Fatalf("status = %s", got.Status)
	}
	if e.rt.Containers["root_addon-insights--web"].Labels[runtime.LabelAddonID] != other.ID {
		t.Fatal("container was replaced")
	}
}

func TestPartialAndTotalFailure(t *testing.T) {
	e := newEnv(t, "root")
	a := e.install(baseDef(), addons.Installing)
	e.rt.PullErr[img("worker")] = errors.New("denied")
	e.reconcile()
	got := e.get(a.ID)
	if got.Status != addons.PartiallyActive || len(got.StatusDetails.FailedServices) != 1 || got.StatusDetails.FailedServices[0] != "worker" {
		t.Fatalf("status = %s %+v", got.Status, got.StatusDetails)
	}

	e2 := newEnv(t, "root")
	b := e2.install(baseDef(), addons.Installing)
	e2.rt.PullErr[img("worker")] = errors.New("denied")
	e2.rt.PullErr[img("web")] = errors.New("denied")
	e2.reconcile()
	if got := e2.get(b.ID); got.Status != addons.Failed {
		t.Fatalf("status = %s", got.Status)
	}
}

func TestUpdateReplacesContainers(t *testing.T) {
	e := newEnv(t, "root")
	a := e.install(baseDef(), addons.Installing)
	e.reconcile()
	oldID := e.rt.Containers["root_addon-insights--web"].ID

	next := baseDef()
	next.Version = "1.1.0"
	next.Services[0].Image = "ghcr.io/x/web@sha256:" + strings.Repeat("ab", 32)
	e.edit(a.ID, func(x *addons.Installed) {
		x.Definition, x.Version, x.Status = next, "1.1.0", addons.Updating
	})
	e.reconcile()

	got := e.get(a.ID)
	if got.Status != addons.Active {
		t.Fatalf("status = %s %+v", got.Status, got.StatusDetails)
	}
	c := e.rt.Containers["root_addon-insights--web"]
	if c.ID == oldID || !strings.Contains(c.Spec.Image, strings.Repeat("ab", 32)) {
		t.Errorf("container was not replaced: %+v", c.Spec.Image)
	}
	if len(e.rt.Containers) != 2 {
		t.Errorf("containers = %v", e.rt.Names())
	}
}

func TestFailedUpdateKeepsOldContainersOnNameClash(t *testing.T) {
	e := newEnv(t, "root")
	a := e.install(baseDef(), addons.Installing)
	e.reconcile()
	oldID := e.rt.Containers["root_addon-insights--web"].ID

	next := baseDef()
	next.Version = "1.1.0"
	next.Services = append(next.Services, definition.Service{ServiceName: "extra", Image: img("extra")})
	e.rt.AddContainer("root_addon-insights--extra", nil, "running")
	e.edit(a.ID, func(x *addons.Installed) {
		x.Definition, x.Version, x.Status = next, "1.1.0", addons.Updating
	})
	e.reconcile()

	if got := e.get(a.ID); got.Status != addons.Failed {
		t.Fatalf("status = %s", got.Status)
	}
	if c := e.rt.Containers["root_addon-insights--web"]; c == nil || c.ID != oldID {
		t.Error("the running version was removed although the update could not proceed")
	}
}

func TestDisableRemovesContainersAndNetworksKeepsVolumes(t *testing.T) {
	e := newEnv(t, "root")
	a := e.install(baseDef(), addons.Installing)
	e.reconcile()
	e.rt.AddContainer("root_unrelated", nil, "running")

	_, _ = e.st.Update(context.Background(), a.ID, func(x *addons.Installed) error { x.Status = addons.Disabling; return nil })
	e.reconcile()

	got := e.get(a.ID)
	if got.Status != addons.Disabled || got.Current {
		t.Fatalf("status = %s current = %v", got.Status, got.Current)
	}
	if names := e.rt.Names(); len(names) != 1 || names[0] != "root_unrelated" {
		t.Errorf("containers = %v", names)
	}
	for n := range e.rt.Networks {
		if strings.HasPrefix(n, "root_addon-") {
			t.Errorf("network %s survived", n)
		}
	}
	if _, ok := e.rt.Volumes["root_addon-insights_data"]; !ok {
		t.Error("volume was removed")
	}
	if _, ok := e.rt.Networks[definition.UINetwork]; !ok {
		t.Error("the shared UI network was removed")
	}

	// A disabled addon can be installed again.
	if _, err := e.st.Insert(context.Background(), addons.Installed{AddonID: "insights"}); err != nil {
		t.Errorf("reinstall after disable: %v", err)
	}
}

func TestWatchReportsAndRecovers(t *testing.T) {
	e := newEnv(t, "root")
	a := e.install(baseDef(), addons.Installing)
	e.reconcile()

	w := e.rt.Containers["root_addon-insights--worker"]
	w.State, w.ExitCode, w.Logs = "exited", 1, "boom"
	e.reconcile()
	got := e.get(a.ID)
	if got.Status != addons.PartiallyActive || got.StatusDetails.FailedServices[0] != "worker" || !strings.Contains(got.Logs, "boom") {
		t.Fatalf("status = %s %+v logs=%q", got.Status, got.StatusDetails, got.Logs)
	}

	w.State = "running"
	e.reconcile()
	if got := e.get(a.ID); got.Status != addons.Active || len(got.StatusDetails.FailedServices) != 0 {
		t.Fatalf("did not recover: %s %+v", got.Status, got.StatusDetails)
	}

	delete(e.rt.Containers, "root_addon-insights--web")
	delete(e.rt.Containers, "root_addon-insights--worker")
	e.reconcile()
	if got := e.get(a.ID); got.Status != addons.Failed {
		t.Fatalf("status = %s", got.Status)
	}
}

func TestRestartLoopCountsAsFailure(t *testing.T) {
	e := newEnv(t, "root")
	a := e.install(baseDef(), addons.Installing)
	e.reconcile()
	w := e.rt.Containers["root_addon-insights--worker"]
	w.State, w.RestartCount = "restarting", 1
	e.reconcile()
	if got := e.get(a.ID); got.Status != addons.Active {
		t.Fatalf("a first restart should not fail the addon, got %s", got.Status)
	}
	w.RestartCount = DefaultMaxRestarts
	e.reconcile()
	if got := e.get(a.ID); got.Status != addons.PartiallyActive {
		t.Fatalf("status = %s", got.Status)
	}
}

func TestOrphansAreRemovedButNotForeignOnes(t *testing.T) {
	e := newEnv(t, "root")
	a := e.install(baseDef(), addons.Installing)
	e.reconcile()

	e.rt.AddContainer("root_addon-gone--x", own(map[string]string{runtime.LabelAddonID: "deadbeef"}), "running")
	e.rt.AddContainer("cluster_addon-other--x", map[string]string{runtime.LabelAddonID: "deadbeef", runtime.LabelPlane: "cluster", runtime.LabelInstance: instance}, "running")
	// A second root deployment on the same daemon, and a container without an instance label.
	e.rt.AddContainer("root_addon-neighbour--x", map[string]string{runtime.LabelAddonID: "deadbeef", runtime.LabelPlane: "root", runtime.LabelInstance: "ffffffffffffffffffffffffffffffff"}, "running")
	e.rt.AddContainer("root_addon-old--x", map[string]string{runtime.LabelAddonID: "deadbeef", runtime.LabelPlane: "root"}, "running")
	e.rt.AddContainer("root_legacy", map[string]string{"oak.addon.id": "oldpython"}, "running")
	e.rt.AddContainer("root_system_manager", nil, "running")
	e.reconcile()

	got := e.rt.Names()
	want := "cluster_addon-other--x,root_addon-insights--web,root_addon-insights--worker,root_addon-neighbour--x,root_addon-old--x,root_legacy,root_system_manager"
	if strings.Join(got, ",") != want {
		t.Errorf("containers = %v\nwant %s", got, want)
	}
	if e.get(a.ID).Status != addons.Active {
		t.Error("live addon was disturbed")
	}
}

func TestSettleIgnoresChangedStatus(t *testing.T) {
	// A DELETE that lands while the install runs must win over the install's final status.
	e := newEnv(t, "root")
	a := e.install(baseDef(), addons.Installing)
	rt := &hookRuntime{Fake: e.rt, onCreate: func() {
		_, _ = e.st.Update(context.Background(), a.ID, func(x *addons.Installed) error { x.Status = addons.Disabling; return nil })
	}}
	e.r.Runtime = rt
	e.reconcile()
	if got := e.get(a.ID); got.Status == addons.Active {
		t.Fatalf("install overwrote the disable request: %s", got.Status)
	}
	e.reconcile()
	if got := e.get(a.ID); got.Status != addons.Disabled || len(e.rt.Containers) != 0 {
		t.Fatalf("status = %s containers = %v", got.Status, e.rt.Names())
	}
}

type hookRuntime struct {
	*runtime.Fake
	onCreate func()
	done     bool
}

func (h *hookRuntime) CreateAndStart(ctx context.Context, s runtime.ContainerSpec) error {
	if !h.done {
		h.done = true
		h.onCreate()
	}
	return h.Fake.CreateAndStart(ctx, s)
}

func TestSplitCommand(t *testing.T) {
	for in, want := range map[string][]string{
		"":                          nil,
		"  ":                        nil,
		"nginx -g 'daemon off;'":    {"nginx", "-g", "daemon off;"},
		`sh -c "echo \"hi\" there"`: {"sh", "-c", `echo "hi" there`},
		`a\ b c`:                    {"a b", "c"},
		`echo ''`:                   {"echo", ""},
	} {
		got, err := splitCommand(in)
		if err != nil || strings.Join(got, "|") != strings.Join(want, "|") || len(got) != len(want) {
			t.Errorf("splitCommand(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := splitCommand(`echo "x`); err == nil {
		t.Error("unterminated quote accepted")
	}
}

func assertUntouched(t *testing.T, rt *runtime.Fake) {
	t.Helper()
	if len(rt.Containers)+len(rt.Networks)+len(rt.Volumes)+len(rt.Pulled) != 0 {
		t.Errorf("runtime was touched: %v %v %v %v", rt.Names(), rt.Networks, rt.Volumes, rt.Pulled)
	}
}

func TestForgedGrantCreatesNothing(t *testing.T) {
	// Someone with database access rewrites the snapshot and the grant so they agree with each
	// other, which the policy re-check alone can't tell from an honest record.
	e := newEnv(t, "root")
	a := e.install(baseDef(), addons.Installing)
	_, err := e.st.Update(context.Background(), a.ID, func(x *addons.Installed) error {
		x.Definition.Services[1].Volumes = []string{"/var/run/docker.sock:/var/run/docker.sock"}
		x.Definition.Permissions.DockerSocket = true
		x.Definition.Permissions.HostRoot = true
		x.Definition.Permissions.HostPaths = []string{"/var/run/docker.sock"}
		x.Granted = x.Definition.Permissions.Canonical()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	e.reconcile()

	got := e.get(a.ID)
	if got.Status != addons.Failed || got.StatusDetails.Message != "grant signature invalid; re-approve the addon" {
		t.Fatalf("status = %s %+v", got.Status, got.StatusDetails)
	}
	assertUntouched(t, e.rt)
}

func TestSignatureFromAnotherKeyOrNoneFails(t *testing.T) {
	e := newEnv(t, "root")
	a := e.install(baseDef(), addons.Installing)
	e.r.GrantKey = testKey(t, 2)
	e.reconcile()
	if got := e.get(a.ID); got.Status != addons.Failed || !strings.Contains(got.StatusDetails.Message, "signature invalid") {
		t.Fatalf("other key: %s %+v", got.Status, got.StatusDetails)
	}
	assertUntouched(t, e.rt)

	e = newEnv(t, "root")
	def := baseDef()
	b, err := e.st.Insert(context.Background(), addons.Installed{
		AddonID: def.ID(), QualifiedName: def.QualifiedName(), Source: def.Source, Version: def.Version,
		Definition: def, Granted: def.Permissions.Canonical(), Status: addons.Installing,
	})
	if err != nil {
		t.Fatal(err)
	}
	e.reconcile()
	if got := e.get(b.ID); got.Status != addons.Failed || !strings.Contains(got.StatusDetails.Message, "signature missing; re-approve") {
		t.Fatalf("unsigned: %s %+v", got.Status, got.StatusDetails)
	}
	assertUntouched(t, e.rt)
}

func TestMisconfiguredLoopDoesNothing(t *testing.T) {
	e := newEnv(t, "root")
	a := e.install(baseDef(), addons.Installing)
	e.r.Instance = ""
	e.reconcile()
	if got := e.get(a.ID); got.Status != addons.Installing {
		t.Fatalf("status = %s", got.Status)
	}
	assertUntouched(t, e.rt)
}

func TestRootEquivalentMountNowNeedsHostRoot(t *testing.T) {
	// Installed under an older policy that only knew host_paths.
	e := newEnv(t, "root")
	def := baseDef()
	def.Services[1].Volumes = []string{"/etc:/host-etc:ro"}
	def.Permissions.HostPaths = []string{"/etc:ro"}
	a := e.install(def, addons.Installing)
	e.reconcile()
	got := e.get(a.ID)
	if got.Status != addons.Failed || !strings.Contains(got.StatusDetails.Message, "current permission policy") ||
		!strings.Contains(got.StatusDetails.Message, "host_root") {
		t.Fatalf("status = %s %q", got.Status, got.StatusDetails.Message)
	}
	assertUntouched(t, e.rt)
}

func TestHostPathOverlapWithAnotherAddonFails(t *testing.T) {
	e := newEnv(t, "root")
	writer := baseDef()
	writer.Name = "writer"
	writer.UI = nil
	writer.Services[1].Volumes = []string{"/srv/shared:/data"}
	writer.Permissions.HostPaths = []string{"/srv/shared"}
	w := e.install(writer, addons.Active)

	reader := baseDef()
	reader.Services[1].Volumes = []string{"/srv/shared/app:/app:ro"}
	reader.Permissions.HostPaths = []string{"/srv/shared/app:ro"}
	a := e.install(reader, addons.Installing)
	e.reconcile()

	got := e.get(a.ID)
	if got.Status != addons.Failed || !strings.Contains(got.StatusDetails.Message, "overlaps") || !strings.Contains(got.StatusDetails.Message, "writer") {
		t.Fatalf("status = %s %+v", got.Status, got.StatusDetails)
	}
	if len(e.rt.Created) != 0 || len(e.rt.Pulled) != 0 {
		t.Errorf("created %v pulled %v", e.rt.Names(), e.rt.Pulled)
	}

	// Once the writer is disabled, nothing is in the way any more.
	e.edit(w.ID, func(x *addons.Installed) { x.Status = addons.Disabled })
	e.edit(a.ID, func(x *addons.Installed) { x.Status = addons.Installing })
	e.reconcile()
	if got := e.get(a.ID); got.Status != addons.Active {
		t.Fatalf("after disabling the writer: %s %+v", got.Status, got.StatusDetails)
	}
}

func TestReservedImageLabelCreatesNothing(t *testing.T) {
	for _, label := range []string{"com.docker.compose.project", "oak.addon.id", "Oakestra.Role"} {
		t.Run(label, func(t *testing.T) {
			e := newEnv(t, "root")
			e.rt.ImageLabelSets[img("worker")] = map[string]string{label: "root_orchestrator", "org.opencontainers.image.title": "w"}
			a := e.install(baseDef(), addons.Installing)
			e.reconcile()
			got := e.get(a.ID)
			if got.Status != addons.Failed || !strings.Contains(got.StatusDetails.Message, "reserved label") {
				t.Fatalf("status = %s %+v", got.Status, got.StatusDetails)
			}
			if len(e.rt.Containers)+len(e.rt.Networks)+len(e.rt.Volumes) != 0 {
				t.Errorf("created %v %v %v", e.rt.Names(), e.rt.Networks, e.rt.Volumes)
			}
		})
	}
}

func TestReservedImageLabelKeepsRunningVersionOnUpdate(t *testing.T) {
	e := newEnv(t, "root")
	a := e.install(baseDef(), addons.Installing)
	e.reconcile()
	oldID := e.rt.Containers["root_addon-insights--web"].ID

	next := baseDef()
	next.Version = "1.1.0"
	next.Services[0].Image = "ghcr.io/x/web@sha256:" + strings.Repeat("cd", 32)
	e.rt.ImageLabelSets[next.Services[0].Image] = map[string]string{"com.docker.compose.service": "system_manager"}
	e.edit(a.ID, func(x *addons.Installed) { x.Definition, x.Version, x.Status = next, "1.1.0", addons.Updating })
	e.reconcile()

	if got := e.get(a.ID); got.Status != addons.Failed {
		t.Fatalf("status = %s", got.Status)
	}
	if c := e.rt.Containers["root_addon-insights--web"]; c == nil || c.ID != oldID {
		t.Error("the running version was replaced")
	}
}

func TestReinstallAdoptsVolumes(t *testing.T) {
	e := newEnv(t, "root")
	a := e.install(baseDef(), addons.Installing)
	e.reconcile()
	vol := e.rt.Volumes["root_addon-insights_data"]
	if _, ok := vol.Labels[runtime.LabelAddonID]; ok || vol.Labels[runtime.LabelAddonKey] != "insights" || vol.Labels[runtime.LabelInstance] != instance {
		t.Fatalf("volume labels = %v", vol.Labels)
	}
	e.edit(a.ID, func(x *addons.Installed) { x.Status = addons.Disabling })
	e.reconcile()

	b := e.install(baseDef(), addons.Installing)
	if b.ID == a.ID {
		t.Fatal("reinstall reused the record")
	}
	e.reconcile()
	if got := e.get(b.ID); got.Status != addons.Active {
		t.Fatalf("reinstall: %s %+v", got.Status, got.StatusDetails)
	}
}

func TestVolumeOfAnotherInstanceIsForeign(t *testing.T) {
	e := newEnv(t, "root")
	e.rt.Volumes["root_addon-insights_data"] = runtime.VolumeSpec{
		Name:   "root_addon-insights_data",
		Labels: map[string]string{runtime.LabelAddonKey: "insights", runtime.LabelPlane: "root", runtime.LabelInstance: "ffffffffffffffffffffffffffffffff"},
	}
	a := e.install(baseDef(), addons.Installing)
	e.reconcile()
	if got := e.get(a.ID); got.Status != addons.Failed || !strings.Contains(got.StatusDetails.Message, "not owned") {
		t.Fatalf("status = %s %+v", got.Status, got.StatusDetails)
	}
}

func TestVolumeWithChangedDriverOptsIsNotReused(t *testing.T) {
	// The old version bound /etc through driver_opts; the new one declares a plain volume of
	// the same name. Reusing the old volume would keep /etc mounted without a grant.
	e := newEnv(t, "root")
	e.rt.Volumes["root_addon-insights_data"] = runtime.VolumeSpec{
		Name:       "root_addon-insights_data",
		DriverOpts: map[string]string{"type": "none", "o": "bind", "device": "/etc"},
		Labels:     own(map[string]string{runtime.LabelAddonKey: "insights"}),
	}
	a := e.install(baseDef(), addons.Installing)
	e.reconcile()
	got := e.get(a.ID)
	if got.Status != addons.Failed || !strings.Contains(got.StatusDetails.Message, "different driver") {
		t.Fatalf("status = %s %+v", got.Status, got.StatusDetails)
	}
	if len(e.rt.Created) != 0 {
		t.Errorf("created %v", e.rt.Names())
	}
}

func TestContainerOfAnotherInstanceIsNotClobbered(t *testing.T) {
	e := newEnv(t, "root")
	a := e.install(baseDef(), addons.Installing)
	e.rt.AddContainer("root_addon-insights--web", map[string]string{
		runtime.LabelAddonID: a.ID, runtime.LabelPlane: "root", runtime.LabelInstance: "ffffffffffffffffffffffffffffffff",
	}, "running")
	e.reconcile()
	if got := e.get(a.ID); got.Status != addons.Failed {
		t.Fatalf("status = %s", got.Status)
	}
	if c := e.rt.Containers["root_addon-insights--web"]; c.Labels[runtime.LabelInstance] == instance {
		t.Fatal("container was replaced")
	}
}

func TestLocalSourceAddonRuns(t *testing.T) {
	e := newEnv(t, "root")
	def := baseDef()
	def.Source = definition.LocalSource
	a := e.install(def, addons.Installing)
	e.reconcile()
	if got := e.get(a.ID); got.Status != addons.Active {
		t.Fatalf("status = %s %+v", got.Status, got.StatusDetails)
	}
	if _, ok := e.rt.Containers["root_addon-local--insights--web"]; !ok {
		t.Errorf("containers = %v", e.rt.Names())
	}
}
