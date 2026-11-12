// Package store persists the marketplace sources and their catalog snapshots.
//
// A source's configuration and sync status live in one record, and the last good catalog of
// that source lives in a separate snapshot. The snapshot carries the serial and content hash it
// was accepted with, so the rollback check and the addons it protects are always written
// together in one document and can't drift apart.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/oakestra/oakestra/addons/internal/definition"
)

var (
	// ErrNotFound is returned when a source or snapshot doesn't exist.
	ErrNotFound = errors.New("not found")
	// ErrExists is returned when creating a source whose name is taken.
	ErrExists = errors.New("already exists")
)

// Source is a configured catalog source.
type Source struct {
	Name string
	Kind definition.SourceKind
	// URL is empty for the local source and for the official source when it is set to "off".
	URL string
	// Token is the bearer token for taps. It is never returned by the API. The official
	// source's token comes from the environment and is not stored.
	Token         string
	AllowInsecure bool
	AllowPrivate  bool
	CreatedAt     time.Time
	Status        Status
}

// Status is the outcome of the most recent sync or import.
type Status struct {
	// LastAttemptAt is the time of the last sync or import, successful or not.
	LastAttemptAt time.Time
	// LastSyncedAt is the time of the last successful sync or import.
	LastSyncedAt time.Time
	// LastError is empty when the last attempt succeeded.
	LastError string
}

// Snapshot is the last good catalog of a source.
type Snapshot struct {
	Source string
	// Serial is the index serial the snapshot was accepted with. Lower serials are rejected.
	Serial uint64
	// ContentHash identifies the addons, so an index that reuses a serial with different
	// content can be told apart from a plain refetch.
	ContentHash string
	// ETag is the validator of the HTTP response the snapshot came from, empty for imports.
	ETag        string
	GeneratedAt time.Time
	UpdatedAt   time.Time
	Addons      []definition.Addon
}

// Store is the persistence the marketplace needs. Implementations must be safe for concurrent
// use; the catalog service serializes writes per source on top of that.
type Store interface {
	// ListSources returns all sources sorted by name.
	ListSources(ctx context.Context) ([]Source, error)
	GetSource(ctx context.Context, name string) (*Source, error)
	// CreateSource returns ErrExists if the name is taken.
	CreateSource(ctx context.Context, s *Source) error
	// UpdateSourceConfig replaces kind, URL, token and the allow flags, keeping the status.
	UpdateSourceConfig(ctx context.Context, s *Source) error
	SetStatus(ctx context.Context, name string, st Status) error
	// DeleteSource removes the source and its snapshot.
	DeleteSource(ctx context.Context, name string) error

	GetSnapshot(ctx context.Context, source string) (*Snapshot, error)
	ListSnapshots(ctx context.Context) ([]Snapshot, error)
	PutSnapshot(ctx context.Context, s *Snapshot) error
}

// cloneAddons deep-copies addons. Definitions hold maps and slices, and a shallow copy would
// let a caller mutate what the store holds.
func cloneAddons(in []definition.Addon) []definition.Addon {
	if in == nil {
		return []definition.Addon{}
	}
	b, err := json.Marshal(in)
	if err != nil {
		panic(err)
	}
	var out []definition.Addon
	if err := json.Unmarshal(b, &out); err != nil {
		panic(err)
	}
	for i := range out {
		out[i].Normalize()
	}
	return out
}
