package store

import (
	"context"
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/errs"
)

func TestStoreLookupWithMalformedIDIsErrInvalidID(t *testing.T) {
	ctx := context.Background()

	if _, err := testStore.FindJobByID(ctx, "not-an-id", bson.M{}); !errors.Is(err, errs.ErrInvalidID) {
		t.Errorf("FindJobByID with a malformed id: got %v, want errs.ErrInvalidID", err)
	}
}

func TestBuildCandidateFilterWithMalformedCandidateIDIsErrInvalidID(t *testing.T) {
	_, err := BuildCandidateFilter(CandidateFilter{CandidateID: "not-an-id"})
	if !errors.Is(err, errs.ErrInvalidID) {
		t.Errorf("BuildCandidateFilter with a malformed CandidateID: got %v, want errs.ErrInvalidID", err)
	}
}
