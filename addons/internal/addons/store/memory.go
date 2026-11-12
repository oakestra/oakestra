package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/oakestra/oakestra/addons/internal/addons"
)

// Memory is an in-memory Store for tests. It enforces the same uniqueness rule as the Mongo
// index.
type Memory struct {
	mu    sync.Mutex
	next  int
	items map[string]addons.Installed
	order []string
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{items: map[string]addons.Installed{}}
}

func (m *Memory) List(_ context.Context, status addons.Status) ([]addons.Installed, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []addons.Installed
	for _, id := range m.order {
		if a := m.items[id]; status == "" || a.Status == status {
			out = append(out, clone(a))
		}
	}
	return out, nil
}

func (m *Memory) Get(_ context.Context, id string) (addons.Installed, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.items[id]
	if !ok {
		return addons.Installed{}, ErrNotFound
	}
	return clone(a), nil
}

func (m *Memory) Insert(_ context.Context, a addons.Installed) (addons.Installed, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, o := range m.items {
		if o.Current && o.AddonID == a.AddonID {
			return addons.Installed{}, ErrDuplicate
		}
	}
	if a.ID == "" {
		m.next++
		// Same shape as an ObjectID so id validation behaves like it does against Mongo.
		a.ID = fmt.Sprintf("%024x", m.next)
	}
	if _, ok := m.items[a.ID]; ok {
		return addons.Installed{}, fmt.Errorf("id %s is already in use", a.ID)
	}
	a.Current = true
	m.items[a.ID] = clone(a)
	m.order = append(m.order, a.ID)
	return clone(a), nil
}

func (m *Memory) Update(_ context.Context, id string, fn func(*addons.Installed) error) (addons.Installed, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.items[id]
	if !ok {
		return addons.Installed{}, ErrNotFound
	}
	work := clone(cur)
	if err := fn(&work); err != nil {
		if errors.Is(err, ErrAbort) {
			return clone(cur), nil
		}
		return addons.Installed{}, err
	}
	work.ID = id
	m.items[id] = clone(work)
	return clone(work), nil
}

func (m *Memory) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[id]; !ok {
		return ErrNotFound
	}
	delete(m.items, id)
	m.order = slices.DeleteFunc(m.order, func(s string) bool { return s == id })
	return nil
}

// clone deep copies the parts of a record that hold slices and maps, so callers and the store
// can't mutate each other's data.
func clone(a addons.Installed) addons.Installed {
	a.Definition = a.Definition.Clone()
	a.Granted.HostPaths = slices.Clone(a.Granted.HostPaths)
	a.Granted.HostPorts = slices.Clone(a.Granted.HostPorts)
	a.StatusDetails.FailedServices = slices.Clone(a.StatusDetails.FailedServices)
	return a
}
