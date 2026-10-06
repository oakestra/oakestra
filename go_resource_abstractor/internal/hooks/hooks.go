// Package hooks implements cross-cutting business logic that sits above
// internal/store: the webhook dispatcher, ported from
// resource-abstractor/services/hook_service.py.
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/jsonutil"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

// HookRegistry answers the one question Hooks needs of storage: which
// webhook URLs are registered for a given (entity, event) pair. Narrowing
// the dependency this way keeps Mongo out of a package that has no business
// knowing about the database driver, and lets dispatch be tested against a
// map instead of a real MongoDB.
//
// *store.Store satisfies this via WebhookURLsFor and is the production
// adapter.
type HookRegistry interface {
	WebhookURLsFor(ctx context.Context, entity string, event model.HookEvent) ([]string, error)
}

// Hooks dispatches the pre/post webhook events fired around writes to
// applications, jobs, resources and custom resources. Sync (pre_*) hooks
// run before the write and may transform the payload that gets persisted;
// async (post_*) hooks run fire-and-forget after the write completes.
//
// Every hook payload is a JSON object (map[string]any), not a model type:
// the sync payload is whatever the caller is about to write, before it's
// known to be valid against any particular model, and the async payload is
// just entity/entity_id/event. A caller that wants model types marshals
// into this shape and unmarshals the result back out.
type Hooks struct {
	registry HookRegistry
	client   *http.Client
	logger   *slog.Logger

	// registryTimeout bounds the WebhookURLsFor lookup for async events.
	// That lookup runs inside the goroutine fireAsync spawns, not on the
	// caller's request path, so it has no request context to inherit a
	// deadline from - by the time the goroutine runs, the request's context
	// may already be canceled since the response was already sent.
	registryTimeout time.Duration

	// wg tracks every in-flight async goroutine (both the registry lookup
	// and each webhook call it fans out to), so Close can wait for them to
	// drain instead of letting SIGTERM cut them off mid-flight.
	wg sync.WaitGroup
}

// New builds a dispatcher whose outbound webhook calls are bounded by
// connectTimeout (time to establish the TCP connection) and requestTimeout
// (time to receive the response after that). Their sum also bounds the
// async registry lookup, since that lookup has no request context of its
// own to inherit a deadline from.
//
// net/http has no separate connect/read deadlines, so this approximates it
// with a Dialer timeout for the connect phase and an overall Client.Timeout
// covering connect+response.
func New(registry HookRegistry, connectTimeout, requestTimeout time.Duration, logger *slog.Logger) *Hooks {
	return &Hooks{
		registry: registry,
		client: &http.Client{
			Timeout: connectTimeout + requestTimeout,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{Timeout: connectTimeout}).DialContext,
			},
		},
		logger:          logger,
		registryTimeout: connectTimeout + requestTimeout,
	}
}

// SyncURLs returns the webhook URLs registered for entity's pre_* event, so
// a caller can skip building the hook payload when nothing is listening.
//
// A lookup failure is reported as "none registered", matching the fail-open
// behavior of the dispatch itself.
func (h *Hooks) SyncURLs(ctx context.Context, entity string, event model.HookEvent) []string {
	urls, err := h.registry.WebhookURLsFor(ctx, entity, event)
	if err != nil {
		h.logger.Warn("hooks: failed to look up sync hooks", "entity", entity, "event", event, "error", err)
		return nil
	}
	return urls
}

// RunSync POSTs data to each of urls in turn, threading the response of one
// into the next, and returns the final (possibly transformed) payload to
// persist. Callers get urls from SyncURLs. On any webhook failure the
// payload passes through unchanged (fail open).
func (h *Hooks) RunSync(ctx context.Context, urls []string, data map[string]any) map[string]any {
	for _, url := range urls {
		data = h.callWebhook(ctx, url, data)
	}
	return data
}

// PreCreate runs registered pre_create webhooks for entity against data,
// synchronously, and returns the (possibly transformed) payload to persist.
// On any webhook failure it returns the original data unchanged (fail
// open).
func (h *Hooks) PreCreate(ctx context.Context, entity string, data map[string]any) map[string]any {
	return h.RunSync(ctx, h.SyncURLs(ctx, entity, model.EventPreCreate), data)
}

// PreUpdate runs registered pre_update webhooks for entity against data.
func (h *Hooks) PreUpdate(ctx context.Context, entity string, data map[string]any) map[string]any {
	return h.RunSync(ctx, h.SyncURLs(ctx, entity, model.EventPreUpdate), data)
}

// PostCreate fires registered post_create webhooks for entity in the
// background and returns immediately. The registry lookup and every
// matching webhook call happen in goroutines tracked by Close.
func (h *Hooks) PostCreate(entity, entityID string) {
	h.fireAsync(entity, model.EventPostCreate, entityID)
}

// PostUpdate fires registered post_update webhooks for entity in the
// background.
func (h *Hooks) PostUpdate(entity, entityID string) {
	h.fireAsync(entity, model.EventPostUpdate, entityID)
}

// PostDelete fires registered post_delete webhooks for entity in the
// background.
func (h *Hooks) PostDelete(entity, entityID string) {
	h.fireAsync(entity, model.EventPostDelete, entityID)
}

// Close waits for every in-flight async hook, registry lookups and webhook
// calls alike, to finish, or returns ctx's error if it's canceled first.
// Call it during shutdown after the HTTP server stops accepting requests,
// so post_* events from the last requests served aren't dropped.
func (h *Hooks) Close(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		h.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// fireAsync runs the registry lookup and every resulting webhook call in
// goroutines registered on h.wg, so Close can drain them. The lookup uses
// its own bounded context rather than the caller's, which may already be
// gone by the time this runs (the request has already been answered).
func (h *Hooks) fireAsync(entity string, event model.HookEvent, entityID string) {
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()

		ctx, cancel := context.WithTimeout(context.Background(), h.registryTimeout)
		defer cancel()

		urls, err := h.registry.WebhookURLsFor(ctx, entity, event)
		if err != nil {
			h.logger.Warn("hooks: failed to look up async hooks", "entity", entity, "event", event, "error", err)
			return
		}

		payload := map[string]any{
			"entity":    entity,
			"entity_id": entityID,
			"event":     string(event),
		}

		for _, url := range urls {
			h.wg.Add(1)
			go func() {
				defer h.wg.Done()
				h.callWebhook(context.Background(), url, payload)
			}()
		}
	}()
}

// callWebhook POSTs data as JSON to url and returns the JSON-decoded
// response body. On any failure (network error, non-2xx status, or a
// non-JSON response body) it logs a warning and returns data unchanged.
func (h *Hooks) callWebhook(ctx context.Context, url string, data map[string]any) map[string]any {
	body, err := json.Marshal(data)
	if err != nil {
		h.logger.Warn("hooks: failed to marshal webhook payload", "url", url, "error", err)
		return data
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		h.logger.Warn("hooks: failed to build webhook request", "url", url, "error", err)
		return data
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		h.logger.Warn("hooks: webhook request failed, keeping original data", "url", url, "error", err)
		return data
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		h.logger.Warn("hooks: webhook returned non-2xx, keeping original data", "url", url, "status", resp.StatusCode)
		// Drain before closing so the connection can be reused: async hooks
		// fire repeatedly against the same endpoint.
		_, _ = io.Copy(io.Discard, resp.Body)
		return data
	}

	// Decoded through jsonutil, not encoding/json: this payload goes straight
	// back into the write path, so an integer the hook returned has to stay an
	// integer instead of collapsing to float64 and landing in Mongo as a double.
	var decoded map[string]any
	if err := jsonutil.Decode(resp.Body, &decoded); err != nil {
		// Non-JSON response body: keep the original data.
		_, _ = io.Copy(io.Discard, resp.Body)
		return data
	}
	if decoded == nil {
		// A literal JSON "null" decodes into a nil map, which would insert
		// an empty document downstream. Treat it as an unusable response
		// instead.
		h.logger.Warn("hooks: webhook returned a null body, keeping original data", "url", url)
		return data
	}
	return decoded
}
