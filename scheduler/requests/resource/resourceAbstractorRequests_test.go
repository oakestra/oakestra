package resource

import "testing"

func TestFormatRequestParameters_EmptyValuesOnly(t *testing.T) {
	params := map[string]string{"candidate_name": ""}

	got := formatRequestParameters(params)

	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestFormatQuery_OnlyActiveWhenNoFilters(t *testing.T) {
	got := formatQuery(map[string]string{"candidate_name": ""}, nil)

	want := "?active=true"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestFormatQuery_WithRequestAndResources(t *testing.T) {
	got := formatQuery(
		map[string]string{"candidate_name": "cluster-a"},
		[]string{"_id", "memory"},
	)

	want := "?active=true&candidate_name=cluster-a&_id,memory"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
