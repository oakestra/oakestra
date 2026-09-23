package store

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

// FindHooks lists hooks matching filter (an empty/nil filter lists all).
//
// Decoded as bson.M rather than model.Hook: a hand-edited registration with
// a wrongly typed field (e.g. a numeric webhook_url) would otherwise fail
// to decode and take the whole list down with it. HEAD returned every
// stored hook document verbatim, malformed or not, and this preserves that.
func (s *Store) FindHooks(ctx context.Context, filter bson.M) ([]bson.M, error) {
	return findAll[bson.M](ctx, s.hooks, filter)
}

// WebhookURLsFor returns the webhook URLs registered for entity that are
// subscribed to event, skipping any registration whose webhook_url is
// missing, empty, or not a string, so callers can dispatch to every URL
// they get without re-checking. This satisfies internal/hooks.HookRegistry,
// so a *Store is the production hook lookup.
func (s *Store) WebhookURLsFor(ctx context.Context, entity string, event model.HookEvent) ([]string, error) {
	// Decoded as bson.M rather than model.Hook: one registration with a
	// non-string field would otherwise fail the whole decode, and the
	// dispatcher fails open on a lookup error, silently skipping every other
	// valid hook for this entity/event.
	hooks, err := findAll[bson.M](ctx, s.hooks, bson.M{"entity": entity, "events": bson.M{"$in": bson.A{string(event)}}})
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

// FindHookByID looks up a single hook by id, returned verbatim (see
// FindHooks).
//
// Python's hooks_db.find_hook_by_id queries {"_id": hook_id} with hook_id
// as a raw string, so GET /hooks/<id> never matches there. This decodes id
// as an ObjectID first, so it actually works.
func (s *Store) FindHookByID(ctx context.Context, id string) (bson.M, error) {
	return findByID[bson.M](ctx, s.hooks, id, nil)
}

// CreateHook inserts a new hook registration, dropping any client-supplied
// _id. Unlike the read paths, the inserted document comes straight from
// data, which the REST layer has already decoded (and so validated) into a
// model.Hook, so decoding the result back into model.Hook can't hit the
// malformed-field problem FindHooks/FindHookByID guard against.
func (s *Store) CreateHook(ctx context.Context, data model.Hook) (model.Hook, error) {
	return insertReturning(ctx, s.hooks, data)
}

// UpdateHook applies a plain $set update, dropping any client-supplied _id,
// and returns the document as it stands after the update.
//
// The result is decoded as bson.M rather than model.Hook: a $set patch only
// touches the fields it names, so any other, previously malformed field on
// the stored document (see FindHooks) survives the update untouched and
// must still be safe to read back.
func (s *Store) UpdateHook(ctx context.Context, id string, data model.Hook) (bson.M, error) {
	oid, err := parseObjectID(id)
	if err != nil {
		return nil, err
	}
	doc, err := toSetDoc(data)
	if err != nil {
		return nil, err
	}
	return findOneAndUpdate[bson.M](ctx, s.hooks, bson.M{"_id": oid}, bson.M{"$set": doc})
}

// DeleteHook removes a hook by id.
func (s *Store) DeleteHook(ctx context.Context, id string) error {
	return deleteByID(ctx, s.hooks, id)
}
