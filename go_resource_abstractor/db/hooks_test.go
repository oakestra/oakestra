package db

import (
	"context"
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TestFindHookByIDMatchesByObjectID exercises the fix for a latent bug in
// the Python service: hooks_db.find_hook_by_id queries {"_id": hook_id}
// with hook_id left as a raw string rather than an ObjectId, so it can
// never match a real document (GET /hooks/<id> is always 404 there). This
// asserts the Go port's ObjectID-based lookup actually finds the hook.
func TestFindHookByIDMatchesByObjectID(t *testing.T) {
	ctx := context.Background()

	created, err := testStore.CreateHook(ctx, bson.M{
		"hook_name":   uniqueName("hook"),
		"webhook_url": "http://example.invalid/webhook",
		"entity":      "resources",
		"events":      bson.A{string(EventPostCreate)},
	})
	if err != nil {
		t.Fatalf("create hook: %v", err)
	}

	found, err := testStore.FindHookByID(ctx, ExtractID(created))
	if err != nil {
		t.Fatalf("find hook by id: %v", err)
	}
	if ExtractID(found) != ExtractID(created) {
		t.Errorf("found hook id %v, want %v", ExtractID(found), ExtractID(created))
	}
}

func TestFindHooksByEntityAndEvent(t *testing.T) {
	ctx := context.Background()
	entity := uniqueName("entity")

	matching, err := testStore.CreateHook(ctx, bson.M{
		"hook_name":   uniqueName("hook-match"),
		"webhook_url": "http://example.invalid/a",
		"entity":      entity,
		"events":      bson.A{string(EventPostCreate), string(EventPreUpdate)},
	})
	if err != nil {
		t.Fatalf("create matching hook: %v", err)
	}

	if _, err := testStore.CreateHook(ctx, bson.M{
		"hook_name":   uniqueName("hook-nomatch"),
		"webhook_url": "http://example.invalid/b",
		"entity":      entity,
		"events":      bson.A{string(EventPostDelete)},
	}); err != nil {
		t.Fatalf("create non-matching hook: %v", err)
	}

	results, err := testStore.FindHooks(ctx, bson.M{
		"entity": entity,
		"events": bson.M{"$in": bson.A{string(EventPostCreate)}},
	})
	if err != nil {
		t.Fatalf("find hooks: %v", err)
	}
	if len(results) != 1 || ExtractID(results[0]) != ExtractID(matching) {
		t.Fatalf("expected only the matching hook, got %v", results)
	}
}

func TestHookNameUniqueIndex(t *testing.T) {
	ctx := context.Background()
	name := uniqueName("dup-hook")

	base := bson.M{
		"hook_name":   name,
		"webhook_url": "http://example.invalid/first",
		"entity":      "resources",
		"events":      bson.A{string(EventPostCreate)},
	}
	if _, err := testStore.CreateHook(ctx, base); err != nil {
		t.Fatalf("create first hook: %v", err)
	}

	dup := bson.M{
		"hook_name":   name,
		"webhook_url": "http://example.invalid/second",
		"entity":      "jobs",
		"events":      bson.A{string(EventPostDelete)},
	}
	_, err := testStore.CreateHook(ctx, dup)
	if err == nil {
		t.Fatal("expected a duplicate key error for a reused hook_name")
	}
	if !mongo.IsDuplicateKeyError(err) {
		t.Errorf("expected a duplicate key error, got %v", err)
	}
}

func TestDeleteHook(t *testing.T) {
	ctx := context.Background()

	created, err := testStore.CreateHook(ctx, bson.M{
		"hook_name":   uniqueName("to-delete"),
		"webhook_url": "http://example.invalid/delete-me",
		"entity":      "resources",
		"events":      bson.A{string(EventPostDelete)},
	})
	if err != nil {
		t.Fatalf("create hook: %v", err)
	}
	id := ExtractID(created)

	if err := testStore.DeleteHook(ctx, id); err != nil {
		t.Fatalf("delete hook: %v", err)
	}

	if _, err := testStore.FindHookByID(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("find deleted hook: got %v, want ErrNotFound", err)
	}
}
