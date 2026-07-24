// Package services implements cross-cutting business logic that sits above
// the db package. Today that is just the webhook dispatcher, ported from
// resource-abstractor/services/hook_service.py.
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"go_resource_abstractor/db"
)

// Hooks dispatches the pre/post webhook events fired around writes to
// applications, jobs, resources and custom resources. Sync (pre_*) hooks
// run before the write and may transform the payload that gets persisted;
// async (post_*) hooks run fire-and-forget after the write completes.
//
// Python's hook_service.py wires this up via a decorator
// (pre_post_hook) that wraps each blueprint write method. Go has no
// equivalent, so handlers call Hooks' methods explicitly around their
// db calls instead.
type Hooks struct {
	store  *db.Store
	client *http.Client
}

// NewHooks builds a dispatcher whose outbound webhook calls are bounded by
// connectTimeout (time to establish the TCP connection) and requestTimeout
// (time to receive the response after that) - the same two knobs as
// HOOK_CONNECT_TIMEOUT/HOOK_REQUEST_TIMEOUT in the Python service.
//
// net/http doesn't expose separate connect/read deadlines the way Python's
// requests library does; this approximates it with a Dialer timeout for the
// connect phase and an overall Client.Timeout covering connect+response for
// the request phase.
func NewHooks(store *db.Store, connectTimeout, requestTimeout time.Duration) *Hooks {
	return &Hooks{
		store: store,
		client: &http.Client{
			Timeout: connectTimeout + requestTimeout,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{Timeout: connectTimeout}).DialContext,
			},
		},
	}
}

// PreCreate runs registered pre_create webhooks for entity against data,
// synchronously, and returns the (possibly transformed) payload to persist.
// On any webhook failure the original data is returned unchanged (fail
// open) - the same error handling call_webhook uses in the Python service.
func (h *Hooks) PreCreate(ctx context.Context, entity string, data map[string]any) map[string]any {
	return h.processSyncHook(ctx, entity, db.EventPreCreate, data)
}

// PreUpdate is PreCreate's counterpart for updates.
func (h *Hooks) PreUpdate(ctx context.Context, entity string, data map[string]any) map[string]any {
	return h.processSyncHook(ctx, entity, db.EventPreUpdate, data)
}

// PostCreate fires registered post_create webhooks for entity in the
// background, one goroutine per matching hook, and returns immediately.
func (h *Hooks) PostCreate(entity, entityID string) {
	h.processAsyncHook(entity, db.EventPostCreate, entityID)
}

// PostUpdate is PostCreate's counterpart for updates.
func (h *Hooks) PostUpdate(entity, entityID string) {
	h.processAsyncHook(entity, db.EventPostUpdate, entityID)
}

// PostDelete is PostCreate's counterpart for deletes.
func (h *Hooks) PostDelete(entity, entityID string) {
	h.processAsyncHook(entity, db.EventPostDelete, entityID)
}

// hookFilter builds the "registered for entity and subscribed to event"
// filter shared by the sync and async lookup paths below.
func hookFilter(entity string, event db.HookEvent) bson.M {
	return bson.M{"entity": entity, "events": bson.M{"$in": bson.A{string(event)}}}
}

func (h *Hooks) processSyncHook(ctx context.Context, entity string, event db.HookEvent, data map[string]any) map[string]any {
	hooks, err := h.store.FindHooks(ctx, hookFilter(entity, event))
	if err != nil {
		slog.Warn("hooks: failed to look up sync hooks", "entity", entity, "event", event, "error", err)
		return data
	}

	for _, hook := range hooks {
		url, _ := hook["webhook_url"].(string)
		if url == "" {
			continue
		}
		data = h.callWebhook(ctx, url, data)
	}
	return data
}

func (h *Hooks) processAsyncHook(entity string, event db.HookEvent, entityID string) {
	// Detached from the caller's context: the write has already completed
	// and the HTTP response is about to be sent, so the lookup and the
	// fire-and-forget POSTs below must outlive the request context.
	ctx := context.Background()

	hooks, err := h.store.FindHooks(ctx, hookFilter(entity, event))
	if err != nil {
		slog.Warn("hooks: failed to look up async hooks", "entity", entity, "event", event, "error", err)
		return
	}

	for _, hook := range hooks {
		url, _ := hook["webhook_url"].(string)
		if url == "" {
			continue
		}
		payload := map[string]any{
			"entity":    entity,
			"entity_id": entityID,
			"event":     string(event),
		}
		go h.callWebhook(ctx, url, payload)
	}
}

// callWebhook POSTs data as JSON to url and returns the JSON-decoded
// response body. On any failure (network error, non-2xx status, or a
// non-JSON response body) it logs a warning and returns data unchanged -
// the same fail-open behavior call_webhook has in hook_service.py.
func (h *Hooks) callWebhook(ctx context.Context, url string, data map[string]any) map[string]any {
	body, err := json.Marshal(data)
	if err != nil {
		slog.Warn("hooks: failed to marshal webhook payload", "url", url, "error", err)
		return data
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		slog.Warn("hooks: failed to build webhook request", "url", url, "error", err)
		return data
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		slog.Warn("hooks: webhook request failed, keeping original data", "url", url, "error", err)
		return data
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Warn("hooks: webhook returned non-2xx, keeping original data", "url", url, "status", resp.StatusCode)
		// Drain before closing so the connection can be reused: async hooks
		// fire repeatedly against the same endpoint.
		_, _ = io.Copy(io.Discard, resp.Body)
		return data
	}

	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		// Non-JSON response body: keep the original data, the same fallback
		// the Python service's JSONDecodeError handling takes.
		_, _ = io.Copy(io.Discard, resp.Body)
		return data
	}
	if decoded == nil {
		// A literal JSON "null" decodes into a nil map. Propagating it would
		// hand every downstream write a nil payload - which inserts an empty
		// document and then panics on the "_id" write-back in
		// db.insertReturning - so treat it like any other unusable response
		// and keep the original data.
		slog.Warn("hooks: webhook returned a null body, keeping original data", "url", url)
		return data
	}
	return decoded
}
