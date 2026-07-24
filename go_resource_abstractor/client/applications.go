package client

import (
	"context"
	"net/http"
)

// AppsClient groups methods for the /api/v1/applications endpoints,
// mirroring libraries/resource_abstractor_client/resource_abstractor_client/app_operations.py.
type AppsClient struct {
	c *Client
}

// List returns applications matching filter (query parameters passed
// through as-is), equivalent to Python's get_apps(**kwargs).
func (a *AppsClient) List(ctx context.Context, filter map[string]string) ([]Document, error) {
	return a.c.doList(ctx, http.MethodGet, "/applications", filter)
}

// ListByUser returns applications owned by userID, additionally matching
// filter. Equivalent to Python's get_user_apps.
func (a *AppsClient) ListByUser(ctx context.Context, userID string, filter map[string]string) ([]Document, error) {
	query := withKey(filter, "userId", userID)
	return a.c.doList(ctx, http.MethodGet, "/applications", query)
}

// GetByNameAndNamespace returns the application identified by name and
// namespace for userID, or ErrNotFound if none matches. Equivalent to
// Python's get_app_by_name_and_namespace.
func (a *AppsClient) GetByNameAndNamespace(ctx context.Context, name, namespace, userID string) (Document, error) {
	query := map[string]string{
		"userId":                userID,
		"application_name":      name,
		"application_namespace": namespace,
	}
	return a.c.doFirst(ctx, http.MethodGet, "/applications", query)
}

// GetByID returns the application with the given id, scoped to userID.
// Equivalent to Python's get_app_by_id.
func (a *AppsClient) GetByID(ctx context.Context, appID, userID string) (Document, error) {
	query := map[string]string{"userId": userID}
	return a.c.doDoc(ctx, http.MethodGet, "/applications/"+appID, query, nil)
}

// Create creates a new application owned by userID. data's "userId" field
// is set/overwritten with userID before sending, matching Python's
// create_app. Returns the created document (with its assigned
// applicationID).
func (a *AppsClient) Create(ctx context.Context, userID string, data Document) (Document, error) {
	return a.c.doDoc(ctx, http.MethodPost, "/applications", nil, withUserID(data, userID))
}

// Update patches the application identified by appID. data's "userId"
// field is set/overwritten with userID before sending, matching Python's
// update_app.
func (a *AppsClient) Update(ctx context.Context, appID, userID string, data Document) (Document, error) {
	return a.c.doDoc(ctx, http.MethodPatch, "/applications/"+appID, nil, withUserID(data, userID))
}

// Delete removes the application identified by appID.
func (a *AppsClient) Delete(ctx context.Context, appID string) error {
	return a.c.do(ctx, http.MethodDelete, "/applications/"+appID, nil, nil, nil)
}

// withUserID returns a shallow copy of data with "userId" set to userID,
// leaving the caller's map untouched.
func withUserID(data Document, userID string) Document {
	return withKey[Document, any](data, "userId", userID)
}
