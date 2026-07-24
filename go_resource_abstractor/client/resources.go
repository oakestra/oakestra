package client

import (
	"context"
	"net/http"
)

// ResourcesClient groups methods for the /api/v1/resources endpoints
// (candidates - clusters at the root level, workers at the cluster level),
// mirroring libraries/resource_abstractor_client/resource_abstractor_client/candidate_operations.py.
type ResourcesClient struct {
	c *Client
}

// List returns candidates matching filter (query parameters passed
// through as-is, e.g. "active": "true", "resources": "cpu_percent,memory_percent"
// for field projection). Equivalent to Python's get_candidates(**kwargs).
func (r *ResourcesClient) List(ctx context.Context, filter map[string]string) ([]Document, error) {
	return r.c.doList(ctx, http.MethodGet, "/resources", filter)
}

// GetByID returns the candidate with the given id, or ErrNotFound if none
// exists. Equivalent to Python's get_candidate_by_id.
func (r *ResourcesClient) GetByID(ctx context.Context, id string) (Document, error) {
	return r.c.doDoc(ctx, http.MethodGet, "/resources/"+id, nil, nil)
}

// GetByName returns the candidate with the given candidate_name, or
// ErrNotFound if none matches. Equivalent to Python's
// get_candidate_by_name - note the Go resource abstractor's list filter
// matches on "candidate_name" (see go_resource_abstractor/db/candidates.go),
// unlike the Python service which used "cluster_name" for the same lookup.
func (r *ResourcesClient) GetByName(ctx context.Context, name string) (Document, error) {
	query := map[string]string{"candidate_name": name}
	return r.c.doFirst(ctx, http.MethodGet, "/resources", query)
}

// GetByIP returns the candidate with the given ip, or ErrNotFound if none
// matches. Equivalent to Python's get_candidate_by_ip.
func (r *ResourcesClient) GetByIP(ctx context.Context, ip string) (Document, error) {
	query := map[string]string{"ip": ip}
	return r.c.doFirst(ctx, http.MethodGet, "/resources", query)
}

// UpdateInformation persists an aggregated resource-usage report for the
// candidate identified by id (cpu/memory history is appended server-side).
// Equivalent to Python's update_candidate_information.
func (r *ResourcesClient) UpdateInformation(ctx context.Context, id string, data Document) (Document, error) {
	return r.c.doDoc(ctx, http.MethodPatch, "/resources/"+id, nil, data)
}

// Create creates or upserts (by candidate_name) a candidate. Equivalent to
// Python's create_candidate - note the resource abstractor treats this as
// a PUT, not a POST.
func (r *ResourcesClient) Create(ctx context.Context, data Document) (Document, error) {
	return r.c.doDoc(ctx, http.MethodPut, "/resources", nil, data)
}
