// Package catalog is the marketplace's business logic: it keeps one last good snapshot per
// source, applies the serial rules when a fetch or import brings a new index, manages taps and
// local registrations, and serves the merged catalog.
//
// An index is accepted or rejected as a whole. If a single addon in it is invalid, the
// previous snapshot stays in place and the error is recorded on the source. Dropping just the
// bad entry would be friendlier, but then what the marketplace serves would no longer be what
// the publisher signed off on (or, later, signed): a reviewer looking at serial N couldn't tell
// which subset a given marketplace actually accepted. Whole-index acceptance keeps "serial N"
// meaning one exact set of definitions everywhere.
package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/oakestra/oakestra/addons/internal/definition"
	"github.com/oakestra/oakestra/addons/internal/marketplace/registry"
	"github.com/oakestra/oakestra/addons/internal/marketplace/source"
	"github.com/oakestra/oakestra/addons/internal/marketplace/store"
)

var (
	// ErrNotFound means the source or addon doesn't exist.
	ErrNotFound = errors.New("not found")
	// ErrSyncDisabled means the source has no URL (the official source set to "off").
	ErrSyncDisabled = errors.New("this source has no URL, it can only be fed by import")
)

// ConflictError is a request that contradicts the current state (duplicate tap, deleting a
// built-in source, a local version that isn't newer).
type ConflictError struct{ Msg string }

func (e *ConflictError) Error() string { return e.Msg }

// ValidationError is invalid input. Fields is set for definition errors.
type ValidationError struct {
	Msg    string
	Fields definition.Errors
}

func (e *ValidationError) Error() string { return e.Msg }

// RejectedError means an index was fetched or uploaded but not accepted. The previous
// snapshot is unchanged and the reason is recorded on the source.
type RejectedError struct{ Msg string }

func (e *RejectedError) Error() string { return e.Msg }

// FetchError means the source couldn't be fetched (network, status, size, decoding). The
// previous snapshot is unchanged and the reason is recorded on the source.
type FetchError struct{ Err error }

func (e *FetchError) Error() string { return e.Err.Error() }
func (e *FetchError) Unwrap() error { return e.Err }

// ResolveError means an image tag of a local registration couldn't be resolved to a digest.
type ResolveError struct{ Err error }

func (e *ResolveError) Error() string { return e.Err.Error() }
func (e *ResolveError) Unwrap() error { return e.Err }

// Config wires a Service.
type Config struct {
	Store    store.Store
	Fetcher  *source.Fetcher
	Resolver registry.Resolver
	// OfficialURL is empty when the official source is off.
	OfficialURL string
	// OfficialToken is kept in memory only; it is never written to the store.
	OfficialToken string
	Logger        *slog.Logger
	// Now defaults to time.Now.
	Now func() time.Time
}

// Service implements the catalog operations. It is safe for concurrent use.
type Service struct {
	cfg Config
	log *slog.Logger

	mu    sync.Mutex
	locks map[string]*sync.Mutex

	cacheMu sync.Mutex
	derived map[string]derivedEntry
}

// New returns a Service. Call Init before using it.
func New(cfg Config) *Service {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{cfg: cfg, log: cfg.Logger, locks: map[string]*sync.Mutex{}, derived: map[string]derivedEntry{}}
}

// lock serializes syncs, imports, registrations and deletion per source, so two writers never
// race on the same snapshot and a sync can't resurrect a tap that was deleted mid-fetch.
func (s *Service) lock(name string) func() {
	s.mu.Lock()
	l, ok := s.locks[name]
	if !ok {
		l = &sync.Mutex{}
		s.locks[name] = l
	}
	s.mu.Unlock()
	l.Lock()
	return l.Unlock
}

// Init makes sure the official and local sources exist and brings the official source in line
// with the environment.
func (s *Service) Init(ctx context.Context) error {
	now := s.cfg.Now().UTC()
	st := s.cfg.Store

	official, err := st.GetSource(ctx, string(definition.SourceOfficial))
	switch {
	case errors.Is(err, store.ErrNotFound):
		err = st.CreateSource(ctx, &store.Source{
			Name: string(definition.SourceOfficial), Kind: definition.SourceOfficial,
			URL: s.cfg.OfficialURL, CreatedAt: now,
		})
		if err != nil && !errors.Is(err, store.ErrExists) {
			return err
		}
	case err != nil:
		return err
	case official.URL != s.cfg.OfficialURL:
		if err := s.officialURLChanged(ctx, official.URL); err != nil {
			return err
		}
	}

	err = st.CreateSource(ctx, &store.Source{
		Name: string(definition.SourceLocal), Kind: definition.SourceLocal, CreatedAt: now,
	})
	if err != nil && !errors.Is(err, store.ErrExists) {
		return err
	}
	return nil
}

// officialURLChanged handles an operator pointing the official source somewhere else. Serials
// are only comparable within one publisher, so moving from a development index to the real
// one (or back online after an import) would otherwise stay stuck on the old serial forever.
// The environment is operator-controlled, so this is no weaker than the rollback check itself.
// Turning the source off keeps the floor, so an air-gapped import still can't go backwards.
func (s *Service) officialURLChanged(ctx context.Context, oldURL string) error {
	name := string(definition.SourceOfficial)
	unlock := s.lock(name)
	defer unlock()

	err := s.cfg.Store.UpdateSourceConfig(ctx, &store.Source{
		Name: name, Kind: definition.SourceOfficial, URL: s.cfg.OfficialURL,
	})
	if err != nil {
		return err
	}
	if s.cfg.OfficialURL == "" {
		s.log.Info("official source turned off, keeping the last good snapshot")
		return nil
	}
	snap, err := s.cfg.Store.GetSnapshot(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	s.log.Warn("official source URL changed, the next index is accepted regardless of its serial",
		"previous_serial", snap.Serial, "had_url", oldURL != "")
	snap.Serial = 0
	snap.ETag = ""
	return s.cfg.Store.PutSnapshot(ctx, snap)
}

// target is what a sync needs to know about a source. Init keeps the official source's URL in
// line with the environment, so src.URL is current for every kind.
type target struct {
	src   *store.Source
	url   string
	token string
	// The official URL comes from the environment, which the operator controls, so it may
	// use http, a private address (a LAN mirror) or a file. Taps get only what the admin
	// allowed explicitly.
	allowInsecure, allowPrivate, allowFile bool
}

func (s *Service) target(ctx context.Context, name string) (*target, error) {
	src, err := s.cfg.Store.GetSource(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	switch src.Kind {
	case definition.SourceOfficial:
		return &target{
			src: src, url: src.URL, token: s.cfg.OfficialToken,
			allowInsecure: true, allowPrivate: true, allowFile: true,
		}, nil
	case definition.SourceTap:
		return &target{
			src: src, url: src.URL, token: src.Token,
			allowInsecure: src.AllowInsecure, allowPrivate: src.AllowPrivate,
		}, nil
	default:
		return nil, &ConflictError{Msg: fmt.Sprintf("source %q is not fed by an index", name)}
	}
}

// Sync fetches the source's index and applies it. Fetch and rejection errors are recorded on
// the source and returned.
func (s *Service) Sync(ctx context.Context, name string) error {
	unlock := s.lock(name)
	defer unlock()

	t, err := s.target(ctx, name)
	if err != nil {
		return err
	}
	if t.url == "" {
		return ErrSyncDisabled
	}
	snap, err := s.snapshot(ctx, name)
	if err != nil {
		return err
	}
	req := source.Request{
		URL: t.url, Token: t.token,
		AllowInsecure: t.allowInsecure, AllowPrivate: t.allowPrivate, AllowFile: t.allowFile,
	}
	if snap != nil {
		req.ETag = snap.ETag
	}
	res, err := s.cfg.Fetcher.Fetch(ctx, req)
	if err != nil {
		return s.fail(ctx, t.src, &FetchError{Err: err})
	}
	if res.NotModified {
		if snap == nil {
			return s.fail(ctx, t.src, &FetchError{Err: errors.New("server answered 304 Not Modified without a cached index")})
		}
		return s.succeed(ctx, t.src)
	}
	return s.apply(ctx, t.src, snap, res.Index, res.ETag)
}

// Import applies an uploaded index, for air-gapped setups. The rules are the same as for a
// sync; the source may have no URL at all.
func (s *Service) Import(ctx context.Context, name string, r io.Reader) error {
	unlock := s.lock(name)
	defer unlock()

	t, err := s.target(ctx, name)
	if err != nil {
		return err
	}
	snap, err := s.snapshot(ctx, name)
	if err != nil {
		return err
	}
	idx, err := source.Decode(r)
	if err != nil {
		return s.fail(ctx, t.src, &RejectedError{Msg: err.Error()})
	}
	// An import has no HTTP validator. Clearing the ETag makes the next online sync fetch the
	// full index instead of trusting a 304 that refers to an older download.
	return s.apply(ctx, t.src, snap, idx, "")
}

func (s *Service) apply(ctx context.Context, src *store.Source, snap *store.Snapshot, idx *definition.Index, etag string) error {
	if idx.Source != src.Name {
		return s.reject(ctx, src, "index source %q doesn't match the source name %q", idx.Source, src.Name)
	}
	if errs := definition.ValidateIndex(idx); len(errs) > 0 {
		return s.reject(ctx, src, "index rejected: %s", summarize(errs))
	}
	hash := contentHash(idx.Addons)
	if snap != nil && snap.Serial > 0 {
		switch {
		case idx.Serial < snap.Serial:
			return s.reject(ctx, src, "index serial %d is lower than the last accepted serial %d", idx.Serial, snap.Serial)
		case idx.Serial == snap.Serial && hash != snap.ContentHash:
			return s.reject(ctx, src, "index serial %d was already accepted with different content; the publisher must increase the serial", idx.Serial)
		case idx.Serial == snap.Serial:
			if etag != snap.ETag {
				snap.ETag = etag
				if err := s.cfg.Store.PutSnapshot(ctx, snap); err != nil {
					return err
				}
			}
			return s.succeed(ctx, src)
		}
	}
	err := s.cfg.Store.PutSnapshot(ctx, &store.Snapshot{
		Source:      src.Name,
		Serial:      idx.Serial,
		ContentHash: hash,
		ETag:        etag,
		GeneratedAt: idx.GeneratedAt,
		UpdatedAt:   s.cfg.Now().UTC(),
		Addons:      idx.Addons,
	})
	if err != nil {
		return err
	}
	s.log.Info("source updated", "source", src.Name, "serial", idx.Serial, "addons", len(idx.Addons))
	return s.succeed(ctx, src)
}

func (s *Service) reject(ctx context.Context, src *store.Source, format string, args ...any) error {
	return s.fail(ctx, src, &RejectedError{Msg: fmt.Sprintf(format, args...)})
}

// maxErrorLength keeps a pathological index from filling the source document with errors.
const maxErrorLength = 2000

func (s *Service) fail(ctx context.Context, src *store.Source, cause error) error {
	msg := s.scrubURL(src, cause.Error())
	if len(msg) > maxErrorLength {
		msg = msg[:maxErrorLength] + "..."
	}
	st := src.Status
	st.LastAttemptAt = s.cfg.Now().UTC()
	st.LastError = msg
	if err := s.cfg.Store.SetStatus(ctx, src.Name, st); err != nil {
		s.log.Error("recording sync failure", "source", src.Name, "error", err)
	}
	s.log.Warn("source not updated, keeping the last good snapshot", "source", src.Name, "error", msg)
	return cause
}

// scrubURL is a backstop for error text: the fetcher already avoids echoing the URL, but this
// list is public and a future error path that does must not leak a query-string secret.
func (s *Service) scrubURL(src *store.Source, msg string) string {
	raw := src.URL
	u, err := url.Parse(raw)
	if raw == "" || err != nil || u.RawQuery == "" {
		return msg
	}
	msg = strings.ReplaceAll(msg, raw, source.RedactURL(raw))
	return strings.ReplaceAll(msg, u.RawQuery, "<redacted>")
}

func (s *Service) succeed(ctx context.Context, src *store.Source) error {
	now := s.cfg.Now().UTC()
	return s.cfg.Store.SetStatus(ctx, src.Name, store.Status{LastAttemptAt: now, LastSyncedAt: now})
}

func (s *Service) snapshot(ctx context.Context, name string) (*store.Snapshot, error) {
	snap, err := s.cfg.Store.GetSnapshot(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	return snap, err
}

// SyncAll syncs the official source (when it has a URL) and every tap, one after the other.
// Failures are recorded per source and logged; they don't stop the others.
func (s *Service) SyncAll(ctx context.Context) {
	sources, err := s.cfg.Store.ListSources(ctx)
	if err != nil {
		s.log.Error("listing sources for sync", "error", err)
		return
	}
	for _, src := range sources {
		if ctx.Err() != nil {
			return
		}
		// The local source has no URL, and neither has the official one when it is turned off.
		if src.URL == "" {
			continue
		}
		// Errors are already recorded and logged by Sync.
		_ = s.Sync(ctx, src.Name)
	}
}

func contentHash(addons []definition.Addon) string {
	b, err := json.Marshal(addons)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func summarize(errs definition.Errors) string {
	const show = 5
	parts := make([]string, 0, show+1)
	for i, e := range errs {
		if i == show {
			parts = append(parts, fmt.Sprintf("and %d more", len(errs)-show))
			break
		}
		parts = append(parts, e.Error())
	}
	return strings.Join(parts, "; ")
}

// sourceOrder lists official first, then taps by name, then local.
func sourceOrder(kind definition.SourceKind) int {
	switch kind {
	case definition.SourceOfficial:
		return 0
	case definition.SourceTap:
		return 1
	default:
		return 2
	}
}

func sortSources(sources []store.Source) {
	slices.SortStableFunc(sources, func(a, b store.Source) int {
		if d := sourceOrder(a.Kind) - sourceOrder(b.Kind); d != 0 {
			return d
		}
		return strings.Compare(a.Name, b.Name)
	})
}

// newerVersion reports whether next is a higher semantic version than prev.
func newerVersion(next, prev string) (bool, error) {
	n, err := semver.StrictNewVersion(next)
	if err != nil {
		return false, err
	}
	p, err := semver.StrictNewVersion(prev)
	if err != nil {
		// A stored version that doesn't parse can't be compared; let the new one win.
		return true, nil
	}
	return n.GreaterThan(p), nil
}
