package abstractor

import (
	"context"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/hooks"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

// entityHooks bundles a hook channel name ("jobs", "applications",
// "resources", or a custom resource type) with the dispatcher, so a write
// path names its entity once and create/update/del handle the
// pre-event/store-call/post-event sequence around it. It's a plain
// two-field value with no need for pointer identity, so it's passed around
// by value.
type entityHooks struct {
	dispatcher *hooks.Hooks
	name       string
}

// create runs pre_create, hands the (possibly hook-transformed) payload to
// fn as a typed value, and fires post_create keyed off the created
// document's own _id if fn succeeds.
func create[T any](ctx context.Context, e entityHooks, data T, fn func(context.Context, T) (T, error)) (T, error) {
	var zero T

	// With no pre_create hook registered - the normal case - data goes to
	// the store as-is, skipping the model->map->model round trip entirely.
	if urls := e.dispatcher.SyncURLs(ctx, e.name, model.EventPreCreate); len(urls) > 0 {
		payload, err := toMap(data)
		if err != nil {
			return zero, err
		}
		data, err = fromMap[T](e.dispatcher.RunSync(ctx, urls, payload))
		if err != nil {
			return zero, err
		}
	}

	created, err := fn(ctx, data)
	if err != nil {
		return zero, err
	}

	createdMap, err := toMap(created)
	if err != nil {
		return zero, err
	}
	e.dispatcher.PostCreate(e.name, idFromMap(createdMap))
	return created, nil
}

// update runs pre_update/post_update around fn. withID controls whether the
// pre_update hook is told which document is being written: a
// path-addressed update (Apps.Update, Jobs.Update, ...) sets data["_id"] = id
// before the hook fires, while the update-by-name upsert branch
// (Jobs.Upsert, Resources.Upsert) leaves it out.
//
// In and Out differ for Jobs.UpdateInstance (JobInstance in, Job out).
func update[In, Out any](
	ctx context.Context,
	e entityHooks,
	id string,
	data In,
	withID bool,
	fn func(context.Context, In) (Out, error),
) (Out, error) {
	var zero Out

	// Same short-circuit as create, and it matters more here: a worker's
	// instance heartbeat is an update, so this runs on every report.
	if urls := e.dispatcher.SyncURLs(ctx, e.name, model.EventPreUpdate); len(urls) > 0 {
		payload, err := toMap(data)
		if err != nil {
			return zero, err
		}
		if withID {
			payload["_id"] = id
		}
		data, err = fromMap[In](e.dispatcher.RunSync(ctx, urls, payload))
		if err != nil {
			return zero, err
		}
	}

	updated, err := fn(ctx, data)
	if err != nil {
		return zero, err
	}

	// Every update is addressed by id, so post_update can use it directly
	// instead of serializing the (possibly history-laden) result to read _id.
	e.dispatcher.PostUpdate(e.name, id)
	return updated, nil
}

// del calls fn and fires post_delete if it succeeded. The event uses the
// path id, not the deleted document's _id: a custom resource delete on an
// already-missing instance has no document to read one from.
// There is no pre_delete hook. If one is added, it goes before fn.
func del[T any](ctx context.Context, e entityHooks, id string, fn func(context.Context) (T, error)) (T, error) {
	var zero T

	deleted, err := fn(ctx)
	if err != nil {
		return zero, err
	}
	e.dispatcher.PostDelete(e.name, id)
	return deleted, nil
}
