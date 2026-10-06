package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/errs"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

func TestFindCandidatesComputesActiveFromFreshness(t *testing.T) {
	ctx := context.Background()

	fresh, err := testStore.CreateCandidate(ctx, model.Resource{
		CandidateName:         model.Ptr(uniqueName("fresh")),
		LastModifiedTimestamp: model.Ptr(unixSeconds(time.Now())),
	})
	if err != nil {
		t.Fatalf("create fresh candidate: %v", err)
	}

	stale, err := testStore.CreateCandidate(ctx, model.Resource{
		CandidateName:         model.Ptr(uniqueName("stale")),
		LastModifiedTimestamp: model.Ptr(unixSeconds(time.Now().Add(-time.Hour))),
	})
	if err != nil {
		t.Fatalf("create stale candidate: %v", err)
	}

	// "active" only appears in the response when requested via the
	// resources projection param.
	results, err := testStore.FindCandidates(ctx, bson.M{
		"_id": bson.M{"$in": bson.A{objectID(t, *fresh.ID), objectID(t, *stale.ID)}},
	}, []string{"active"})
	if err != nil {
		t.Fatalf("find candidates: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(results))
	}

	for _, r := range results {
		wantActive := *r.ID == *fresh.ID
		gotActive := r.Active != nil && *r.Active
		if gotActive != wantActive {
			t.Errorf("candidate %v: active = %v, want %v", *r.ID, gotActive, wantActive)
		}
	}
}

func TestBuildCandidateFilterActiveOnlyMatchesFresh(t *testing.T) {
	ctx := context.Background()
	name := uniqueName("active-filter")

	fresh, err := testStore.CreateCandidate(ctx, model.Resource{
		CandidateName:         model.Ptr(name),
		LastModifiedTimestamp: model.Ptr(unixSeconds(time.Now())),
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
	if len(results) != 1 || *results[0].ID != *fresh.ID {
		t.Fatalf("expected only the fresh candidate, got %+v", results)
	}
}

func TestFindCandidatesProjection(t *testing.T) {
	ctx := context.Background()

	created, err := testStore.CreateCandidate(ctx, model.Resource{
		CandidateName:         model.Ptr(uniqueName("projection")),
		LastModifiedTimestamp: model.Ptr(unixSeconds(time.Now())),
		Extra: model.Extra{
			"custom_extra_field": "should only appear when requested",
		},
	})
	if err != nil {
		t.Fatalf("create candidate: %v", err)
	}
	filter := bson.M{"_id": objectID(t, *created.ID)}

	withoutExtra, err := testStore.FindCandidates(ctx, filter, nil)
	if err != nil {
		t.Fatalf("find candidates: %v", err)
	}
	if _, ok := withoutExtra[0].Extra["custom_extra_field"]; ok {
		t.Errorf("custom_extra_field should not be projected by default, got %v", withoutExtra[0].Extra)
	}

	withExtra, err := testStore.FindCandidates(ctx, filter, []string{"custom_extra_field"})
	if err != nil {
		t.Fatalf("find candidates with resources projection: %v", err)
	}
	if withExtra[0].Extra["custom_extra_field"] != "should only appear when requested" {
		t.Errorf("custom_extra_field should be projected when requested, got %v", withExtra[0].Extra)
	}

	// The canonical fields must still be present alongside the extra one.
	if withExtra[0].CandidateName == nil {
		t.Errorf("candidate_name (a canonical field) missing from projection: %+v", withExtra[0])
	}
}

func TestUpdateCandidateInformationCapsHistoryAt100(t *testing.T) {
	ctx := context.Background()

	created, err := testStore.CreateCandidate(ctx, model.Resource{
		CandidateName: model.Ptr(uniqueName("history")),
	})
	if err != nil {
		t.Fatalf("create candidate: %v", err)
	}
	id := *created.ID

	const updates = 150
	var last model.Resource
	for i := range updates {
		last, err = testStore.UpdateCandidateInformation(ctx, id, model.Resource{
			CPUPercent:    model.Ptr(float64(i)),
			MemoryPercent: model.Ptr(float64(i) / 2),
		})
		if err != nil {
			t.Fatalf("update candidate information (iteration %d): %v", i, err)
		}
	}

	if last.CPUHistory == nil || len(*last.CPUHistory) != 100 {
		t.Fatalf("cpu_history length = %d, want 100 (capped)", len(*last.CPUHistory))
	}
	cpuHistory := *last.CPUHistory

	lastEntry := cpuHistory[len(cpuHistory)-1]
	if lastEntry.Value == nil || *lastEntry.Value != float64(updates-1) {
		t.Errorf("last cpu_history entry value = %v, want %v", lastEntry.Value, updates-1)
	}
}

func TestFindCandidateByNameNotFound(t *testing.T) {
	_, err := testStore.FindCandidateByName(context.Background(), uniqueName("does-not-exist"))
	if !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("expected errs.ErrNotFound, got %v", err)
	}
}

func TestFindCandidateByIDInvalidHex(t *testing.T) {
	if _, err := testStore.FindCandidateByID(context.Background(), "not-a-valid-object-id"); err == nil {
		t.Fatal("expected an error for an invalid ObjectID hex string")
	}
}

// objectID parses id into a bson.ObjectID for tests building their own
// $in filter by hand.
func objectID(t *testing.T, id string) bson.ObjectID {
	t.Helper()
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		t.Fatalf("parse object id %q: %v", id, err)
	}
	return oid
}
