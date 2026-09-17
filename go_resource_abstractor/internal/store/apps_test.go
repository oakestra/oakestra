package store

import (
	"context"
	"testing"

	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

func TestCreateAppSetsApplicationID(t *testing.T) {
	ctx := context.Background()

	created, err := testStore.CreateApp(ctx, model.Application{ApplicationName: model.Ptr(uniqueName("app"))})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}

	if created.ID == nil || *created.ID == "" {
		t.Fatalf("created app has no usable _id: %+v", created)
	}
	if created.ApplicationID == nil || *created.ApplicationID != *created.ID {
		t.Errorf("applicationID = %v, want %v (own _id)", created.ApplicationID, created.ID)
	}
}
