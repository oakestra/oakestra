package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

const testHookTimeout = 3 * time.Second

// testLogger discards output: these tests assert on webhook side effects,
// not on log lines, and a nil *slog.Logger would panic the first time Hooks
// logs a warning (every fail-open path does).
var testLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// fakeRegistry is an in-memory HookRegistry double keyed by (entity, event).
// Registrations are the only state Hooks reads, so a map is a complete
// substitute for the real store, and these tests run without Docker.
type fakeRegistry map[registryKey][]string

type registryKey struct {
	entity string
	event  model.HookEvent
}

func (f fakeRegistry) register(event model.HookEvent, url string) {
	key := registryKey{"entity", event}
	f[key] = append(f[key], url)
}

func (f fakeRegistry) WebhookURLsFor(_ context.Context, entity string, event model.HookEvent) ([]string, error) {
	return f[registryKey{entity, event}], nil
}

// erroringRegistry always fails the lookup, simulating a HookRegistry whose
// backing store is unreachable.
type erroringRegistry struct{ err error }

func (r erroringRegistry) WebhookURLsFor(context.Context, string, model.HookEvent) ([]string, error) {
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
	registry.register(model.EventPreCreate, server.URL)

	h := New(registry, testHookTimeout, testHookTimeout, testLogger)
	result := h.PreCreate(ctx, entity, map[string]any{"name": "widget"})

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
	registry.register(model.EventPreUpdate, server.URL)

	h := New(registry, testHookTimeout, testHookTimeout, testLogger)
	original := map[string]any{"name": "widget"}
	result := h.PreUpdate(ctx, entity, original)

	if result["name"] != "widget" || len(result) != 1 {
		t.Errorf("expected the original payload unchanged on webhook failure, got %v", result)
	}
}

func TestPreCreateFailsOpenWhenWebhookUnreachable(t *testing.T) {
	ctx := context.Background()
	entity := "entity"

	registry := fakeRegistry{}
	registry.register(model.EventPreCreate, "http://127.0.0.1:1") // nothing listens on port 1

	h := New(registry, testHookTimeout, testHookTimeout, testLogger)
	original := map[string]any{"name": "widget"}
	result := h.PreCreate(ctx, entity, original)

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
	registry.register(model.EventPostCreate, server.URL)

	h := New(registry, testHookTimeout, testHookTimeout, testLogger)
	h.PostCreate(entity, "entity-id-123")

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
	registry.register(model.EventPostDelete, server.URL)

	h := New(registry, testHookTimeout, testHookTimeout, testLogger)
	h.PostDelete(otherEntity, "some-id")

	select {
	case <-received:
		t.Fatal("webhook registered for a different entity should not have fired")
	case <-time.After(500 * time.Millisecond):
		// expected: no request arrived
	}
}

// When WebhookURLsFor itself errors, sync must return the payload
// unchanged and async must fire no webhooks.
func TestFailsOpenWhenRegistryLookupFails(t *testing.T) {
	ctx := context.Background()
	registry := erroringRegistry{err: errors.New("registry unavailable")}
	h := New(registry, testHookTimeout, testHookTimeout, testLogger)

	t.Run("sync", func(t *testing.T) {
		original := map[string]any{"name": "widget"}
		result := h.PreCreate(ctx, "entity", original)
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

		// The registry always errors, so this server should never be
		// reached; it only exists to catch a stray call.
		h.PostCreate("entity", "entity-id-123")

		select {
		case <-received:
			t.Fatal("expected no webhook call when the registry lookup fails")
		case <-time.After(300 * time.Millisecond):
			// expected: no request arrived
		}
	})
}

// Close must block until a slow, already in-flight webhook call finishes,
// so a SIGTERM during shutdown can't silently drop a post_* delivery
// mid-flight.
func TestCloseDrainsInFlightAsyncHooks(t *testing.T) {
	entity := "entity"

	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	delivered := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-releaseRequest
		delivered <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	registry := fakeRegistry{}
	registry.register(model.EventPostCreate, server.URL)

	// Long enough that Close can't outrun the handler by timing out first;
	// the test itself controls when the handler returns via releaseRequest.
	h := New(registry, testHookTimeout, testHookTimeout, testLogger)
	h.PostCreate(entity, "entity-id-123")

	select {
	case <-requestStarted:
	case <-time.After(testHookTimeout):
		t.Fatal("timed out waiting for the async webhook request to start")
	}

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- h.Close(context.Background())
	}()

	// Close must still be waiting: the handler hasn't returned yet.
	select {
	case <-closeDone:
		t.Fatal("Close returned before the in-flight webhook call finished")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseRequest)

	select {
	case err := <-closeDone:
		if err != nil {
			t.Errorf("Close returned an error after a normal drain: %v", err)
		}
	case <-time.After(testHookTimeout):
		t.Fatal("Close did not return after the in-flight webhook call finished")
	}

	select {
	case <-delivered:
	default:
		t.Error("webhook handler ran but did not report delivery before Close returned")
	}
}

// Close gives up rather than blocking forever when a hook is stuck.
func TestCloseReturnsContextErrorWhenHooksDoNotDrainInTime(t *testing.T) {
	entity := "entity"

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer func() {
		close(release)
		server.Close()
	}()

	registry := fakeRegistry{}
	registry.register(model.EventPostCreate, server.URL)

	h := New(registry, testHookTimeout, testHookTimeout, testLogger)
	h.PostCreate(entity, "entity-id-123")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := h.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Close error = %v, want context.DeadlineExceeded", err)
	}
}
