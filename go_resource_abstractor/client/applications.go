package client

import "context"

// AppsService groups methods for the /api/v1/applications endpoints,
// mirroring libraries/resource_abstractor_client/resource_abstractor_client/app_operations.py.
type AppsService struct {
	c *Client
}

// AppFilter narrows AppsService.List. The functions below are the filters the
// spec defines; the type is exported so a caller can hold a slice of them,
// not so they can write their own. See the naming rule documented on
// ResourceFilter for why some of them name "app" and some don't.
type AppFilter func(*ListApplicationsParams)

// OfUser limits the result to applications owned by userID.
func OfUser(userID string) AppFilter {
	return func(p *ListApplicationsParams) { p.UserId = &userID }
}

// NamedApp limits the result to applications with this application_name.
// Names are unique per namespace, not globally - pair it with InNamespace to
// identify one application.
func NamedApp(name string) AppFilter {
	return func(p *ListApplicationsParams) { p.ApplicationName = &name }
}

// InNamespace limits the result to applications in this
// application_namespace.
func InNamespace(namespace string) AppFilter {
	return func(p *ListApplicationsParams) { p.ApplicationNamespace = &namespace }
}

// List returns applications matching every filter given, or all of them when
// given none. Equivalent to Python's get_apps(**kwargs).
func (a *AppsService) List(ctx context.Context, filters ...AppFilter) ([]Application, error) {
	return list[Application](a.c.api.ListApplications(ctx, appParams(filters)))
}

// ListByUser returns applications owned by userID, additionally matching
// filters. Equivalent to Python's get_user_apps.
func (a *AppsService) ListByUser(ctx context.Context, userID string, filters ...AppFilter) ([]Application, error) {
	// Set UserId on the built query rather than appending OfUser to filters,
	// which would write into the caller's slice whenever it has the spare
	// capacity for one more element.
	params := appParams(filters)
	params.UserId = &userID
	return list[Application](a.c.api.ListApplications(ctx, params))
}

// GetByNameAndNamespace returns the application identified by name and
// namespace for userID, or ErrNotFound if none matches. Equivalent to
// Python's get_app_by_name_and_namespace.
func (a *AppsService) GetByNameAndNamespace(ctx context.Context, name, namespace, userID string) (*Application, error) {
	params := appParams([]AppFilter{NamedApp(name), InNamespace(namespace), OfUser(userID)})
	return firstOf[Application](a.c.api.ListApplications(ctx, params))
}

// GetByID returns the application with the given id, scoped to userID.
// Equivalent to Python's get_app_by_id.
func (a *AppsService) GetByID(ctx context.Context, appID, userID string) (*Application, error) {
	params := &GetApplicationParams{UserId: &userID}
	return doc[Application](a.c.api.GetApplication(ctx, appID, params))
}

// Create creates a new application owned by userID. app's UserId field is
// set to userID before sending, matching Python's create_app. Returns the
// created document (with its assigned applicationID).
func (a *AppsService) Create(ctx context.Context, userID string, app Application) (*Application, error) {
	// app is a copy already (it is passed by value), so overwriting UserId
	// here cannot be seen by the caller. Its AdditionalProperties map is
	// shared with the caller's value, but nothing below writes to it.
	app.UserId = &userID
	return doc[Application](a.c.api.CreateApplication(ctx, app))
}

// Update patches the application identified by appID. app's UserId field is
// set to userID before sending, matching Python's update_app.
func (a *AppsService) Update(ctx context.Context, appID, userID string, app Application) (*Application, error) {
	app.UserId = &userID
	return doc[Application](a.c.api.PatchApplication(ctx, appID, app))
}

// Delete removes the application identified by appID. The service answers
// with the application as it was immediately before deletion; this discards
// it, as the Python client does.
func (a *AppsService) Delete(ctx context.Context, appID string) error {
	return done(a.c.api.DeleteApplication(ctx, appID))
}

// appParams collapses filters into the query the generated client takes.
func appParams(filters []AppFilter) *ListApplicationsParams {
	var params ListApplicationsParams
	for _, filter := range filters {
		filter(&params)
	}
	return &params
}
