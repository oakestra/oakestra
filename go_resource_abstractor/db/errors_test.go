package db

import (
	"context"
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestStoreLookupWithMalformedIDIsErrInvalidID(t *testing.T) {
	ctx := context.Background()

	if _, err := testStore.FindJobByID(ctx, "not-an-id", bson.M{}); !errors.Is(err, ErrInvalidID) {
		t.Errorf("FindJobByID with a malformed id: got %v, want ErrInvalidID", err)
	}
}

func TestBuildCandidateFilterWithMalformedCandidateIDIsErrInvalidID(t *testing.T) {
	_, err := BuildCandidateFilter(CandidateFilter{CandidateID: "not-an-id"})
	if !errors.Is(err, ErrInvalidID) {
		t.Errorf("BuildCandidateFilter with a malformed CandidateID: got %v, want ErrInvalidID", err)
	}
}
