package api

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"

	"go_resource_abstractor/db"
	"go_resource_abstractor/services"
)

// entity bundles a hook channel name ("jobs", "applications", "resources",
// or a custom resource type) with the dispatcher, so a write path can name
// its entity once and let Create/Update/UpdateFound/Delete own the
// pre-event/store-call/post-event choreography around it instead of
// repeating it inline. Every write path in this package hand-rolled that
// shape before this type existed - see api/jobs.go's history for the bug
// that caused (a hook firing under the wrong entity name because the string
// literal at one call site didn't match its neighbours).
type entity struct {
	hooks *services.Hooks
	name  string
}

// entityFor builds the entity handle write paths against name use. Called
// once per fixed entity in NewRouter (s.apps/s.jobs/s.resources); custom
// resource handlers call it per request since their entity name is the
// resource type from the path.
func (s *Server) entityFor(name string) *entity {
	return &entity{hooks: s.hooks, name: name}
}

// Create runs pre_create, then fn, then - iff fn succeeds - post_create
// keyed off the created document's own _id.
func (e *entity) Create(
	ctx context.Context,
	data map[string]any,
	fn func(context.Context, bson.M) (bson.M, error),
) (bson.M, error) {
	data = e.hooks.PreCreate(ctx, e.name, data)

	created, err := fn(ctx, bson.M(data))
	if err != nil {
		return nil, err
	}
	e.hooks.PostCreate(e.name, db.ExtractID(created))
	return created, nil
}

// Update injects data["_id"] = id - so a pre_update hook can see which
// document is being written, the same as every hand-rolled update path did
// - then runs pre_update, fn, and (iff fn succeeds) post_update.
func (e *entity) Update(
	ctx context.Context,
	id string,
	data map[string]any,
	fn func(context.Context, string, bson.M) (bson.M, error),
) (bson.M, error) {
	data["_id"] = id
	return e.update(ctx, id, data, fn)
}

// UpdateFound is Update for a document located by a lookup rather than a
// path id (upsertByName's update-by-name branch). It does NOT inject _id:
// the pre-hook is not told the id, matching Python's perform_update, which
// passes the id as a separate argument alongside the untouched payload
// rather than folding it in. Used only by upsertByName.
func (e *entity) UpdateFound(
	ctx context.Context,
	id string,
	data map[string]any,
	fn func(context.Context, string, bson.M) (bson.M, error),
) (bson.M, error) {
	return e.update(ctx, id, data, fn)
}

// update is the shared body of Update and UpdateFound: pre_update -> fn ->
// post_update iff fn succeeds. The two exported methods differ only in
// whether _id was injected into data before this runs.
func (e *entity) update(
	ctx context.Context,
	id string,
	data map[string]any,
	fn func(context.Context, string, bson.M) (bson.M, error),
) (bson.M, error) {
	data = e.hooks.PreUpdate(ctx, e.name, data)

	updated, err := fn(ctx, id, bson.M(data))
	if err != nil {
		return nil, err
	}
	e.hooks.PostUpdate(e.name, db.ExtractID(updated))
	return updated, nil
}

// Delete runs fn, then - iff fn succeeds - post_delete keyed off id, the
// PATH id rather than whatever the deleted document's own _id was (already
// uniform across all five delete paths before this refactor).
//
// There is deliberately no pre_delete here: hook_service.py maps the
// "delete" action to (None, process_post_delete), so a pre_delete stage
// never fires in the Python original either. If that ever changes, this is
// where the call would go, ahead of fn.
func (e *entity) Delete(
	ctx context.Context,
	id string,
	fn func(context.Context, string) (bson.M, error),
) (bson.M, error) {
	deleted, err := fn(ctx, id)
	if err != nil {
		return nil, err
	}
	e.hooks.PostDelete(e.name, id)
	return deleted, nil
}
