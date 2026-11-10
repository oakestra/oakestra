package random

import (
	"encoding/json"
	"errors"
	"scheduler/calculate/schedulers/placement"
	"testing"
)

func base(id string, virt []string, mem, cpu float64) placement.BaseResources {
	return placement.BaseResources{ID: id, Virtualization: virt, AvailableMem: mem, AvailableCPU: cpu}
}

// --- Calculate ---

func TestCalculate_EmptyCandidates(t *testing.T) {
	_, err := Scheduler{}.Calculate(Resources{}, nil)
	var se placement.SchedulingError
	if !errors.As(err, &se) || se.NegativeSchedulingStatus != placement.TargetClusterNotActive {
		t.Errorf("expected TargetClusterNotActive, got %v", err)
	}
}

func TestCalculate_AllFilteredOut(t *testing.T) {
	job := Resources{BaseResources: base("j1", []string{"docker"}, 5000, 1)}
	candidates := []Resources{
		{BaseResources: base("c1", []string{"docker"}, 100, 4)},
	}
	_, err := Scheduler{}.Calculate(job, candidates)
	var se placement.SchedulingError
	if !errors.As(err, &se) || se.NegativeSchedulingStatus != placement.NoActiveClusterWithCapacity {
		t.Errorf("expected NoActiveClusterWithCapacity, got %v", err)
	}
}

func TestCalculate_ReturnsQualifyingCandidate(t *testing.T) {
	job := Resources{BaseResources: base("j1", []string{"docker"}, 100, 1)}
	c1 := Resources{BaseResources: base("c1", []string{"docker"}, 1000, 4)}
	c2 := Resources{BaseResources: base("c2", []string{"docker"}, 2000, 8)}

	chosen, err := Scheduler{}.Calculate(job, []Resources{c1, c2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chosen.ID() != "c1" && chosen.ID() != "c2" {
		t.Errorf("got unexpected candidate %q", chosen.ID())
	}
}

func TestCalculate_SingleCandidate_IsDeterministic(t *testing.T) {
	job := Resources{BaseResources: base("j1", []string{"docker"}, 100, 1)}
	c1 := Resources{BaseResources: base("c1", []string{"docker"}, 1000, 4)}

	for range 10 {
		chosen, err := Scheduler{}.Calculate(job, []Resources{c1})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if chosen.ID() != "c1" {
			t.Errorf("expected c1, got %q", chosen.ID())
		}
	}
}

func gpuBase(id string, mem float64, vram float64, free ...float64) placement.BaseResources {
	b := base(id, []string{"docker"}, mem, 4)
	b.Vgpus = 1
	b.Vram = vram
	b.GpuVramFree = free
	return b
}

func TestCalculate_GPUJobPicksGPUCandidate(t *testing.T) {
	job := Resources{BaseResources: gpuBase("j1", 100, 4000)}
	cpuOnly := Resources{BaseResources: base("cpu-only", []string{"docker"}, 4000, 8)}
	gpu := Resources{BaseResources: gpuBase("gpu", 1000, 8000, 8000)}

	for range 10 {
		chosen, err := Scheduler{}.Calculate(job, []Resources{cpuOnly, gpu})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if chosen.ID() != "gpu" {
			t.Errorf("expected gpu, got %q", chosen.ID())
		}
	}
}

func TestCalculate_NoGpuCapacity(t *testing.T) {
	job := Resources{BaseResources: gpuBase("j1", 100, 4000)}
	candidates := []Resources{
		{BaseResources: base("cpu-only", []string{"docker"}, 4000, 8)},
		{BaseResources: gpuBase("small-gpu", 1000, 2000, 2000)},
	}
	_, err := Scheduler{}.Calculate(job, candidates)
	var se placement.SchedulingError
	if !errors.As(err, &se) || se.NegativeSchedulingStatus != placement.NoGpuCapacity {
		t.Errorf("expected NoGpuCapacity, got %v", err)
	}
}

func TestCalculate_GPUJobWithoutMemoryAnywhere(t *testing.T) {
	job := Resources{BaseResources: gpuBase("j1", 5000, 4000)}
	candidates := []Resources{
		{BaseResources: base("cpu-only", []string{"docker"}, 4000, 8)},
		{BaseResources: gpuBase("gpu", 1000, 8000, 8000)},
	}
	_, err := Scheduler{}.Calculate(job, candidates)
	var se placement.SchedulingError
	if !errors.As(err, &se) || se.NegativeSchedulingStatus != placement.NoActiveClusterWithCapacity {
		t.Errorf("expected NoActiveClusterWithCapacity, got %v", err)
	}
}

// --- UnmarshalJSON ---

func TestUnmarshalJSON_GPUFields(t *testing.T) {
	var r Resources
	raw := `{"_id":"r5","virtualization":"docker","vgpus":2,"vram":16000,"gpu_vram_free":[8000,6000.5]}`
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Vgpus != 2 || r.Vram != 16000 {
		t.Errorf("Vgpus, Vram = %d, %v, want 2, 16000", r.Vgpus, r.Vram)
	}
	if len(r.GpuVramFree) != 2 || r.GpuVramFree[0] != 8000 || r.GpuVramFree[1] != 6000.5 {
		t.Errorf("GpuVramFree = %v, want [8000 6000.5]", r.GpuVramFree)
	}
}

func TestUnmarshalJSON_StringVirtualization(t *testing.T) {
	var r Resources
	if err := json.Unmarshal([]byte(`{"_id":"r1","virtualization":"docker","memory":1000,"vcpus":4}`), &r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Virtualization) != 1 || r.Virtualization[0] != "docker" {
		t.Errorf("Virtualization = %v, want [docker]", r.Virtualization)
	}
}

func TestUnmarshalJSON_ArrayVirtualization(t *testing.T) {
	var r Resources
	if err := json.Unmarshal([]byte(`{"_id":"r2","virtualization":["docker","unikernel"]}`), &r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Virtualization) != 2 || r.Virtualization[0] != "docker" || r.Virtualization[1] != "unikernel" {
		t.Errorf("Virtualization = %v, want [docker unikernel]", r.Virtualization)
	}
}

func TestUnmarshalJSON_NullVirtualization(t *testing.T) {
	var r Resources
	if err := json.Unmarshal([]byte(`{"_id":"r3","virtualization":null}`), &r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Virtualization != nil {
		t.Errorf("Virtualization = %v, want nil", r.Virtualization)
	}
}

func TestUnmarshalJSON_InvalidVirtualization(t *testing.T) {
	var r Resources
	if err := json.Unmarshal([]byte(`{"_id":"r4","virtualization":[42]}`), &r); err == nil {
		t.Error("expected error for invalid virtualization element, got nil")
	}
}

// --- ResourceConstraints ---

func TestResourceConstraints_DirectConstraint(t *testing.T) {
	r := Resources{
		Constraints: []Constraints{
			{GenericConstraints: placement.GenericConstraints{Type: "direct", Cluster: "cluster-1", Node: "node-1"}},
		},
	}
	got := r.ResourceConstraints()
	if got["cluster_name"] != "cluster-1" {
		t.Errorf("cluster_name = %q, want cluster-1", got["cluster_name"])
	}
	if got["node_name"] != "node-1" {
		t.Errorf("node_name = %q, want node-1", got["node_name"])
	}
}

func TestResourceConstraints_NonDirectIgnored(t *testing.T) {
	r := Resources{
		Constraints: []Constraints{
			{GenericConstraints: placement.GenericConstraints{Type: "affinity", Cluster: "cluster-1"}},
		},
	}
	if got := r.ResourceConstraints(); len(got) != 0 {
		t.Errorf("expected empty constraints, got %v", got)
	}
}

func TestResourceConstraints_Empty(t *testing.T) {
	r := Resources{}
	if got := r.ResourceConstraints(); len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}
