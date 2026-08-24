package db

import (
	"context"
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TestFindHookByIDMatchesByObjectID guards the fix for a latent Python bug:
// hooks_db.find_hook_by_id queries with hook_id as a raw string, so GET
// /hooks/<id> is always 404 there. Asserts the Go port's ObjectID lookup
// actually finds the hook.
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

// TestWebhookURLsForFiltersByEntityAndEventAndSkipsUnusableURLs covers
// WebhookURLsFor, the services.HookRegistry adapter: it must return only
// URLs from hooks matching both entity and event, and must skip a
// registration whose webhook_url is empty or missing rather than returning
// it as a usable URL.
func TestWebhookURLsForFiltersByEntityAndEventAndSkipsUnusableURLs(t *testing.T) {
	ctx := context.Background()
	entity := uniqueName("entity")

	if _, err := testStore.CreateHook(ctx, bson.M{
		"hook_name":   uniqueName("hook-match"),
		"webhook_url": "http://example.invalid/a",
		"entity":      entity,
		"events":      bson.A{string(EventPostCreate), string(EventPreUpdate)},
	}); err != nil {
		t.Fatalf("create matching hook: %v", err)
	}

	if _, err := testStore.CreateHook(ctx, bson.M{
		"hook_name":   uniqueName("hook-wrong-event"),
		"webhook_url": "http://example.invalid/b",
		"entity":      entity,
		"events":      bson.A{string(EventPostDelete)},
	}); err != nil {
		t.Fatalf("create non-matching hook: %v", err)
	}

	if _, err := testStore.CreateHook(ctx, bson.M{
		"hook_name":   uniqueName("hook-wrong-entity"),
		"webhook_url": "http://example.invalid/c",
		"entity":      uniqueName("other-entity"),
		"events":      bson.A{string(EventPostCreate)},
	}); err != nil {
		t.Fatalf("create hook for a different entity: %v", err)
	}

	if _, err := testStore.CreateHook(ctx, bson.M{
		"hook_name":   uniqueName("hook-empty-url"),
		"webhook_url": "",
		"entity":      entity,
		"events":      bson.A{string(EventPostCreate)},
	}); err != nil {
		t.Fatalf("create hook with empty webhook_url: %v", err)
	}

	if _, err := testStore.CreateHook(ctx, bson.M{
		"hook_name": uniqueName("hook-missing-url"),
		"entity":    entity,
		"events":    bson.A{string(EventPostCreate)},
	}); err != nil {
		t.Fatalf("create hook with missing webhook_url: %v", err)
	}

	urls, err := testStore.WebhookURLsFor(ctx, entity, EventPostCreate)
	if err != nil {
		t.Fatalf("webhook urls for: %v", err)
	}
	if len(urls) != 1 || urls[0] != "http://example.invalid/a" {
		t.Fatalf("expected only the matching hook's URL, got %v", urls)
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
