package client

import "context"

// JobsService groups methods for the /api/v1/jobs endpoints, including the
// instance sub-routes, mirroring
// libraries/resource_abstractor_client/resource_abstractor_client/job_operations.py.
type JobsService struct {
	c *Client
}

// JobFilter narrows JobsService.List. OfApplication and NamedJob below are the
// filters the spec defines; the type is exported so a caller can hold a slice
// of them, not so they can write their own. See the naming rule documented on
// ResourceFilter for why some of them name "job" and some don't.
type JobFilter func(*ListJobsParams)

// OfApplication limits the result to jobs belonging to appID.
func OfApplication(appID string) JobFilter {
	return func(p *ListJobsParams) { p.ApplicationID = &appID }
}

// NamedJob limits the result to the job with this job_name, which is unique
// per deployment.
func NamedJob(name string) JobFilter {
	return func(p *ListJobsParams) { p.JobName = &name }
}

// List returns jobs matching every filter given, or all of them when given
// none. Equivalent to Python's get_jobs(**kwargs).
func (j *JobsService) List(ctx context.Context, filters ...JobFilter) ([]Job, error) {
	return list[Job](j.c.api.ListJobs(ctx, collapseParams(filters)))
}

// ListByApplication returns the jobs belonging to appID. Equivalent to
// Python's get_jobs_of_application.
func (j *JobsService) ListByApplication(ctx context.Context, appID string) ([]Job, error) {
	return j.List(ctx, OfApplication(appID))
}

// GetByID returns the job with the given id, or ErrNotFound if none
// exists. Equivalent to Python's get_job_by_id.
func (j *JobsService) GetByID(ctx context.Context, jobID string) (*Job, error) {
	return doc[Job](j.c.api.GetJob(ctx, jobID, nil))
}

// GetInstance returns job jobID with its instance_list filtered down to
// instanceNumber. Equivalent to Python's get_job_instance.
func (j *JobsService) GetInstance(ctx context.Context, jobID string, instanceNumber int) (*Job, error) {
	return doc[Job](j.c.api.GetJobInstance(ctx, jobID, instanceNumber))
}

// AppendInstance adds a new instance (instanceNumber, instance) to job
// jobID. Equivalent to Python's append_job_instance. Returns *APIError with
// Status 400 if the instance already exists (the resource abstractor rejects
// duplicate instance numbers).
func (j *JobsService) AppendInstance(ctx context.Context, jobID string, instanceNumber int, instance JobInstanceAppend) (*Job, error) {
	return doc[Job](j.c.api.AppendJobInstance(ctx, jobID, instanceNumber, instance))
}

// Create creates or upserts (by job_name) a job. Equivalent to Python's
// create_job - note the resource abstractor treats this as a PUT, not a
// POST.
func (j *JobsService) Create(ctx context.Context, job Job) (*Job, error) {
	return doc[Job](j.c.api.UpsertJob(ctx, job))
}

// Update patches the job identified by jobID. Equivalent to Python's
// update_job.
func (j *JobsService) Update(ctx context.Context, jobID string, job Job) (*Job, error) {
	return doc[Job](j.c.api.PatchJob(ctx, jobID, job))
}

// UpdateStatus patches the job identified by jobID with a new status, and
// statusDetail when non-empty. Equivalent to Python's update_job_status.
//
// Python's version takes an oakestra_utils Status enum and sends its
// .value; there's no equivalent shared enum in Go, so status is a plain
// string here - use the same values the rest of the platform does (see
// libraries/oakestra_utils_library's status enums).
//
// The fields go through Set rather than struct fields because openapi.yaml
// doesn't declare status/status_detail on Job - the service just stores
// whatever additionalProperties gives it.
func (j *JobsService) UpdateStatus(ctx context.Context, jobID, status, statusDetail string) (*Job, error) {
	var patch Job
	patch.Set("status", status)
	if statusDetail != "" {
		patch.Set("status_detail", statusDetail)
	}
	return j.Update(ctx, jobID, patch)
}

// UpdateInstance patches instance instanceNumber of job jobID with a
// status report. Equivalent to Python's update_job_instance.
func (j *JobsService) UpdateInstance(ctx context.Context, jobID string, instanceNumber int, instance JobInstance) (*Job, error) {
	return doc[Job](j.c.api.PatchJobInstance(ctx, jobID, instanceNumber, instance))
}

// DeleteInstance removes instance instanceNumber from job jobID,
// returning the updated job document. Equivalent to Python's
// delete_job_instance.
func (j *JobsService) DeleteInstance(ctx context.Context, jobID string, instanceNumber int) (*Job, error) {
	return doc[Job](j.c.api.DeleteJobInstance(ctx, jobID, instanceNumber))
}

// Delete removes the job identified by jobID. Equivalent to Python's
// delete_job.
func (j *JobsService) Delete(ctx context.Context, jobID string) error {
	return done(j.c.api.DeleteJob(ctx, jobID))
}
