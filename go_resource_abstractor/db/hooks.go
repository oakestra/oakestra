package db

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// HookEvent identifies a webhook trigger point, mirroring
// hooks_db.HookEventsEnum.
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

// AllEvents is the full set of event names accepted by POST/PATCH /hooks/,
// mirroring the OneOf validator built from ASYNC_EVENTS + SYNC_EVENTS.
var AllEvents = append(append([]HookEvent{}, AsyncEvents...), SyncEvents...)

// FindHooks lists hooks matching filter (an empty/nil filter lists all).
func (s *Store) FindHooks(ctx context.Context, filter bson.M) ([]bson.M, error) {
	return findAll(ctx, s.Hooks, filter)
}

// FindHookByID looks up a single hook by id.
//
// Deviation from the Python service: hooks_db.find_hook_by_id queries
// {"_id": hook_id} with hook_id left as a raw string rather than an
// ObjectId, so GET /hooks/<id> can never match a real document there. Fixed
// here to decode id as an ObjectID before querying.
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
