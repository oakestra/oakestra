package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// docValue looks up key in an ordered bson.D document, as returned by
// history-array elements decoded via the v2 driver's default "any" typemap.
func docValue(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

// uniqueName returns a collision-free name for test fixtures, since tests
// share one long-lived MongoDB instance and several fields (hook_name,
// resource_type) carry unique indexes.
func uniqueName(prefix string) string {
	return prefix + "-" + bson.NewObjectID().Hex()
}

func TestFindCandidatesComputesActiveFromFreshness(t *testing.T) {
	ctx := context.Background()

	fresh, err := testStore.CreateCandidate(ctx, bson.M{
		"candidate_name":          uniqueName("fresh"),
		"last_modified_timestamp": unixSeconds(time.Now()),
	})
	if err != nil {
		t.Fatalf("create fresh candidate: %v", err)
	}

	stale, err := testStore.CreateCandidate(ctx, bson.M{
		"candidate_name":          uniqueName("stale"),
		"last_modified_timestamp": unixSeconds(time.Now().Add(-time.Hour)),
	})
	if err != nil {
		t.Fatalf("create stale candidate: %v", err)
	}

	// "active" is a computed field that, like Python's CANONICAL_RESOURCES,
	// is only included in the response when explicitly requested via the
	// resources projection param - which is exactly what system_manager's
	// get_candidates(resources="last_modified_timestamp,active") does.
	results, err := testStore.FindCandidates(ctx, bson.M{
		"_id": bson.M{"$in": bson.A{fresh["_id"], stale["_id"]}},
	}, []string{"active"})
	if err != nil {
		t.Fatalf("find candidates: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(results))
	}

	for _, r := range results {
		wantActive := r["_id"] == fresh["_id"]
		gotActive, _ := r["active"].(bool)
		if gotActive != wantActive {
			t.Errorf("candidate %v: active = %v, want %v", r["_id"], gotActive, wantActive)
		}
	}
}

func TestBuildCandidateFilterActiveOnlyMatchesFresh(t *testing.T) {
	ctx := context.Background()
	name := uniqueName("active-filter")

	fresh, err := testStore.CreateCandidate(ctx, bson.M{
		"candidate_name":          name,
		"last_modified_timestamp": unixSeconds(time.Now()),
	})
	if err != nil {
		t.Fatalf("create candidate: %v", err)
	}

	filter, err := BuildCandidateFilter(CandidateFilter{
		ActiveOnly:    true,
		CandidateName: name,
	})
	if err != nil {
		t.Fatalf("build filter: %v", err)
	}
	if _, ok := filter["active"]; ok {
		t.Errorf("active key should be stripped from the built filter, got %v", filter)
	}

	results, err := testStore.FindCandidates(ctx, filter, nil)
	if err != nil {
		t.Fatalf("find candidates: %v", err)
	}
	if len(results) != 1 || results[0]["_id"] != fresh["_id"] {
		t.Fatalf("expected only the fresh candidate, got %v", results)
	}
}

func TestFindCandidatesProjection(t *testing.T) {
	ctx := context.Background()

	created, err := testStore.CreateCandidate(ctx, bson.M{
		"candidate_name":          uniqueName("projection"),
		"last_modified_timestamp": unixSeconds(time.Now()),
		"custom_extra_field":      "should only appear when requested",
	})
	if err != nil {
		t.Fatalf("create candidate: %v", err)
	}
	filter := bson.M{"_id": created["_id"]}

	withoutExtra, err := testStore.FindCandidates(ctx, filter, nil)
	if err != nil {
		t.Fatalf("find candidates: %v", err)
	}
	if _, ok := withoutExtra[0]["custom_extra_field"]; ok {
		t.Errorf("custom_extra_field should not be projected by default, got %v", withoutExtra[0])
	}

	withExtra, err := testStore.FindCandidates(ctx, filter, []string{"custom_extra_field"})
	if err != nil {
		t.Fatalf("find candidates with resources projection: %v", err)
	}
	if withExtra[0]["custom_extra_field"] != "should only appear when requested" {
		t.Errorf("custom_extra_field should be projected when requested, got %v", withExtra[0])
	}

	// The canonical fields must still be present alongside the extra one.
	if _, ok := withExtra[0]["candidate_name"]; !ok {
		t.Errorf("candidate_name (a canonical field) missing from projection: %v", withExtra[0])
	}
}

func TestUpdateCandidateInformationCapsHistoryAt100(t *testing.T) {
	ctx := context.Background()

	created, err := testStore.CreateCandidate(ctx, bson.M{
		"candidate_name": uniqueName("history"),
	})
	if err != nil {
		t.Fatalf("create candidate: %v", err)
	}
	id := ExtractID(created)

	const updates = 150
	var last bson.M
	for i := range updates {
		last, err = testStore.UpdateCandidateInformation(ctx, id, bson.M{
			"cpu_percent":    float64(i),
			"memory_percent": float64(i) / 2,
		})
		if err != nil {
			t.Fatalf("update candidate information (iteration %d): %v", i, err)
		}
	}

	cpuHistory, ok := last["cpu_history"].(bson.A)
	if !ok {
		t.Fatalf("cpu_history is not an array: %#v", last["cpu_history"])
	}
	if len(cpuHistory) != 100 {
		t.Fatalf("cpu_history length = %d, want 100 (capped)", len(cpuHistory))
	}

	lastEntry, ok := cpuHistory[len(cpuHistory)-1].(bson.D)
	if !ok {
		t.Fatalf("cpu_history entry is not a document: %#v", cpuHistory[len(cpuHistory)-1])
	}
	if got := docValue(lastEntry, "value"); got != float64(updates-1) {
		t.Errorf("last cpu_history entry value = %v, want %v", got, updates-1)
	}
}

func TestFindCandidateByNameNotFound(t *testing.T) {
	_, err := testStore.FindCandidateByName(context.Background(), uniqueName("does-not-exist"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFindCandidateByIDInvalidHex(t *testing.T) {
	if _, err := testStore.FindCandidateByID(context.Background(), "not-a-valid-object-id"); err == nil {
		t.Fatal("expected an error for an invalid ObjectID hex string")
	}
}
