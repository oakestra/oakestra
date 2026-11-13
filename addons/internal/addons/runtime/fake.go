package runtime

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
)

// Fake is an in-memory Runtime for tests.
type Fake struct {
	mu sync.Mutex
	// Containers, Networks and Volumes are keyed by name. Tests may seed them directly before
	// starting a reconcile.
	Containers map[string]*FakeContainer
	Networks   map[string]NetworkSpec
	Volumes    map[string]VolumeSpec
	Pulled     []string
	// Created records every spec passed to CreateAndStart, in order.
	Created []ContainerSpec
	// PullErr and CreateErr fail the matching image ref or container name.
	PullErr   map[string]error
	CreateErr map[string]error
	// ImageLabelSets are the labels baked into an image, by ref.
	ImageLabelSets map[string]map[string]string
	nextID         int
}

// FakeContainer is a container held by the fake.
type FakeContainer struct {
	Container
	Spec ContainerSpec
	Logs string
}

// NewFake returns an empty fake runtime.
func NewFake() *Fake {
	return &Fake{
		Containers: map[string]*FakeContainer{},
		Networks:   map[string]NetworkSpec{},
		Volumes:    map[string]VolumeSpec{},
		PullErr:    map[string]error{},
		CreateErr:  map[string]error{},

		ImageLabelSets: map[string]map[string]string{},
	}
}

// AddContainer seeds a container, e.g. a core service or a foreign addon.
func (f *Fake) AddContainer(name string, labels map[string]string, state string) *FakeContainer {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	c := &FakeContainer{Container: Container{
		ID: fmt.Sprintf("fake%d", f.nextID), Name: name, Labels: labels, State: state,
	}}
	f.Containers[name] = c
	return c
}

// Names returns the sorted container names.
func (f *Fake) Names() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Sorted(maps.Keys(f.Containers))
}

func (f *Fake) PullImage(_ context.Context, ref string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.PullErr[ref]; err != nil {
		return err
	}
	f.Pulled = append(f.Pulled, ref)
	return nil
}

func (f *Fake) ImageLabels(_ context.Context, ref string) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !slices.Contains(f.Pulled, ref) {
		return nil, fmt.Errorf("image %s not pulled", ref)
	}
	return maps.Clone(f.ImageLabelSets[ref]), nil
}

func (f *Fake) InspectContainer(_ context.Context, name string) (*Container, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.Containers[name]
	if !ok {
		return nil, nil
	}
	out := c.Container
	return &out, nil
}

func hasLabels(have, want map[string]string) bool {
	for k, v := range want {
		if got, ok := have[k]; !ok || got != v {
			return false
		}
	}
	return true
}

func (f *Fake) ListContainers(_ context.Context, labels map[string]string) ([]Container, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Container
	for _, name := range slices.Sorted(maps.Keys(f.Containers)) {
		if c := f.Containers[name]; hasLabels(c.Labels, labels) {
			out = append(out, c.Container)
		}
	}
	return out, nil
}

func (f *Fake) CreateAndStart(_ context.Context, spec ContainerSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.CreateErr[spec.Name]; err != nil {
		return err
	}
	if _, ok := f.Containers[spec.Name]; ok {
		return fmt.Errorf("container name %q is already in use", spec.Name)
	}
	for _, n := range spec.Networks {
		if _, ok := f.Networks[n.Name]; !ok {
			return fmt.Errorf("%w: %s", ErrNetworkMissing, n.Name)
		}
	}
	f.nextID++
	f.Containers[spec.Name] = &FakeContainer{
		Container: Container{ID: fmt.Sprintf("fake%d", f.nextID), Name: spec.Name, Labels: maps.Clone(spec.Labels), State: "running"},
		Spec:      spec,
	}
	f.Created = append(f.Created, spec)
	return nil
}

func (f *Fake) RemoveContainer(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for name, c := range f.Containers {
		if c.ID == id {
			delete(f.Containers, name)
			return nil
		}
	}
	return nil
}

func (f *Fake) ContainerLogs(_ context.Context, id string, _ int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.Containers {
		if c.ID == id {
			return c.Logs, nil
		}
	}
	return "", fmt.Errorf("no container %s", id)
}

func (f *Fake) EnsureNetwork(_ context.Context, spec NetworkSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if have, ok := f.Networks[spec.Name]; ok {
		if spec.Shared || sameOwner(have.Labels, spec.Labels, networkOwnerLabels) {
			return nil
		}
		return fmt.Errorf("network %q: %w", spec.Name, ErrForeign)
	}
	f.Networks[spec.Name] = spec
	return nil
}

func (f *Fake) ListNetworks(_ context.Context, labels map[string]string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, name := range slices.Sorted(maps.Keys(f.Networks)) {
		if hasLabels(f.Networks[name].Labels, labels) {
			out = append(out, name)
		}
	}
	return out, nil
}

func (f *Fake) RemoveNetwork(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Networks, name)
	return nil
}

func (f *Fake) EnsureVolume(_ context.Context, spec VolumeSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if have, ok := f.Volumes[spec.Name]; ok {
		if !sameOwner(have.Labels, spec.Labels, volumeOwnerLabels) {
			return fmt.Errorf("volume %q: %w", spec.Name, ErrForeign)
		}
		if !sameVolumeConfig(have.Driver, have.DriverOpts, spec) {
			return fmt.Errorf("volume %q: %w", spec.Name, ErrVolumeMismatch)
		}
		return nil
	}
	f.Volumes[spec.Name] = spec
	return nil
}
