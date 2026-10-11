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
