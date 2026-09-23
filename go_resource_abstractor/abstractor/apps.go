package abstractor

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/store"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

// ApplicationFilter narrows List/Get to applications matching the given
// fields; a zero field imposes no constraint. Mirrors the three query
// parameters ListApplicationsParams/GetApplicationParams declare in
// openapi.yaml.
type ApplicationFilter struct {
	Name      string
	Namespace string
	UserID    string
}

func (f ApplicationFilter) toBSON() bson.M {
	filter := bson.M{}
	if f.Name != "" {
		filter["application_name"] = f.Name
	}
	if f.Namespace != "" {
		filter["application_namespace"] = f.Namespace
	}
	if f.UserID != "" {
		filter["userId"] = f.UserID
	}
	return filter
}

// Apps is the application sub-service: create/read/update/delete for
// applications, with pre/post webhooks fired under the "applications"
// entity around every write.
type Apps struct {
	store  *store.Store
	entity entityHooks
}

// List returns applications matching filter.
func (a *Apps) List(ctx context.Context, filter ApplicationFilter) ([]model.Application, error) {
	return a.store.FindApps(ctx, filter.toBSON())
}

// Get returns a single application by id, additionally constrained by
// filter.
func (a *Apps) Get(ctx context.Context, id string, filter ApplicationFilter) (model.Application, error) {
	return a.store.FindAppByID(ctx, id, filter.toBSON())
}

// Create runs pre_create/post_create around inserting a new application.
// The store always overwrites ApplicationID with the new document's own
// stringified _id, regardless of what data.ApplicationID was.
func (a *Apps) Create(ctx context.Context, data model.Application) (model.Application, error) {
	return create(ctx, a.entity, data, a.store.CreateApp)
}

// Update runs pre_update/post_update around a $set patch addressed by id.
// The pre_update hook sees data["_id"] = id, so it knows which document is
// being written.
func (a *Apps) Update(ctx context.Context, id string, patch model.Application) (model.Application, error) {
	return update(ctx, a.entity, id, patch, true, func(ctx context.Context, d model.Application) (model.Application, error) {
		return a.store.UpdateApp(ctx, id, d)
	})
}

// Delete runs post_delete around removing an application by id, and returns
// the deleted document.
func (a *Apps) Delete(ctx context.Context, id string) (model.Application, error) {
	return del(ctx, a.entity, id, func(ctx context.Context) (model.Application, error) {
		return a.store.DeleteApp(ctx, id)
	})
}
