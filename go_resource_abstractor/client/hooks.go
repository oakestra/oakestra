package client

import "context"

// HooksService groups methods for the /api/v1/hooks endpoints. Unlike Apps,
// Resources and Jobs, it has no Python equivalent to mirror - hooks are new
// surface area, not a port.
type HooksService struct {
	c *Client
}

// List returns every registered hook. Equivalent to GET /api/v1/hooks; there
// are no query parameters to filter by, unlike Apps/Resources/Jobs.List.
func (h *HooksService) List(ctx context.Context) ([]Hook, error) {
	return list[Hook](h.c.api.ListHooks(ctx))
}

// GetByID returns the hook with the given id, or ErrNotFound if none exists.
func (h *HooksService) GetByID(ctx context.Context, id string) (*Hook, error) {
	return doc[Hook](h.c.api.GetHook(ctx, id))
}

// Create registers a new hook. Unlike Jobs.Create and Resources.Create, which
// upsert via PUT, this is a genuine POST answering 201 - calling it twice
// with the same Hook registers two hooks, not one.
func (h *HooksService) Create(ctx context.Context, hook Hook) (*Hook, error) {
	return doc[Hook](h.c.api.CreateHook(ctx, hook))
}

// Update patches the hook identified by id.
func (h *HooksService) Update(ctx context.Context, id string, hook Hook) (*Hook, error) {
	return doc[Hook](h.c.api.PatchHook(ctx, id, hook))
}

// Delete removes the hook identified by id. DeleteHook answers 204 whether or
// not id matched a hook, so unlike other Delete/GetByID methods here, this
// doesn't report ErrNotFound for an unknown id - the spec gives it nothing to
// tell that apart from a successful delete.
func (h *HooksService) Delete(ctx context.Context, id string) error {
	return Done(h.c.api.DeleteHook(ctx, id))
}
