package db

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestCreateAppSetsApplicationID(t *testing.T) {
	ctx := context.Background()

	created, err := testStore.CreateApp(ctx, bson.M{"application_name": uniqueName("app")})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}

	id := ExtractID(created)
	if id == "" {
		t.Fatalf("created app has no usable _id: %v", created)
	}
	if created["applicationID"] != id {
		t.Errorf("applicationID = %v, want %v (own _id)", created["applicationID"], id)
	}
}
