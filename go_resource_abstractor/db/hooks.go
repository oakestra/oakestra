package db

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// HookEvent identifies a webhook trigger point, the same set
// hooks_db.HookEventsEnum declares.
type HookEvent string

const (
	EventPostCreate HookEvent = "post_create"
	EventPreCreate  HookEvent = "pre_create"
	EventPostUpdate HookEvent = "post_update"
	EventPreUpdate  HookEvent = "pre_update"
	EventPostDelete HookEvent = "post_delete"
	EventPreDelete  HookEvent = "pre_delete"
)

// AsyncEvents fire fire-and-forget after the write completes.
var AsyncEvents = []HookEvent{EventPostCreate, EventPostUpdate, EventPostDelete}

// SyncEvents fire synchronously before the write and may transform the
// payload that gets persisted.
var SyncEvents = []HookEvent{EventPreCreate, EventPreUpdate, EventPreDelete}

// The full set of event names POST/PATCH /hooks/ accepts is not repeated
// here: it is the HookEvent enum in openapi/openapi.yaml, and the API layer
// validates against the membership test generated from it.

// FindHooks lists hooks matching filter (an empty/nil filter lists all).
func (s *Store) FindHooks(ctx context.Context, filter bson.M) ([]bson.M, error) {
	return findAll(ctx, s.Hooks, filter)
}

// WebhookURLsFor returns the webhook URLs registered for entity that are
// subscribed to event. A registration whose webhook_url is missing, empty,
// or not a string is skipped rather than returned, so callers can dispatch
// to every URL they get back without re-checking.
func (s *Store) WebhookURLsFor(ctx context.Context, entity string, event HookEvent) ([]string, error) {
	hooks, err := findAll(ctx, s.Hooks, bson.M{"entity": entity, "events": bson.M{"$in": bson.A{string(event)}}})
	if err != nil {
		return nil, err
	}

	urls := make([]string, 0, len(hooks))
	for _, hook := range hooks {
		if url, ok := hook["webhook_url"].(string); ok && url != "" {
			urls = append(urls, url)
		}
	}
	return urls, nil
}

// FindHookByID looks up a single hook by id.
//
// Deviation from Python's hooks_db.find_hook_by_id, which queries
// {"_id": hook_id} with hook_id as a raw string - so GET /hooks/<id> can
// never match there. Fixed here by decoding id as an ObjectID first.
func (s *Store) FindHookByID(ctx context.Context, id string) (bson.M, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	var hook bson.M
	if err := s.Hooks.FindOne(ctx, bson.M{"_id": oid}).Decode(&hook); err != nil {
		return nil, err
	}
	return hook, nil
}

// CreateHook inserts a new hook registration, dropping any client-supplied
// _id.
func (s *Store) CreateHook(ctx context.Context, data bson.M) (bson.M, error) {
	return insertReturning(ctx, s.Hooks, data)
}

// UpdateHook applies a plain $set update, dropping any client-supplied _id.
func (s *Store) UpdateHook(ctx context.Context, id string, data bson.M) (bson.M, error) {
	return updateByID(ctx, s.Hooks, id, data)
}

// DeleteHook removes a hook by id.
func (s *Store) DeleteHook(ctx context.Context, id string) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	_, err = s.Hooks.DeleteOne(ctx, bson.M{"_id": oid})
	return err
}
