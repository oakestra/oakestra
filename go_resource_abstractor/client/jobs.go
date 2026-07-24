package client

import (
	"context"
	"net/http"
	"strconv"
)

// JobsClient groups methods for the /api/v1/jobs endpoints, including the
// instance sub-routes, mirroring
// libraries/resource_abstractor_client/resource_abstractor_client/job_operations.py.
type JobsClient struct {
	c *Client
}

// List returns jobs matching filter (query parameters passed through
// as-is, e.g. "job_name", "applicationID"). Equivalent to Python's
// get_jobs(**kwargs).
func (j *JobsClient) List(ctx context.Context, filter map[string]string) ([]Document, error) {
	return j.c.doList(ctx, http.MethodGet, "/jobs", filter)
}

// ListByApplication returns the jobs belonging to appID. Equivalent to
// Python's get_jobs_of_application.
func (j *JobsClient) ListByApplication(ctx context.Context, appID string) ([]Document, error) {
	return j.List(ctx, map[string]string{"applicationID": appID})
}

// GetByID returns the job with the given id, or ErrNotFound if none
// exists. Equivalent to Python's get_job_by_id.
func (j *JobsClient) GetByID(ctx context.Context, jobID string) (Document, error) {
	return j.c.doDoc(ctx, http.MethodGet, "/jobs/"+jobID, nil, nil)
}

// GetInstance returns job jobID with its instance_list filtered down to
// instanceNumber. Equivalent to Python's get_job_instance.
func (j *JobsClient) GetInstance(ctx context.Context, jobID string, instanceNumber int) (Document, error) {
	return j.c.doDoc(ctx, http.MethodGet, instancePath(jobID, instanceNumber), nil, nil)
}

// AppendInstance adds a new instance (instanceNumber, instanceData) to
// job jobID. Equivalent to Python's append_job_instance. Returns
// *APIError with Status 400 if the instance already exists (the resource
// abstractor rejects duplicate instance numbers).
func (j *JobsClient) AppendInstance(ctx context.Context, jobID string, instanceNumber int, instanceData Document) (Document, error) {
	return j.c.doDoc(ctx, http.MethodPut, instancePath(jobID, instanceNumber), nil, instanceData)
}

// Create creates or upserts (by job_name) a job. Equivalent to Python's
// create_job - note the resource abstractor treats this as a PUT, not a
// POST.
func (j *JobsClient) Create(ctx context.Context, data Document) (Document, error) {
	return j.c.doDoc(ctx, http.MethodPut, "/jobs", nil, data)
}

// Update patches the job identified by jobID. Equivalent to Python's
// update_job.
func (j *JobsClient) Update(ctx context.Context, jobID string, data Document) (Document, error) {
	return j.c.doDoc(ctx, http.MethodPatch, "/jobs/"+jobID, nil, data)
}

// UpdateStatus patches the job identified by jobID with a new status, and
// statusDetail when non-empty. Equivalent to Python's update_job_status.
//
// Python's version takes an oakestra_utils Status enum and sends its
// .value; there is no equivalent shared enum in Go, so status is a plain
// string here - pass the same status values the rest of the platform uses
// (see libraries/oakestra_utils_library's status enums for the accepted
// set).
func (j *JobsClient) UpdateStatus(ctx context.Context, jobID, status, statusDetail string) (Document, error) {
	data := Document{"status": status}
	if statusDetail != "" {
		data["status_detail"] = statusDetail
	}
	return j.Update(ctx, jobID, data)
}

// UpdateInstance patches instance instanceNumber of job jobID with a
// status report. Equivalent to Python's update_job_instance.
func (j *JobsClient) UpdateInstance(ctx context.Context, jobID string, instanceNumber int, data Document) (Document, error) {
	return j.c.doDoc(ctx, http.MethodPatch, instancePath(jobID, instanceNumber), nil, data)
}

// DeleteInstance removes instance instanceNumber from job jobID,
// returning the updated job document. Equivalent to Python's
// delete_job_instance.
func (j *JobsClient) DeleteInstance(ctx context.Context, jobID string, instanceNumber int) (Document, error) {
	return j.c.doDoc(ctx, http.MethodDelete, instancePath(jobID, instanceNumber), nil, nil)
}

// Delete removes the job identified by jobID. Equivalent to Python's
// delete_job.
func (j *JobsClient) Delete(ctx context.Context, jobID string) error {
	return j.c.do(ctx, http.MethodDelete, "/jobs/"+jobID, nil, nil, nil)
}

// instancePath builds the path for job jobID's instanceNumber sub-route,
// shared by every instance-level method above.
func instancePath(jobID string, instanceNumber int) string {
	return "/jobs/" + jobID + "/" + strconv.Itoa(instanceNumber)
}
