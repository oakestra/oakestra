// Package reconcile drives containers towards the installed addons recorded in the store.
//
// State machine (the API only writes the states marked with *):
//
//	installing*, updating*  -> active | partially_active | failed
//	active, partially_active -> partially_active | failed | active (watched every interval)
//	disabling*              -> disabled
//	failed, disabled        -> terminal until the user updates or deletes the addon
//
// Every container and network the loop creates carries the oak.addon.id label with the
// record's id, plus the plane and the engine's instance id. The loop only ever removes objects
// that carry all three, which is what stops an addon from replacing a core service by choosing
// its name, and two engines sharing a docker daemon from removing each other's containers.
// Volumes are owned by the addon id instead of the record id, see teardown.
//
// Nothing in a record is trusted before verify has checked its grant signature (package
// grant): the database is the one place an attacker on the core network could write to.
package reconcile

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/oakestra/oakestra/addons/internal/addons"
	"github.com/oakestra/oakestra/addons/internal/addons/grant"
	"github.com/oakestra/oakestra/addons/internal/addons/runtime"
	"github.com/oakestra/oakestra/addons/internal/addons/store"
	"github.com/oakestra/oakestra/addons/internal/definition"
)

const (
	// DefaultInterval is how often the loop runs.
	DefaultInterval = 15 * time.Second
	// DefaultMaxRestarts is how often docker may restart a container before the addon counts
	// it as failed. A restarting container is otherwise indistinguishable from a healthy one.
	DefaultMaxRestarts = 5

	logTail     = 100
	pullTimeout = 10 * time.Minute
)

// Reconciler is the reconcile loop.
type Reconciler struct {
	Store   store.Store
	Runtime runtime.Runtime
	// Plane is "root" or "cluster". It prefixes object names and labels containers, so the
	// root and cluster engines on one host don't touch each other's addons.
	Plane string
	// Instance is this engine's instance id. It labels everything the loop creates and scopes
	// every listing, so a second deployment on the same daemon is invisible to this one.
	Instance string
	// GrantKey verifies the signature the API put on each record.
	GrantKey    grant.Key
	Interval    time.Duration
	MaxRestarts int
	Logger      *slog.Logger
}

func (r *Reconciler) log() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}

// Run reconciles once immediately and then every Interval until ctx is done.
func (r *Reconciler) Run(ctx context.Context) {
	interval := r.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	r.ReconcileOnce(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.ReconcileOnce(ctx)
		}
	}
}

// ReconcileOnce handles every addon once. It is sequential: image pulls can make it slow, but
// it keeps container and network changes free of races between addons.
func (r *Reconciler) ReconcileOnce(ctx context.Context) {
	if r.Plane == "" || r.Instance == "" || r.GrantKey.IsZero() {
		// Without these the label filters would match other engines' objects and every
		// record would fail verification, so do nothing rather than damage.
		r.log().Error("reconcile loop is misconfigured: plane, instance id and grant key are required")
		return
	}
	list, err := r.Store.List(ctx, "")
	if err != nil {
		// Without the list, "no record" can't be told apart from "can't read records", so
		// skip the orphan cleanup too.
		r.log().Error("listing addons failed", "error", err)
		return
	}
	for _, a := range list {
		if ctx.Err() != nil {
			return
		}
		switch a.Status {
		case addons.Installing, addons.Updating:
			r.deploy(ctx, a, list)
		case addons.Disabling:
			r.teardown(ctx, a)
		case addons.Active, addons.PartiallyActive:
			r.watch(ctx, a)
		}
	}
	r.removeOrphans(ctx, list)
}

// scope are the labels that mark an object as this engine's.
func (r *Reconciler) scope() map[string]string {
	return map[string]string{runtime.LabelPlane: r.Plane, runtime.LabelInstance: r.Instance}
}

// recordLabels are scope plus the record id.
func (r *Reconciler) recordLabels(id string) map[string]string {
	l := r.scope()
	l[runtime.LabelAddonID] = id
	return l
}

// owned lists this engine's containers that carry the addon's record id.
func (r *Reconciler) owned(ctx context.Context, id string) ([]runtime.Container, error) {
	return r.Runtime.ListContainers(ctx, r.recordLabels(id))
}

func (r *Reconciler) isOwned(c *runtime.Container, id string) bool {
	for k, v := range r.recordLabels(id) {
		if c.Labels[k] != v {
			return false
		}
	}
	return true
}

// settle moves an addon to a final state, but only if it is still in one of the states the
// reconcile started from. If the API changed it in the meantime (e.g. a DELETE during an
// install), the next pass handles the new state.
func (r *Reconciler) settle(ctx context.Context, id string, from []addons.Status, apply func(*addons.Installed)) {
	_, err := r.Store.Update(ctx, id, func(a *addons.Installed) error {
		if !slices.Contains(from, a.Status) {
			return store.ErrAbort
		}
		apply(a)
		a.UpdatedAt = time.Now().UTC()
		return nil
	})
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		r.log().Error("storing addon status failed", "addon", id, "error", err)
	}
}

func (r *Reconciler) fail(ctx context.Context, a addons.Installed, msg string, failed []string, logs string) {
	r.log().Warn("addon failed", "addon", a.AddonID, "reason", msg)
	r.settle(ctx, a.ID, []addons.Status{a.Status}, func(x *addons.Installed) {
		x.Status = addons.Failed
		x.StatusDetails = addons.StatusDetails{Message: msg, FailedServices: failed}
		x.Logs = logs
	})
}

func (r *Reconciler) deploy(ctx context.Context, a addons.Installed, all []addons.Installed) {
	def, err := r.verify(a)
	if err != nil {
		r.fail(ctx, a, err.Error(), nil, "")
		return
	}
	p, err := r.buildPlan(a, &def)
	if err != nil {
		r.fail(ctx, a, err.Error(), nil, "")
		return
	}
	// The API checked this at install time, but another addon may have been installed or
	// updated since, and the records come from a database.
	if msg, conflict := addons.HostBindConflict(&def, a.ID, all); conflict {
		r.fail(ctx, a, msg, nil, "")
		return
	}

	// Look for name clashes before touching anything, so a clash doesn't cost the running
	// version of an addon that is being updated.
	for _, sp := range p.services {
		c, err := r.Runtime.InspectContainer(ctx, sp.spec.Name)
		if err != nil {
			r.log().Error("inspecting container failed, will retry", "container", sp.spec.Name, "error", err)
			return
		}
		if c != nil && !r.isOwned(c, a.ID) {
			r.fail(ctx, a, fmt.Sprintf("container name %q is already used by a container that does not belong to this addon", sp.spec.Name), []string{sp.name}, "")
			return
		}
	}

	// Pull everything before changing anything, so an image that turns out to carry reserved
	// labels fails the addon with nothing created and an update keeps the running version.
	// A pull failure only fails that service.
	pullErrs := map[string]error{}
	for _, sp := range p.services {
		if err := r.pull(ctx, sp.spec.Image); err != nil {
			pullErrs[sp.name] = err
			continue
		}
		labels, err := r.Runtime.ImageLabels(ctx, sp.spec.Image)
		if err != nil {
			pullErrs[sp.name] = fmt.Errorf("inspecting %s: %w", sp.spec.Image, err)
			continue
		}
		// Image labels land on the container without passing the definition check: a
		// com.docker.compose.project label would make the addon part of the core compose
		// project, and log shippers trust such labels too.
		for _, k := range slices.Sorted(maps.Keys(labels)) {
			if definition.IsReservedLabel(k) {
				r.fail(ctx, a, fmt.Sprintf("service %s: image %s carries the reserved label %q; images may not set oak.*, oakestra.*, org.oakestra.* or com.docker.compose.* labels", sp.name, sp.spec.Image, k), []string{sp.name}, "")
				return
			}
		}
	}

	// Replace rather than reuse: after an update the old containers run the old snapshot, and
	// after a crashed install they are in an unknown state.
	old, err := r.owned(ctx, a.ID)
	if err != nil {
		r.log().Error("listing containers failed, will retry", "addon", a.AddonID, "error", err)
		return
	}
	for _, c := range old {
		if err := r.Runtime.RemoveContainer(ctx, c.ID); err != nil {
			r.log().Error("removing old container failed, will retry", "container", c.Name, "error", err)
			return
		}
	}

	for _, n := range p.networks {
		if err := r.Runtime.EnsureNetwork(ctx, n); err != nil {
			r.fail(ctx, a, fmt.Sprintf("network %s: %v", n.Name, err), nil, "")
			return
		}
	}
	for _, v := range p.volumes {
		if err := r.Runtime.EnsureVolume(ctx, v); err != nil {
			r.fail(ctx, a, fmt.Sprintf("volume %s: %v", v.Name, err), nil, "")
			return
		}
	}

	var failed []string
	var problems []string
	for _, sp := range p.services {
		err := pullErrs[sp.name]
		if err == nil {
			err = r.Runtime.CreateAndStart(ctx, sp.spec)
		}
		if err != nil {
			r.log().Warn("starting service failed", "addon", a.AddonID, "service", sp.name, "error", err)
			failed = append(failed, sp.name)
			problems = append(problems, fmt.Sprintf("%s: %v", sp.name, err))
		}
	}

	status := addons.Active
	switch {
	case len(failed) == len(p.services):
		status = addons.Failed
	case len(failed) > 0:
		status = addons.PartiallyActive
	}
	r.settle(ctx, a.ID, []addons.Status{a.Status}, func(x *addons.Installed) {
		x.Status = status
		x.StatusDetails = addons.StatusDetails{FailedServices: failed, Message: strings.Join(problems, "; ")}
		x.Logs = ""
	})
}

func (r *Reconciler) pull(ctx context.Context, ref string) error {
	pullCtx, cancel := context.WithTimeout(ctx, pullTimeout)
	defer cancel()
	if err := r.Runtime.PullImage(pullCtx, ref); err != nil {
		return fmt.Errorf("pulling %s: %w", ref, err)
	}
	return nil
}

// teardown removes the addon's containers and networks. Volumes stay: they are labeled with the
// addon id, plane and instance rather than the record id, so a later install of the same addon
// on this engine adopts them and finds its data again. Deleting them is a deliberate manual
// step.
func (r *Reconciler) teardown(ctx context.Context, a addons.Installed) {
	cs, err := r.owned(ctx, a.ID)
	if err != nil {
		r.log().Error("listing containers failed, will retry", "addon", a.AddonID, "error", err)
		return
	}
	for _, c := range cs {
		if err := r.Runtime.RemoveContainer(ctx, c.ID); err != nil {
			r.log().Error("removing container failed, will retry", "container", c.Name, "error", err)
			return
		}
	}
	nets, err := r.Runtime.ListNetworks(ctx, r.recordLabels(a.ID))
	if err != nil {
		r.log().Warn("listing networks failed", "addon", a.AddonID, "error", err)
	}
	for _, n := range nets {
		// A network that is still in use by someone else is left behind, which is harmless.
		if err := r.Runtime.RemoveNetwork(ctx, n); err != nil {
			r.log().Warn("removing network failed", "network", n, "error", err)
		}
	}
	r.settle(ctx, a.ID, []addons.Status{addons.Disabling}, func(x *addons.Installed) {
		x.Status = addons.Disabled
		x.Current = false
		x.StatusDetails = addons.StatusDetails{}
		x.Logs = ""
	})
}

// watch compares the containers with the snapshot and reports services that are gone or not
// running.
func (r *Reconciler) watch(ctx context.Context, a addons.Installed) {
	cs, err := r.owned(ctx, a.ID)
	if err != nil {
		r.log().Error("listing containers failed", "addon", a.AddonID, "error", err)
		return
	}
	byName := map[string]runtime.Container{}
	for _, c := range cs {
		byName[c.Name] = c
	}
	maxRestarts := r.MaxRestarts
	if maxRestarts <= 0 {
		maxRestarts = DefaultMaxRestarts
	}

	var failed []string
	var logs []string
	for _, s := range a.Definition.Services {
		c, ok := byName[r.containerName(a.AddonID, s.ServiceName)]
		switch {
		case !ok:
			failed = append(failed, s.ServiceName)
		case c.State == "running" || c.State == "paused":
		case c.State == "restarting" && c.RestartCount < maxRestarts:
		default:
			failed = append(failed, s.ServiceName)
			if out, err := r.Runtime.ContainerLogs(ctx, c.ID, logTail); err == nil {
				logs = append(logs, fmt.Sprintf("Logs for service '%s' (%s, exit code %d):\n%s", s.ServiceName, c.State, c.ExitCode, out))
			}
		}
	}

	want := addons.Active
	switch {
	case len(failed) > 0 && len(failed) == len(a.Definition.Services):
		want = addons.Failed
	case len(failed) > 0:
		want = addons.PartiallyActive
	}
	// Logs change on every pass, so only a change in status or in the failing set is written.
	if want == a.Status && slices.Equal(failed, a.StatusDetails.FailedServices) {
		return
	}
	r.log().Warn("addon state changed", "addon", a.AddonID, "from", a.Status, "to", want, "failed_services", failed)
	r.settle(ctx, a.ID, []addons.Status{addons.Active, addons.PartiallyActive}, func(x *addons.Installed) {
		x.Status = want
		x.StatusDetails = addons.StatusDetails{FailedServices: failed}
		x.Logs = strings.Join(logs, "\n----------------\n")
	})
}

// removeOrphans removes this engine's containers that carry an addon id no live record claims,
// e.g. after a record was deleted by hand. Containers without our plane and instance labels are
// never considered.
func (r *Reconciler) removeOrphans(ctx context.Context, list []addons.Installed) {
	live := map[string]bool{}
	for _, a := range list {
		if a.Status != addons.Disabled {
			live[a.ID] = true
		}
	}
	cs, err := r.Runtime.ListContainers(ctx, r.scope())
	if err != nil {
		r.log().Warn("listing containers for orphan cleanup failed", "error", err)
		return
	}
	for _, c := range cs {
		id, ok := c.Labels[runtime.LabelAddonID]
		if !ok || live[id] {
			continue
		}
		r.log().Info("removing orphaned addon container", "container", c.Name)
		if err := r.Runtime.RemoveContainer(ctx, c.ID); err != nil {
			r.log().Warn("removing orphaned container failed", "container", c.Name, "error", err)
		}
	}
}
