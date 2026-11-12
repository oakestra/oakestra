package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oakestra/oakestra/addons/internal/definition"
	"github.com/oakestra/oakestra/addons/internal/marketplace/source"
	"github.com/oakestra/oakestra/addons/internal/marketplace/store"
)

// SourceView is a source as the API shows it. It deliberately has no token field, only
// HasToken, so no code path can serialize a credential by accident. URL has its query string
// redacted because the sources list is unauthenticated and presigned or ?token= URLs carry
// secrets there.
type SourceView struct {
	Name          string                `json:"name"`
	Kind          definition.SourceKind `json:"kind"`
	URL           string                `json:"url"`
	HasToken      bool                  `json:"has_token"`
	AllowInsecure bool                  `json:"allow_insecure"`
	AllowPrivate  bool                  `json:"allow_private"`
	// SyncEnabled is false for the local source and for the official source when it is off.
	SyncEnabled   bool       `json:"sync_enabled"`
	Deletable     bool       `json:"deletable"`
	CreatedAt     time.Time  `json:"created_at"`
	LastAttemptAt *time.Time `json:"last_attempt_at"`
	LastSyncedAt  *time.Time `json:"last_synced_at"`
	LastError     string     `json:"last_error"`
	Serial        uint64     `json:"serial"`
	ETag          string     `json:"etag"`
	GeneratedAt   *time.Time `json:"generated_at"`
	AddonCount    int        `json:"addon_count"`
}

func optTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	t = t.UTC()
	return &t
}

func (s *Service) view(src store.Source, snap *store.Snapshot) SourceView {
	v := SourceView{
		Name:          src.Name,
		Kind:          src.Kind,
		URL:           source.RedactURL(src.URL),
		HasToken:      src.Token != "",
		AllowInsecure: src.AllowInsecure,
		AllowPrivate:  src.AllowPrivate,
		SyncEnabled:   src.Kind == definition.SourceTap,
		Deletable:     src.Kind == definition.SourceTap,
		CreatedAt:     src.CreatedAt.UTC(),
		LastAttemptAt: optTime(src.Status.LastAttemptAt),
		LastSyncedAt:  optTime(src.Status.LastSyncedAt),
		LastError:     src.Status.LastError,
	}
	if src.Kind == definition.SourceOfficial {
		v.URL = source.RedactURL(s.cfg.OfficialURL)
		v.HasToken = s.cfg.OfficialToken != ""
		v.SyncEnabled = s.cfg.OfficialURL != ""
		// The environment-configured URL is trusted with both (see target).
		v.AllowInsecure, v.AllowPrivate = true, true
	}
	if snap != nil {
		v.Serial = snap.Serial
		v.ETag = snap.ETag
		v.GeneratedAt = optTime(snap.GeneratedAt)
		v.AddonCount = len(snap.Addons)
	}
	return v
}

// Sources lists all sources: official, taps by name, local.
func (s *Service) Sources(ctx context.Context) ([]SourceView, error) {
	sources, err := s.cfg.Store.ListSources(ctx)
	if err != nil {
		return nil, err
	}
	snaps, err := s.cfg.Store.ListSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	bySource := map[string]*store.Snapshot{}
	for i := range snaps {
		bySource[snaps[i].Source] = &snaps[i]
	}
	sortSources(sources)
	out := make([]SourceView, 0, len(sources))
	for _, src := range sources {
		out = append(out, s.view(src, bySource[src.Name]))
	}
	return out, nil
}

// Source returns one source.
func (s *Service) Source(ctx context.Context, name string) (*SourceView, error) {
	src, err := s.cfg.Store.GetSource(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	snap, err := s.snapshot(ctx, name)
	if err != nil {
		return nil, err
	}
	v := s.view(*src, snap)
	return &v, nil
}

// TapInput is the body of a tap registration.
type TapInput struct {
	Name          string `json:"name"`
	URL           string `json:"url"`
	Token         string `json:"token"`
	AllowInsecure bool   `json:"allow_insecure"`
	AllowPrivate  bool   `json:"allow_private"`
}

const (
	maxURLLength   = 2048
	maxTokenLength = 4096
)

// AddTap registers a tap and runs a first sync. A failing first sync doesn't undo the
// registration; the error shows up on the source like any later failure, so the admin can fix
// the remote side and sync again.
func (s *Service) AddTap(ctx context.Context, in TapInput) (*SourceView, error) {
	if err := definition.ValidateTapName(in.Name); err != nil {
		return nil, &ValidationError{Msg: "invalid tap name: " + err.Error()}
	}
	if len(in.URL) > maxURLLength {
		return nil, &ValidationError{Msg: fmt.Sprintf("url is longer than %d characters", maxURLLength)}
	}
	if _, err := source.CheckURL(in.URL, in.AllowInsecure, false); err != nil {
		return nil, &ValidationError{Msg: "invalid url: " + err.Error()}
	}
	if len(in.Token) > maxTokenLength || strings.ContainsFunc(in.Token, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return nil, &ValidationError{Msg: fmt.Sprintf("token must be at most %d printable characters without spaces", maxTokenLength)}
	}
	err := s.cfg.Store.CreateSource(ctx, &store.Source{
		Name:          in.Name,
		Kind:          definition.SourceTap,
		URL:           in.URL,
		Token:         in.Token,
		AllowInsecure: in.AllowInsecure,
		AllowPrivate:  in.AllowPrivate,
		CreatedAt:     s.cfg.Now().UTC(),
	})
	if errors.Is(err, store.ErrExists) {
		return nil, &ConflictError{Msg: fmt.Sprintf("source %q already exists", in.Name)}
	}
	if err != nil {
		return nil, err
	}
	s.log.Info("tap added", "source", in.Name, "allow_insecure", in.AllowInsecure, "allow_private", in.AllowPrivate)
	if err := s.Sync(ctx, in.Name); err != nil {
		s.log.Warn("first sync of new tap failed", "source", in.Name, "error", err)
	}
	return s.Source(ctx, in.Name)
}

// DeleteSource removes a tap and its snapshot. The official and local sources can't be
// deleted: the official one is configured by the environment, and local registrations are
// removed one by one.
func (s *Service) DeleteSource(ctx context.Context, name string) error {
	if name == string(definition.SourceOfficial) || name == string(definition.SourceLocal) {
		return &ConflictError{Msg: fmt.Sprintf("the %s source can't be deleted", name)}
	}
	unlock := s.lock(name)
	defer unlock()
	err := s.cfg.Store.DeleteSource(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	if err == nil {
		s.log.Info("tap deleted", "source", name)
	}
	return err
}
