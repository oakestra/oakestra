package calculate

import (
	"scheduler/calculate/schedulers/cpumemfit"
	"scheduler/calculate/schedulers/random"
	"testing"
)

// TestGetInterestedResources_EmbeddedFields verifies that fields promoted from
// embedded structs (placement.BaseResources) are included in the projection
// list alongside fields declared directly on the concrete type.
func TestGetInterestedResources_EmbeddedFields(t *testing.T) {
	var r cpumemfit.Resources
	fields := getInterestedResources(r)

	want := map[string]bool{
		// From embedded placement.BaseResources:
		"_id":            true,
		"virtualization": true,
		"memory":         true,
		"vcpus":          true,
		"cpu_percent":    true,
		// From cpumemfit.Resources directly:
		"constraints": true,
		"csi_drivers": true,
		"volumes":     true,
	}

	got := make(map[string]bool, len(fields))
	for _, f := range fields {
		got[f] = true
	}

	for field := range want {
		if !got[field] {
			t.Errorf("missing field %q; got %v", field, fields)
		}
	}
	for field := range got {
		if !want[field] {
			t.Errorf("unexpected field %q", field)
		}
	}
}

// TestGetInterestedResources_Cached checks that a second call with the same
// type hits the cache and returns the same result.
func TestGetInterestedResources_Cached(t *testing.T) {
	var r cpumemfit.Resources
	first := getInterestedResources(r)
	second := getInterestedResources(r)

	if len(first) != len(second) {
		t.Errorf("cache inconsistency: first=%v second=%v", first, second)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("[%d] first=%q second=%q", i, first[i], second[i])
		}
	}
}

// TestGetInterestedResources_RandomScheduler verifies the random scheduler's
// resource type, which has a different set of direct fields.
func TestGetInterestedResources_RandomScheduler(t *testing.T) {
	var r random.Resources
	fields := getInterestedResources(r)

	got := make(map[string]bool, len(fields))
	for _, f := range fields {
		got[f] = true
	}

	// BaseResources fields must be present.
	for _, required := range []string{"_id", "virtualization", "memory", "vcpus", "cpu_percent"} {
		if !got[required] {
			t.Errorf("missing field %q in random.Resources projection; got %v", required, fields)
		}
	}

	// cpumemfit-specific fields must NOT be present.
	for _, absent := range []string{"csi_drivers", "volumes"} {
		if got[absent] {
			t.Errorf("unexpected field %q in random.Resources projection", absent)
		}
	}
}
