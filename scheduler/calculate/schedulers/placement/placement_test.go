package placement

import (
	"errors"
	"testing"
)

func TestNormalizeVirtualization(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    []string
		wantErr bool
	}{
		{"nil", nil, nil, false},
		{"single string", "docker", []string{"docker"}, false},
		{"string array", []any{"docker", "unikernel"}, []string{"docker", "unikernel"}, false},
		{"empty array", []any{}, []string{}, false},
		{"non-string element in array", []any{42}, nil, true},
		{"unknown type", 3.14, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeVirtualization(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NormalizeVirtualization(%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestMeetsBasicRequirements(t *testing.T) {
	enough := BaseResources{Virtualization: []string{"docker"}, AvailableMem: 1000, AvailableCPU: 4}

	tests := []struct {
		name      string
		job       BaseResources
		candidate BaseResources
		want      bool
	}{
		{
			"all met",
			BaseResources{Virtualization: []string{"docker"}, AvailableMem: 500, AvailableCPU: 2},
			enough,
			true,
		},
		{
			"exact resource match",
			BaseResources{Virtualization: []string{"docker"}, AvailableMem: 1000, AvailableCPU: 4},
			enough,
			true,
		},
		{
			"candidate supports multiple virt types",
			BaseResources{Virtualization: []string{"docker"}, AvailableMem: 100, AvailableCPU: 1},
			BaseResources{Virtualization: []string{"unikernel", "docker"}, AvailableMem: 1000, AvailableCPU: 4},
			true,
		},
		{
			"empty job virtualization",
			BaseResources{Virtualization: nil, AvailableMem: 100, AvailableCPU: 1},
			enough,
			false,
		},
		{
			"virtualization mismatch",
			BaseResources{Virtualization: []string{"unikernel"}, AvailableMem: 100, AvailableCPU: 1},
			enough,
			false,
		},
		{
			"insufficient CPU",
			BaseResources{Virtualization: []string{"docker"}, AvailableMem: 100, AvailableCPU: 8},
			enough,
			false,
		},
		{
			"insufficient memory",
			BaseResources{Virtualization: []string{"docker"}, AvailableMem: 2000, AvailableCPU: 1},
			enough,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MeetsBasicRequirements(tt.job, tt.candidate); got != tt.want {
				t.Errorf("MeetsBasicRequirements() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMeetsGPURequirements(t *testing.T) {
	docker := []string{"docker"}
	job := func(vgpus int, vram float64) BaseResources {
		return BaseResources{Virtualization: docker, AvailableMem: 100, AvailableCPU: 1, Vgpus: vgpus, Vram: vram}
	}
	node := func(free ...float64) BaseResources {
		return BaseResources{Virtualization: docker, AvailableMem: 1000, AvailableCPU: 4, GpuVramFree: free}
	}

	tests := []struct {
		name      string
		job       BaseResources
		candidate BaseResources
		want      bool
	}{
		{"no GPU needed on GPU-less node", job(0, 0), node(), true},
		{"one GPU on GPU-less node", job(1, 0), node(), false},
		{"two GPUs on single-GPU node", job(2, 0), node(8000), false},
		{"GPU count only, no VRAM request", job(1, 0), node(10), true},
		{"VRAM fits across top two GPUs", job(2, 12000), node(8000, 6000), true},
		{"VRAM does not fit on one GPU", job(1, 12000), node(8000, 6000), false},
		{"VRAM exactly fits", job(1, 8000), node(8000, 6000), true},
		{"VRAM without vgpus implies one GPU, fits", job(0, 7000), node(8000), true},
		{"VRAM without vgpus implies one GPU, GPU-less node", job(0, 7000), node(), false},
		{"VRAM without vgpus implies one GPU, too little", job(0, 9000), node(8000, 6000), false},
		{"unsorted list uses largest GPUs", job(2, 13000), node(1000, 6000, 8000), true},
		{
			"legacy fallback splits installed VRAM evenly",
			job(2, 16000),
			BaseResources{Virtualization: docker, AvailableMem: 1000, AvailableCPU: 4, Vgpus: 2, Vram: 16000},
			true,
		},
		{
			"legacy fallback rejects VRAM above installed total",
			job(2, 16001),
			BaseResources{Virtualization: docker, AvailableMem: 1000, AvailableCPU: 4, Vgpus: 2, Vram: 16000},
			false,
		},
		{
			"legacy fallback rejects more GPUs than installed",
			job(3, 0),
			BaseResources{Virtualization: docker, AvailableMem: 1000, AvailableCPU: 4, Vgpus: 2, Vram: 16000},
			false,
		},
		{
			"reported free VRAM wins over installed total",
			job(1, 8000),
			BaseResources{
				Virtualization: docker, AvailableMem: 1000, AvailableCPU: 4,
				Vgpus: 1, Vram: 16000, GpuVramFree: []float64{4000},
			},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MeetsGPURequirements(tt.job, tt.candidate); got != tt.want {
				t.Errorf("MeetsGPURequirements() = %v, want %v", got, tt.want)
			}
			// CPU, memory and virtualization always fit here, so the combined
			// check must agree with the GPU check alone.
			if got := MeetsBasicRequirements(tt.job, tt.candidate); got != tt.want {
				t.Errorf("MeetsBasicRequirements() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMeetsGPURequirements_NonContainerRuntimes(t *testing.T) {
	gpuLess := BaseResources{AvailableMem: 1000, AvailableCPU: 4}
	for _, virt := range []string{"crosvm", "unikernel"} {
		t.Run(virt, func(t *testing.T) {
			job := BaseResources{Virtualization: []string{virt}, Vgpus: 2, Vram: 8000}
			if !MeetsGPURequirements(job, gpuLess) {
				t.Errorf("%s job must not be filtered on NVIDIA GPU capacity", virt)
			}
		})
	}
}

func TestMeetsGPURequirements_DoesNotMutateCandidate(t *testing.T) {
	candidate := BaseResources{GpuVramFree: []float64{1000, 6000, 8000}}
	MeetsGPURequirements(BaseResources{Virtualization: []string{"docker"}, Vgpus: 2, Vram: 1}, candidate)
	want := []float64{1000, 6000, 8000}
	for i := range want {
		if candidate.GpuVramFree[i] != want[i] {
			t.Fatalf("GpuVramFree mutated: got %v, want %v", candidate.GpuVramFree, want)
		}
	}
}

func TestNoCapacityStatus(t *testing.T) {
	always := func(bool) bool { return true }
	never := func(bool) bool { return false }
	docker := []string{"docker"}
	gpuJob := BaseResources{Virtualization: docker, Vgpus: 1}

	tests := []struct {
		name string
		job  BaseResources
		keep func(bool) bool
		want NegativeSchedulingStatus
	}{
		{"GPU job, near miss exists", gpuJob, always, NoGpuCapacity},
		{"GPU job, nothing passes other checks", gpuJob, never, NoActiveClusterWithCapacity},
		{"VRAM-only job counts as GPU job", BaseResources{Virtualization: docker, Vram: 100}, always, NoGpuCapacity},
		{"non-GPU job", BaseResources{Virtualization: docker}, always, NoActiveClusterWithCapacity},
		{"crosvm job with vgpus", BaseResources{Virtualization: []string{"crosvm"}, Vgpus: 1}, always, NoActiveClusterWithCapacity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NoCapacityStatus(tt.job, []bool{true}, tt.keep); got != tt.want {
				t.Errorf("NoCapacityStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSchedulingErrorString(t *testing.T) {
	tests := []struct {
		status NegativeSchedulingStatus
		want   string
	}{
		{TargetClusterNotFound, "TargetClusterNotFound"},
		{TargetClusterNotActive, "TargetClusterNotActive"},
		{NoActiveClusterWithCapacity, "NoActiveClusterWithCapacity"},
		{NoWorkerCapacity, "NO_WORKER_CAPACITY"},
		{NoQualifiedWorkerFound, "NO_QUALIFIED_WORKER_FOUND"},
		{NoNodeFound, "NO_NODE_FOUND"},
		{NoGpuCapacity, "NoGpuCapacity"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			err := SchedulingError{NegativeSchedulingStatus: tt.status}
			if got := err.Error(); got != tt.want {
				t.Errorf("SchedulingError.Error() = %q, want %q", got, tt.want)
			}
			// Must satisfy the error interface and be detectable with errors.As.
			var target SchedulingError
			if !errors.As(err, &target) {
				t.Error("errors.As failed for SchedulingError")
			}
		})
	}
}
