package catalog

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/oakestra/oakestra/addons/internal/definition"
	"github.com/oakestra/oakestra/addons/internal/marketplace/store"
)

// Item is one addon as the API serves it: the definition plus the derived fields consumers
// shouldn't have to recompute. cmd/addons reads exactly this shape from
// GET /api/v1/marketplace/addons/{source}/{name}.
type Item struct {
	definition.Addon
	// QualifiedName is the user-facing name ("name", "<tap>/name", "local/name").
	QualifiedName string `json:"qualified_name"`
	// ID is the DNS-safe id ("name", "<tap>--name", "local--name").
	ID string `json:"id"`
	// RequiredPermissions is what the definition actually uses, derived from its services.
	RequiredPermissions definition.Permissions `json:"required_permissions"`
	// PermissionsHash is the hash of the declared permissions, which the install API makes
	// the admin echo back.
	PermissionsHash string `json:"permissions_hash"`
}

func newItem(a definition.Addon) Item {
	a.Normalize()
	return Item{
		Addon:               a,
		QualifiedName:       a.QualifiedName(),
		ID:                  a.ID(),
		RequiredPermissions: definition.RequiredPermissions(&a),
		PermissionsHash:     a.Permissions.Hash(),
	}
}

// Filter narrows the merged list. Empty fields match everything.
type Filter struct {
	// Source is a source name: "official", "local" or a tap name.
	Source string
	// Name matches the bare name or the qualified name.
	Name string
}

// derivedKey identifies the content a cached item list was computed from. A snapshot with the
// same serial and content hash has the same addons, so the cache needs no explicit
// invalidation on writes.
type derivedKey struct {
	serial uint64
	hash   string
}

type derivedEntry struct {
	key   derivedKey
	items []Item
}

// derivedItems returns the sorted items of the snapshot that head describes, reading and
// decoding the snapshot only when the cache doesn't hold that serial and content hash. The read
// endpoints are unauthenticated, so decoding up to MaxIndexSize of JSON per request would be a
// cheap way to burn CPU and memory. Callers must treat the result as read-only.
func (s *Service) derivedItems(ctx context.Context, head *store.Snapshot) ([]Item, error) {
	s.cacheMu.Lock()
	e, ok := s.derived[head.Source]
	s.cacheMu.Unlock()
	if ok && e.key == (derivedKey{head.Serial, head.ContentHash}) {
		return e.items, nil
	}
	snap, err := s.snapshot(ctx, head.Source)
	if err != nil || snap == nil {
		return nil, err
	}
	// The snapshot may be newer than head; cache it under its own key.
	key := derivedKey{snap.Serial, snap.ContentHash}
	addons := slices.Clone(snap.Addons)
	slices.SortFunc(addons, func(a, b definition.Addon) int { return strings.Compare(a.Name, b.Name) })
	items := make([]Item, 0, len(addons))
	for _, a := range addons {
		items = append(items, newItem(a))
	}
	s.cacheMu.Lock()
	// A concurrent request may have stored the same key already; the result is identical.
	s.derived[snap.Source] = derivedEntry{key: key, items: items}
	s.cacheMu.Unlock()
	return items, nil
}

// Addons returns the merged catalog: official, taps by name, local; addons by name within a
// source. Snapshots whose source no longer exists are skipped.
func (s *Service) Addons(ctx context.Context, f Filter) ([]Item, error) {
	sources, err := s.cfg.Store.ListSources(ctx)
	if err != nil {
		return nil, err
	}
	snaps, err := s.cfg.Store.ListSnapshotHeaders(ctx)
	if err != nil {
		return nil, err
	}
	bySource := map[string]*store.Snapshot{}
	for i := range snaps {
		bySource[snaps[i].Source] = &snaps[i]
	}
	sortSources(sources)

	out := []Item{}
	for _, src := range sources {
		if f.Source != "" && src.Name != f.Source {
			continue
		}
		snap := bySource[src.Name]
		if snap == nil {
			continue
		}
		items, err := s.derivedItems(ctx, snap)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if f.Name != "" && item.Name != f.Name && item.QualifiedName != f.Name {
				continue
			}
			out = append(out, item)
		}
	}
	return out, nil
}

// Addon returns one addon of a source.
func (s *Service) Addon(ctx context.Context, sourceName, name string) (*Item, error) {
	if _, err := s.cfg.Store.GetSource(ctx, sourceName); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	items, err := s.Addons(ctx, Filter{Source: sourceName, Name: name})
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		// Filter.Name also matches qualified names, which a bare name lookup must not.
		if item.Name == name {
			return &item, nil
		}
	}
	return nil, ErrNotFound
}

// RegisterLocal validates a definition, pins its images and stores it in the local source.
// Re-registering a name replaces the old definition, but only with a higher version, so
// installed copies can tell an update from a rollback.
func (s *Service) RegisterLocal(ctx context.Context, a *definition.Addon) (*Item, error) {
	a.Source = definition.LocalSource
	a.Normalize()
	if err := definition.Check(a, definition.ModeLocal); err != nil {
		var fields definition.Errors
		errors.As(err, &fields)
		return nil, &ValidationError{Msg: "invalid addon definition: " + err.Error(), Fields: fields}
	}

	// Resolve before taking the lock: it talks to registries and may take a while.
	for i := range a.Services {
		svc := &a.Services[i]
		ref, err := definition.ParseImage(svc.Image)
		if err != nil {
			// Check already validated the reference.
			return nil, &ValidationError{Msg: err.Error()}
		}
		if ref.Pinned() {
			continue
		}
		digest, err := s.cfg.Resolver.Resolve(ctx, svc.Image)
		if err != nil {
			return nil, &ResolveError{Err: fmt.Errorf("services[%d].image: %w", i, err)}
		}
		if _, err := definition.ParseImage(ref.Name + "@" + digest); err != nil {
			return nil, &ResolveError{Err: fmt.Errorf("services[%d].image: registry returned an invalid digest %q", i, digest)}
		}
		ref.Digest = digest
		svc.Image = ref.String()
	}

	name := string(definition.SourceLocal)
	unlock := s.lock(name)
	defer unlock()

	src, err := s.cfg.Store.GetSource(ctx, name)
	if err != nil {
		return nil, err
	}
	snap, err := s.snapshot(ctx, name)
	if err != nil {
		return nil, err
	}
	if snap == nil {
		snap = &store.Snapshot{Source: name}
	}
	addons := slices.Clone(snap.Addons)
	pos := slices.IndexFunc(addons, func(o definition.Addon) bool { return o.Name == a.Name })
	if pos >= 0 {
		newer, err := newerVersion(a.Version, addons[pos].Version)
		if err != nil {
			return nil, &ValidationError{Msg: err.Error()}
		}
		if !newer {
			return nil, &ConflictError{Msg: fmt.Sprintf("local/%s is already registered with version %s; a new registration needs a higher version", a.Name, addons[pos].Version)}
		}
		addons[pos] = *a
	} else {
		addons = append(addons, *a)
	}
	if err := s.putLocal(ctx, src, snap, addons); err != nil {
		return nil, err
	}
	s.log.Info("local addon registered", "name", a.Name, "version", a.Version)
	item := newItem(*a)
	return &item, nil
}

// DeleteLocal removes a local registration.
func (s *Service) DeleteLocal(ctx context.Context, addonName string) error {
	name := string(definition.SourceLocal)
	unlock := s.lock(name)
	defer unlock()

	src, err := s.cfg.Store.GetSource(ctx, name)
	if err != nil {
		return err
	}
	snap, err := s.snapshot(ctx, name)
	if err != nil {
		return err
	}
	if snap == nil {
		return ErrNotFound
	}
	pos := slices.IndexFunc(snap.Addons, func(o definition.Addon) bool { return o.Name == addonName })
	if pos < 0 {
		return ErrNotFound
	}
	addons := slices.Delete(slices.Clone(snap.Addons), pos, pos+1)
	if err := s.putLocal(ctx, src, snap, addons); err != nil {
		return err
	}
	s.log.Info("local addon deleted", "name", addonName)
	return nil
}

// putLocal stores the local catalog. Its serial counts changes, which gives consumers the same
// "has anything changed" signal as for indexes.
func (s *Service) putLocal(ctx context.Context, src *store.Source, prev *store.Snapshot, addons []definition.Addon) error {
	slices.SortFunc(addons, func(a, b definition.Addon) int { return strings.Compare(a.Name, b.Name) })
	now := s.cfg.Now().UTC()
	err := s.cfg.Store.PutSnapshot(ctx, &store.Snapshot{
		Source:      src.Name,
		Serial:      prev.Serial + 1,
		ContentHash: contentHash(addons),
		GeneratedAt: now,
		UpdatedAt:   now,
		Addons:      addons,
	})
	if err != nil {
		return err
	}
	return s.succeed(ctx, src)
}
