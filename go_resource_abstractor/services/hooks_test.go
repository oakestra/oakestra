package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go_resource_abstractor/db"
)

const testHookTimeout = 3 * time.Second

// fakeRegistry is an in-memory HookRegistry double keyed by (entity, event).
// Registrations are the only state Hooks reads, so a map is a complete
// substitute for the real store, and these tests run without Docker.
type fakeRegistry map[registryKey][]string

type registryKey struct {
	entity string
	event  db.HookEvent
}

func (f fakeRegistry) register(event db.HookEvent, url string) {
	key := registryKey{"entity", event}
	f[key] = append(f[key], url)
}

func (f fakeRegistry) WebhookURLsFor(_ context.Context, entity string, event db.HookEvent) ([]string, error) {
	return f[registryKey{entity, event}], nil
}

// erroringRegistry always fails the lookup, simulating a HookRegistry whose
// backing store is unreachable.
type erroringRegistry struct{ err error }

func (r erroringRegistry) WebhookURLsFor(context.Context, string, db.HookEvent) ([]string, error) {
	return nil, r.err
}

func TestPreCreateTransformsPayload(t *testing.T) {
	ctx := context.Background()
	entity := "entity"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		body["transformed"] = true

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()

	registry := fakeRegistry{}
	registry.register(db.EventPreCreate, server.URL)

	hooks := NewHooks(registry, testHookTimeout, testHookTimeout)
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
	entity := "entity"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	registry := fakeRegistry{}
	registry.register(db.EventPreUpdate, server.URL)

	hooks := NewHooks(registry, testHookTimeout, testHookTimeout)
	original := map[string]any{"name": "widget"}
	result := hooks.PreUpdate(ctx, entity, original)

	if result["name"] != "widget" || len(result) != 1 {
		t.Errorf("expected the original payload unchanged on webhook failure, got %v", result)
	}
}

func TestPreCreateFailsOpenWhenWebhookUnreachable(t *testing.T) {
	ctx := context.Background()
	entity := "entity"

	registry := fakeRegistry{}
	registry.register(db.EventPreCreate, "http://127.0.0.1:1") // nothing listens on port 1

	hooks := NewHooks(registry, testHookTimeout, testHookTimeout)
	original := map[string]any{"name": "widget"}
	result := hooks.PreCreate(ctx, entity, original)

	if result["name"] != "widget" || len(result) != 1 {
		t.Errorf("expected the original payload unchanged when the webhook is unreachable, got %v", result)
	}
}

func TestPostCreateFiresAsyncWebhookWithEntityPayload(t *testing.T) {
	entity := "entity"

	received := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	registry := fakeRegistry{}
	registry.register(db.EventPostCreate, server.URL)

	hooks := NewHooks(registry, testHookTimeout, testHookTimeout)
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
	otherEntity := "other-entity"

	received := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	registry := fakeRegistry{}
	registry.register(db.EventPostDelete, server.URL)

	hooks := NewHooks(registry, testHookTimeout, testHookTimeout)
	hooks.PostDelete(otherEntity, "some-id")

	select {
	case <-received:
		t.Fatal("webhook registered for a different entity should not have fired")
	case <-time.After(500 * time.Millisecond):
		// expected: no request arrived
	}
}

// TestFailsOpenWhenRegistryLookupFails covers a path the old *db.Store-based
// tests couldn't reach without a real Mongo failure: WebhookURLsFor itself
// erroring. Both call paths must fail open - sync returns the payload
// unchanged, async fires no webhooks at all - exactly like a webhook call
// failure does, just one step earlier in the pipeline.
func TestFailsOpenWhenRegistryLookupFails(t *testing.T) {
	ctx := context.Background()
	registry := erroringRegistry{err: errors.New("registry unavailable")}
	hooks := NewHooks(registry, testHookTimeout, testHookTimeout)

	t.Run("sync", func(t *testing.T) {
		original := map[string]any{"name": "widget"}
		result := hooks.PreCreate(ctx, "entity", original)
		if result["name"] != "widget" || len(result) != 1 {
			t.Errorf("expected the original payload unchanged when the registry lookup fails, got %v", result)
		}
	})

	t.Run("async", func(t *testing.T) {
		received := make(chan struct{}, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			received <- struct{}{}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// The registry always errors regardless of entity/event, so this
		// server is never actually reachable through the registry - it only
		// exists so a stray call would be observable.
		hooks.PostCreate("entity", "entity-id-123")

		select {
		case <-received:
			t.Fatal("expected no webhook call when the registry lookup fails")
		case <-time.After(300 * time.Millisecond):
			// expected: no request arrived
		}
	})
}
