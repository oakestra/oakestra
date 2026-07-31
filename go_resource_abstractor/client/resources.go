package client

import "context"

// ResourcesService groups methods for the /api/v1/resources endpoints
// (candidates - clusters at the root level, workers at the cluster level),
// mirroring libraries/resource_abstractor_client/resource_abstractor_client/candidate_operations.py.
type ResourcesService struct {
	c *Client
}

// ResourceFilter narrows ResourcesService.List. The functions below are the
// filters the spec defines; the type is exported so a caller can hold a slice
// of them, not so they can write their own.
//
// Naming rule: a filter constructor names its resource when the same
// concept could plausibly filter more than one of applications, resources or
// jobs (hence ResourceFields, NamedCandidate, NamedApp, NamedJob), and omits
// it when the concept exists on only one of them (hence Active, InNamespace,
// CandidateIP, RunningJob, OfUser, OfApplication).
type ResourceFilter func(*ListResourcesParams)

// Active limits the result to candidates that reported within the freshness
// window (30 seconds).
//
// It takes no argument on purpose: the service only applies the freshness
// filter when ?active= is truthy (BuildCandidateFilter in
// go_resource_abstractor/db/candidates.go), so `active=false` isn't "only
// stale candidates" - it's no filter at all, same as leaving Active off. An
// Active(bool) would imply an inverse the service doesn't have.
func Active() ResourceFilter {
	active := "true"
	return func(p *ListResourcesParams) { p.Active = &active }
}

// NamedCandidate limits the result to the candidate with this candidate_name.
func NamedCandidate(name string) ResourceFilter {
	return func(p *ListResourcesParams) { p.CandidateName = &name }
}

// CandidateIP limits the result to the candidate reachable at this address.
func CandidateIP(ip string) ResourceFilter {
	return func(p *ListResourcesParams) { p.Ip = &ip }
}

// RunningJob limits the result to the candidate the given job is placed on.
// An unknown job, or one not yet placed, is ErrNotFound.
func RunningJob(jobID string) ResourceFilter {
	return func(p *ListResourcesParams) { p.JobId = &jobID }
}

// ResourceFields extends the canonical projection with further document
// fields, for reading data the service stores but doesn't return by
// default:
//
//	c.Resources.List(ctx, client.ResourceFields("gpu_temp", "gpu_drivers"))
//
// The spec sends them as one comma-separated value; pass them as separate
// arguments and let the generated encoder join them.
func ResourceFields(names ...string) ResourceFilter {
	return func(p *ListResourcesParams) { p.Resources = &names }
}

// List returns candidates matching every filter given, or all of them when
// given none. Equivalent to Python's get_candidates(**kwargs).
func (r *ResourcesService) List(ctx context.Context, filters ...ResourceFilter) ([]Resource, error) {
	return list[Resource](r.c.api.ListResources(ctx, resourceParams(filters)))
}

// GetByID returns the candidate with the given id, or ErrNotFound if none
// exists. Equivalent to Python's get_candidate_by_id.
func (r *ResourcesService) GetByID(ctx context.Context, id string) (*Resource, error) {
	return doc[Resource](r.c.api.GetResource(ctx, id))
}

// GetByName returns the candidate with the given candidate_name, or
// ErrNotFound if none matches. Equivalent to Python's
// get_candidate_by_name - note the Go resource abstractor's list filter
// matches on "candidate_name" (see go_resource_abstractor/db/candidates.go),
// unlike the Python service which used "cluster_name" for the same lookup.
func (r *ResourcesService) GetByName(ctx context.Context, name string) (*Resource, error) {
	params := resourceParams([]ResourceFilter{NamedCandidate(name)})
	return firstOf[Resource](r.c.api.ListResources(ctx, params))
}

// GetByIP returns the candidate with the given ip, or ErrNotFound if none
// matches. Equivalent to Python's get_candidate_by_ip.
func (r *ResourcesService) GetByIP(ctx context.Context, ip string) (*Resource, error) {
	params := resourceParams([]ResourceFilter{CandidateIP(ip)})
	return firstOf[Resource](r.c.api.ListResources(ctx, params))
}

// UpdateInformation persists an aggregated resource-usage report for the
// candidate identified by id (cpu/memory history is appended server-side).
// Equivalent to Python's update_candidate_information.
func (r *ResourcesService) UpdateInformation(ctx context.Context, id string, resource Resource) (*Resource, error) {
	return doc[Resource](r.c.api.PatchResource(ctx, id, resource))
}

// Create creates or upserts (by candidate_name) a candidate. Equivalent to
// Python's create_candidate - note the resource abstractor treats this as
// a PUT, not a POST.
func (r *ResourcesService) Create(ctx context.Context, resource Resource) (*Resource, error) {
	return doc[Resource](r.c.api.UpsertResource(ctx, resource))
}

// resourceParams collapses filters into the query the generated client takes.
func resourceParams(filters []ResourceFilter) *ListResourcesParams {
	var params ListResourcesParams
	for _, filter := range filters {
		filter(&params)
	}
	return &params
}
