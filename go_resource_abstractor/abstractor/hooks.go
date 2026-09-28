package abstractor

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/store"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

// Hooks is the webhook-registration sub-service: create/read/update/delete
// for hook registrations themselves. It's distinct from internal/hooks.Hooks,
// the dispatcher that fires pre/post events around writes to the other
// entities. Registering, listing or changing a hook doesn't itself fire any
// pre/post event.
type Hooks struct {
	store *store.Store
}

// List returns every registered hook, exactly as stored. A hook document
// isn't decoded into model.Hook here: a hand-edited registration with a
// wrongly typed field (e.g. a numeric webhook_url) must still show up in
// the list instead of taking the whole response down with it.
func (h *Hooks) List(ctx context.Context) ([]bson.M, error) {
	return h.store.FindHooks(ctx, nil)
}

// Get returns a single hook by id, exactly as stored (see List).
func (h *Hooks) Get(ctx context.Context, id string) (bson.M, error) {
	return h.store.FindHookByID(ctx, id)
}

// Create registers a new hook, after checking every entry of data.Events
// against the model.HookEvent enum.
func (h *Hooks) Create(ctx context.Context, data model.Hook) (model.Hook, error) {
	if err := validateHookEvents(data.Events); err != nil {
		return model.Hook{}, err
	}
	return h.store.CreateHook(ctx, data)
}

// Update applies a $set patch to a hook registration, after the same event
// validation as Create, and returns the document as it stands afterwards
// (see List for why that isn't a model.Hook).
func (h *Hooks) Update(ctx context.Context, id string, patch model.Hook) (bson.M, error) {
	if err := validateHookEvents(patch.Events); err != nil {
		return nil, err
	}
	return h.store.UpdateHook(ctx, id, patch)
}

// Delete removes a hook registration by id.
func (h *Hooks) Delete(ctx context.Context, id string) error {
	return h.store.DeleteHook(ctx, id)
}

// validateHookEvents checks that every entry of events is a member of the
// model.HookEvent enum. A nil or empty events is valid - there's nothing to
// check.
func validateHookEvents(events *[]model.HookEvent) error {
	if events == nil {
		return nil
	}
	for _, e := range *events {
		if !e.Valid() {
			return fmt.Errorf("%w: %q", ErrInvalidHookEvent, e)
		}
	}
	return nil
}
