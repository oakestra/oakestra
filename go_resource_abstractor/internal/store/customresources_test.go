package store

import (
	"context"
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/errs"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

// Without this check, registering "meta_data" as a type would point
// instance writes and the cascading delete at the definitions collection
// itself.
func TestCreateCustomResourceRejectsInvalidResourceType(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
	}{
		{"empty string", ""},
		{"reserved meta_data name", "meta_data"},
		{"system prefix", "system.foo"},
		{"dollar sign", "a$b"},
		{"nul byte", "a\x00b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			data := model.CustomResourceDefinition{ResourceType: tt.resourceType}
			if _, err := testStore.CreateCustomResource(ctx, data); !errors.Is(err, errs.ErrInvalidResourceType) {
				t.Errorf("CreateCustomResource(%q): got %v, want errs.ErrInvalidResourceType", tt.resourceType, err)
			}
		})
	}
}

func TestCustomResourceDefinitionUniqueIndex(t *testing.T) {
	ctx := context.Background()
	resourceType := uniqueName("widget")

	if _, err := testStore.CreateCustomResource(ctx, model.CustomResourceDefinition{ResourceType: resourceType}); err != nil {
		t.Fatalf("create first definition: %v", err)
	}

	_, err := testStore.CreateCustomResource(ctx, model.CustomResourceDefinition{ResourceType: resourceType})
	if err == nil {
		t.Fatal("expected a duplicate key error for a reused resource_type")
	}
	if !mongo.IsDuplicateKeyError(err) {
		t.Errorf("expected a duplicate key error, got %v", err)
	}
}

// Deleting a resource type must remove every instance of that type along
// with the definition itself.
func TestCustomResourceCascadingDelete(t *testing.T) {
	ctx := context.Background()
	resourceType := uniqueName("gadget")

	if _, err := testStore.CreateCustomResource(ctx, model.CustomResourceDefinition{ResourceType: resourceType}); err != nil {
		t.Fatalf("create definition: %v", err)
	}

	for range 3 {
		instance := model.CustomResourceInstance{Extra: model.Extra{"name": uniqueName("instance")}}
		if _, err := testStore.CreateResource(ctx, resourceType, instance); err != nil {
			t.Fatalf("create instance: %v", err)
		}
	}

	instancesBefore, err := testStore.FindResources(ctx, resourceType, nil)
	if err != nil {
		t.Fatalf("find instances before delete: %v", err)
	}
	if len(instancesBefore) != 3 {
		t.Fatalf("expected 3 instances before delete, got %d", len(instancesBefore))
	}

	deletedCount, err := testStore.DeleteAllResources(ctx, resourceType)
	if err != nil {
		t.Fatalf("delete all resources: %v", err)
	}
	if deletedCount != 3 {
		t.Errorf("deleted count = %d, want 3", deletedCount)
	}

	if _, err := testStore.DeleteCustomResourceByType(ctx, resourceType); err != nil {
		t.Fatalf("delete definition: %v", err)
	}

	if _, err := testStore.FindCustomResourceByType(ctx, resourceType); !errors.Is(err, errs.ErrNotFound) {
		t.Errorf("find deleted definition: got %v, want errs.ErrNotFound", err)
	}

	instancesAfter, err := testStore.FindResources(ctx, resourceType, nil)
	if err != nil {
		t.Fatalf("find instances after delete: %v", err)
	}
	if len(instancesAfter) != 0 {
		t.Errorf("expected 0 instances after cascading delete, got %d", len(instancesAfter))
	}
}

func TestCustomResourceInstanceCRUD(t *testing.T) {
	ctx := context.Background()
	resourceType := uniqueName("thingamajig")

	if _, err := testStore.CreateCustomResource(ctx, model.CustomResourceDefinition{ResourceType: resourceType}); err != nil {
		t.Fatalf("create definition: %v", err)
	}

	created, err := testStore.CreateResource(ctx, resourceType, model.CustomResourceInstance{
		Extra: model.Extra{"status": "new"},
	})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	id := *created.ID

	found, err := testStore.FindResourceByID(ctx, resourceType, id)
	if err != nil {
		t.Fatalf("find instance: %v", err)
	}
	if found.Extra["status"] != "new" {
		t.Errorf("status = %v, want new", found.Extra["status"])
	}

	updated, err := testStore.UpdateResource(ctx, resourceType, id, model.CustomResourceInstance{
		Extra: model.Extra{"status": "updated"},
	})
	if err != nil {
		t.Fatalf("update instance: %v", err)
	}
	if updated.Extra["status"] != "updated" {
		t.Errorf("status after update = %v, want updated", updated.Extra["status"])
	}

	if _, err := testStore.DeleteResource(ctx, resourceType, id); err != nil {
		t.Fatalf("delete instance: %v", err)
	}
	if _, err := testStore.FindResourceByID(ctx, resourceType, id); !errors.Is(err, errs.ErrNotFound) {
		t.Errorf("find deleted instance: got %v, want errs.ErrNotFound", err)
	}
}
