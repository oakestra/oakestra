package store

import (
	"context"
	"slices"
	"strings"
	"sync"
)

// Memory is an in-memory Store for tests and local experiments.
type Memory struct {
	mu        sync.Mutex
	sources   map[string]Source
	snapshots map[string]Snapshot
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{sources: map[string]Source{}, snapshots: map[string]Snapshot{}}
}

func (m *Memory) ListSources(_ context.Context) ([]Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Source, 0, len(m.sources))
	for _, s := range m.sources {
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b Source) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func (m *Memory) GetSource(_ context.Context, name string) (*Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sources[name]
	if !ok {
		return nil, ErrNotFound
	}
	return &s, nil
}

func (m *Memory) CreateSource(_ context.Context, s *Source) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sources[s.Name]; ok {
		return ErrExists
	}
	m.sources[s.Name] = *s
	return nil
}

func (m *Memory) UpdateSourceConfig(_ context.Context, s *Source) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.sources[s.Name]
	if !ok {
		return ErrNotFound
	}
	cur.Kind, cur.URL, cur.Token = s.Kind, s.URL, s.Token
	cur.AllowInsecure, cur.AllowPrivate = s.AllowInsecure, s.AllowPrivate
	m.sources[s.Name] = cur
	return nil
}

func (m *Memory) SetStatus(_ context.Context, name string, st Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.sources[name]
	if !ok {
		return ErrNotFound
	}
	cur.Status = st
	m.sources[name] = cur
	return nil
}

func (m *Memory) DeleteSource(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sources[name]; !ok {
		return ErrNotFound
	}
	delete(m.sources, name)
	delete(m.snapshots, name)
	return nil
}

func (m *Memory) GetSnapshot(_ context.Context, source string) (*Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.snapshots[source]
	if !ok {
		return nil, ErrNotFound
	}
	s.Addons = cloneAddons(s.Addons)
	return &s, nil
}

func (m *Memory) ListSnapshots(_ context.Context) ([]Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Snapshot, 0, len(m.snapshots))
	for _, s := range m.snapshots {
		s.Addons = cloneAddons(s.Addons)
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b Snapshot) int { return strings.Compare(a.Source, b.Source) })
	return out, nil
}

func (m *Memory) PutSnapshot(_ context.Context, s *Snapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *s
	c.Addons = cloneAddons(s.Addons)
	m.snapshots[s.Source] = c
	return nil
}
