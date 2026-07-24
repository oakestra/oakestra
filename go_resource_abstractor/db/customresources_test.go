package db

import (
	"context"
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestCustomResourceDefinitionUniqueIndex(t *testing.T) {
	ctx := context.Background()
	resourceType := uniqueName("widget")

	if _, err := testStore.CreateCustomResource(ctx, bson.M{"resource_type": resourceType}); err != nil {
		t.Fatalf("create first definition: %v", err)
	}

	_, err := testStore.CreateCustomResource(ctx, bson.M{"resource_type": resourceType})
	if err == nil {
		t.Fatal("expected a duplicate key error for a reused resource_type")
	}
	if !mongo.IsDuplicateKeyError(err) {
		t.Errorf("expected a duplicate key error, got %v", err)
	}
}

// TestCustomResourceCascadingDelete mirrors
// CustomResourceDefinitionController.delete: deleting a resource type must
// remove every instance of that type along with the definition itself.
func TestCustomResourceCascadingDelete(t *testing.T) {
	ctx := context.Background()
	resourceType := uniqueName("gadget")

	if _, err := testStore.CreateCustomResource(ctx, bson.M{"resource_type": resourceType}); err != nil {
		t.Fatalf("create definition: %v", err)
	}

	for range 3 {
		if _, err := testStore.CreateResource(ctx, resourceType, bson.M{"name": uniqueName("instance")}); err != nil {
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

	if _, err := testStore.FindCustomResourceByType(ctx, resourceType); !errors.Is(err, ErrNotFound) {
		t.Errorf("find deleted definition: got %v, want ErrNotFound", err)
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

	if _, err := testStore.CreateCustomResource(ctx, bson.M{"resource_type": resourceType}); err != nil {
		t.Fatalf("create definition: %v", err)
	}

	created, err := testStore.CreateResource(ctx, resourceType, bson.M{"status": "new"})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	id := ExtractID(created)

	found, err := testStore.FindResourceByID(ctx, resourceType, id)
	if err != nil {
		t.Fatalf("find instance: %v", err)
	}
	if found["status"] != "new" {
		t.Errorf("status = %v, want new", found["status"])
	}

	updated, err := testStore.UpdateResource(ctx, resourceType, id, bson.M{"status": "updated"})
	if err != nil {
		t.Fatalf("update instance: %v", err)
	}
	if updated["status"] != "updated" {
		t.Errorf("status after update = %v, want updated", updated["status"])
	}

	if _, err := testStore.DeleteResource(ctx, resourceType, id); err != nil {
		t.Fatalf("delete instance: %v", err)
	}
	if _, err := testStore.FindResourceByID(ctx, resourceType, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("find deleted instance: got %v, want ErrNotFound", err)
	}
}
