package store

import (
	"context"
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/errs"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

// hexID extracts doc["_id"] as a hex string. A document read back as
// bson.M (rather than decoded into a model's `ID *string` field) keeps its
// _id as a bson.ObjectID - the collections' ObjectIDAsHexString option only
// converts on decode into a concrete string field, not into an `any` map
// slot - so tests comparing ids need to convert it themselves. The JSON
// wire format is unaffected: encoding/json calls ObjectID's own
// MarshalJSON regardless.
func hexID(t *testing.T, doc bson.M) string {
	t.Helper()
	oid, ok := doc["_id"].(bson.ObjectID)
	if !ok {
		t.Fatalf("doc[_id] = %#v, want bson.ObjectID", doc["_id"])
	}
	return oid.Hex()
}

// FindHookByID must match by ObjectID, not by a raw hex string (the bug
// GET /hooks/<id> hit in the Python service).
func TestFindHookByIDMatchesByObjectID(t *testing.T) {
	ctx := context.Background()

	created, err := testStore.CreateHook(ctx, model.Hook{
		HookName:   model.Ptr(uniqueName("hook")),
		WebhookURL: model.Ptr("http://example.invalid/webhook"),
		Entity:     model.Ptr("resources"),
		Events:     model.Ptr([]model.HookEvent{model.EventPostCreate}),
	})
	if err != nil {
		t.Fatalf("create hook: %v", err)
	}

	found, err := testStore.FindHookByID(ctx, *created.ID)
	if err != nil {
		t.Fatalf("find hook by id: %v", err)
	}
	if hexID(t, found) != *created.ID {
		t.Errorf("found hook id %v, want %v", hexID(t, found), *created.ID)
	}
}

func TestFindHooksByEntityAndEvent(t *testing.T) {
	ctx := context.Background()
	entity := uniqueName("entity")

	matching, err := testStore.CreateHook(ctx, model.Hook{
		HookName:   model.Ptr(uniqueName("hook-match")),
		WebhookURL: model.Ptr("http://example.invalid/a"),
		Entity:     model.Ptr(entity),
		Events:     model.Ptr([]model.HookEvent{model.EventPostCreate, model.EventPreUpdate}),
	})
	if err != nil {
		t.Fatalf("create matching hook: %v", err)
	}

	if _, err := testStore.CreateHook(ctx, model.Hook{
		HookName:   model.Ptr(uniqueName("hook-nomatch")),
		WebhookURL: model.Ptr("http://example.invalid/b"),
		Entity:     model.Ptr(entity),
		Events:     model.Ptr([]model.HookEvent{model.EventPostDelete}),
	}); err != nil {
		t.Fatalf("create non-matching hook: %v", err)
	}

	results, err := testStore.FindHooks(ctx, bson.M{
		"entity": entity,
		"events": bson.M{"$in": bson.A{string(model.EventPostCreate)}},
	})
	if err != nil {
		t.Fatalf("find hooks: %v", err)
	}
	if len(results) != 1 || hexID(t, results[0]) != *matching.ID {
		t.Fatalf("expected only the matching hook, got %+v", results)
	}
}

// WebhookURLsFor must return only URLs from hooks matching both entity and
// event, and skip a registration whose webhook_url is empty or missing.
func TestWebhookURLsForFiltersByEntityAndEventAndSkipsUnusableURLs(t *testing.T) {
	ctx := context.Background()
	entity := uniqueName("entity")

	if _, err := testStore.CreateHook(ctx, model.Hook{
		HookName:   model.Ptr(uniqueName("hook-match")),
		WebhookURL: model.Ptr("http://example.invalid/a"),
		Entity:     model.Ptr(entity),
		Events:     model.Ptr([]model.HookEvent{model.EventPostCreate, model.EventPreUpdate}),
	}); err != nil {
		t.Fatalf("create matching hook: %v", err)
	}

	if _, err := testStore.CreateHook(ctx, model.Hook{
		HookName:   model.Ptr(uniqueName("hook-wrong-event")),
		WebhookURL: model.Ptr("http://example.invalid/b"),
		Entity:     model.Ptr(entity),
		Events:     model.Ptr([]model.HookEvent{model.EventPostDelete}),
	}); err != nil {
		t.Fatalf("create non-matching hook: %v", err)
	}

	if _, err := testStore.CreateHook(ctx, model.Hook{
		HookName:   model.Ptr(uniqueName("hook-wrong-entity")),
		WebhookURL: model.Ptr("http://example.invalid/c"),
		Entity:     model.Ptr(uniqueName("other-entity")),
		Events:     model.Ptr([]model.HookEvent{model.EventPostCreate}),
	}); err != nil {
		t.Fatalf("create hook for a different entity: %v", err)
	}

	if _, err := testStore.CreateHook(ctx, model.Hook{
		HookName:   model.Ptr(uniqueName("hook-empty-url")),
		WebhookURL: model.Ptr(""),
		Entity:     model.Ptr(entity),
		Events:     model.Ptr([]model.HookEvent{model.EventPostCreate}),
	}); err != nil {
		t.Fatalf("create hook with empty webhook_url: %v", err)
	}

	if _, err := testStore.CreateHook(ctx, model.Hook{
		HookName: model.Ptr(uniqueName("hook-missing-url")),
		Entity:   model.Ptr(entity),
		Events:   model.Ptr([]model.HookEvent{model.EventPostCreate}),
	}); err != nil {
		t.Fatalf("create hook with missing webhook_url: %v", err)
	}

	urls, err := testStore.WebhookURLsFor(ctx, entity, model.EventPostCreate)
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

	base := model.Hook{
		HookName:   model.Ptr(name),
		WebhookURL: model.Ptr("http://example.invalid/first"),
		Entity:     model.Ptr("resources"),
		Events:     model.Ptr([]model.HookEvent{model.EventPostCreate}),
	}
	if _, err := testStore.CreateHook(ctx, base); err != nil {
		t.Fatalf("create first hook: %v", err)
	}

	dup := model.Hook{
		HookName:   model.Ptr(name),
		WebhookURL: model.Ptr("http://example.invalid/second"),
		Entity:     model.Ptr("jobs"),
		Events:     model.Ptr([]model.HookEvent{model.EventPostDelete}),
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

	created, err := testStore.CreateHook(ctx, model.Hook{
		HookName:   model.Ptr(uniqueName("to-delete")),
		WebhookURL: model.Ptr("http://example.invalid/delete-me"),
		Entity:     model.Ptr("resources"),
		Events:     model.Ptr([]model.HookEvent{model.EventPostDelete}),
	})
	if err != nil {
		t.Fatalf("create hook: %v", err)
	}
	id := *created.ID

	if err := testStore.DeleteHook(ctx, id); err != nil {
		t.Fatalf("delete hook: %v", err)
	}

	if _, err := testStore.FindHookByID(ctx, id); !errors.Is(err, errs.ErrNotFound) {
		t.Errorf("find deleted hook: got %v, want errs.ErrNotFound", err)
	}
}

// A hand-edited hook document with a wrongly typed field (webhook_url as a
// number rather than a string) must not take down FindHooks/FindHookByID
// for the whole collection: HEAD returned every stored hook verbatim, and
// decoding into model.Hook would fail the entire list on this one document.
func TestFindHooksToleratesMalformedDocument(t *testing.T) {
	ctx := context.Background()
	entity := uniqueName("entity")

	valid, err := testStore.CreateHook(ctx, model.Hook{
		HookName:   model.Ptr(uniqueName("hook-valid")),
		WebhookURL: model.Ptr("http://example.invalid/valid"),
		Entity:     model.Ptr(entity),
		Events:     model.Ptr([]model.HookEvent{model.EventPostCreate}),
	})
	if err != nil {
		t.Fatalf("create valid hook: %v", err)
	}

	malformedID := bson.NewObjectID()
	malformed := bson.M{
		"_id":         malformedID,
		"hook_name":   uniqueName("hook-broken"),
		"webhook_url": int32(42),
		"entity":      entity,
		"events":      bson.A{string(model.EventPostCreate)},
	}
	if _, err := testStore.hooks.InsertOne(ctx, malformed); err != nil {
		t.Fatalf("insert malformed hook: %v", err)
	}

	results, err := testStore.FindHooks(ctx, bson.M{"entity": entity})
	if err != nil {
		t.Fatalf("find hooks: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected the valid hook and the malformed one, got %+v", results)
	}
	var sawValid, sawMalformed bool
	for _, r := range results {
		switch hexID(t, r) {
		case *valid.ID:
			sawValid = true
		case malformedID.Hex():
			sawMalformed = true
			if r["webhook_url"] != int32(42) {
				t.Errorf("malformed hook webhook_url = %#v, want int32(42) verbatim", r["webhook_url"])
			}
		}
	}
	if !sawValid || !sawMalformed {
		t.Errorf("expected both the valid and malformed hooks in the list, got %+v", results)
	}

	found, err := testStore.FindHookByID(ctx, malformedID.Hex())
	if err != nil {
		t.Fatalf("find malformed hook by id: %v", err)
	}
	if found["webhook_url"] != int32(42) {
		t.Errorf("found malformed hook webhook_url = %#v, want int32(42) verbatim", found["webhook_url"])
	}
}
