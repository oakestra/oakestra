package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const testHookTimeout = 3 * time.Second

func TestPreCreateTransformsPayload(t *testing.T) {
	ctx := context.Background()
	entity := uniqueName("entity")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		body["transformed"] = true

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()

	if _, err := testStore.CreateHook(ctx, bson.M{
		"hook_name":   uniqueName("hook"),
		"webhook_url": server.URL,
		"entity":      entity,
		"events":      bson.A{"pre_create"},
	}); err != nil {
		t.Fatalf("register hook: %v", err)
	}

	hooks := NewHooks(testStore, testHookTimeout, testHookTimeout)
	result := hooks.PreCreate(ctx, entity, map[string]any{"name": "widget"})

	if result["name"] != "widget" {
		t.Errorf("original field lost: %v", result)
	}
	if transformed, _ := result["transformed"].(bool); !transformed {
		t.Errorf("expected the webhook's transformation to be applied, got %v", result)
	}
}

func TestPreUpdateFailsOpenOnWebhookError(t *testing.T) {
	ctx := context.Background()
	entity := uniqueName("entity")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	if _, err := testStore.CreateHook(ctx, bson.M{
		"hook_name":   uniqueName("hook"),
		"webhook_url": server.URL,
		"entity":      entity,
		"events":      bson.A{"pre_update"},
	}); err != nil {
		t.Fatalf("register hook: %v", err)
	}

	hooks := NewHooks(testStore, testHookTimeout, testHookTimeout)
	original := map[string]any{"name": "widget"}
	result := hooks.PreUpdate(ctx, entity, original)

	if result["name"] != "widget" || len(result) != 1 {
		t.Errorf("expected the original payload unchanged on webhook failure, got %v", result)
	}
}

func TestPreCreateFailsOpenWhenWebhookUnreachable(t *testing.T) {
	ctx := context.Background()
	entity := uniqueName("entity")

	if _, err := testStore.CreateHook(ctx, bson.M{
		"hook_name":   uniqueName("hook"),
		"webhook_url": "http://127.0.0.1:1", // nothing listens on port 1
		"entity":      entity,
		"events":      bson.A{"pre_create"},
	}); err != nil {
		t.Fatalf("register hook: %v", err)
	}

	hooks := NewHooks(testStore, testHookTimeout, testHookTimeout)
	original := map[string]any{"name": "widget"}
	result := hooks.PreCreate(ctx, entity, original)

	if result["name"] != "widget" || len(result) != 1 {
		t.Errorf("expected the original payload unchanged when the webhook is unreachable, got %v", result)
	}
}

func TestPostCreateFiresAsyncWebhookWithEntityPayload(t *testing.T) {
	ctx := context.Background()
	entity := uniqueName("entity")

	received := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if _, err := testStore.CreateHook(ctx, bson.M{
		"hook_name":   uniqueName("hook"),
		"webhook_url": server.URL,
		"entity":      entity,
		"events":      bson.A{"post_create"},
	}); err != nil {
		t.Fatalf("register hook: %v", err)
	}

	hooks := NewHooks(testStore, testHookTimeout, testHookTimeout)
	hooks.PostCreate(entity, "entity-id-123")

	select {
	case body := <-received:
		if body["entity"] != entity {
			t.Errorf("entity = %v, want %v", body["entity"], entity)
		}
		if body["entity_id"] != "entity-id-123" {
			t.Errorf("entity_id = %v, want entity-id-123", body["entity_id"])
		}
		if body["event"] != "post_create" {
			t.Errorf("event = %v, want post_create", body["event"])
		}
	case <-time.After(testHookTimeout):
		t.Fatal("timed out waiting for the async post_create webhook to fire")
	}
}

func TestPostDeleteDoesNotFireForUnrelatedEntity(t *testing.T) {
	ctx := context.Background()
	entity := uniqueName("entity")
	otherEntity := uniqueName("other-entity")

	received := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if _, err := testStore.CreateHook(ctx, bson.M{
		"hook_name":   uniqueName("hook"),
		"webhook_url": server.URL,
		"entity":      entity,
		"events":      bson.A{"post_delete"},
	}); err != nil {
		t.Fatalf("register hook: %v", err)
	}

	hooks := NewHooks(testStore, testHookTimeout, testHookTimeout)
	hooks.PostDelete(otherEntity, "some-id")

	select {
	case <-received:
		t.Fatal("webhook registered for a different entity should not have fired")
	case <-time.After(500 * time.Millisecond):
		// expected: no request arrived
	}
}
