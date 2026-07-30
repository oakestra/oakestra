package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"

	"go_resource_abstractor/db"
	"go_resource_abstractor/openapi"
)

// ListJobs implements GET /api/v1/jobs.
//
// The Python service also honored a raw ?params= value by assigning it as
// the entire Mongo filter, but that value is always a string and pymongo
// rejects a string filter, so the branch only ever produced a 500. It
// carried no usable behavior and is neither reproduced here nor declared in
// the spec.
func (s *Server) ListJobs(c *gin.Context, params openapi.ListJobsParams) {
	filter := map[string]any{}
	addFilter(filter, "applicationID", params.ApplicationID)
	addFilter(filter, "job_name", params.JobName)

	results, err := s.store.FindJobs(c.Request.Context(), bson.M(filter))
	if err != nil {
		abortInternalError(c, err)
		return
	}

	writeJSON(c, http.StatusOK, results)
}

// CreateJob implements POST /api/v1/jobs.
func (s *Server) CreateJob(c *gin.Context) {
	data, ok := bindJSONMap(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	data = s.hooks.PreCreate(ctx, "jobs", data)

	created, err := s.store.CreateJob(ctx, bson.M(data))
	if err != nil {
		abortInternalError(c, err)
		return
	}
	s.hooks.PostCreate("jobs", db.ExtractID(created))

	writeJSON(c, http.StatusOK, created)
}

// UpsertJob implements PUT /api/v1/jobs: update-by-job_name if a match
// exists, else create.
//
// Deviation from the Python service: the original fires hooks under the
// entity name "job" (singular) here, while every other job route uses
// "jobs" - so a hook registered for "jobs" would never see this path's
// events. Normalized to "jobs" throughout so one hook registration covers
// all job writes.
func (s *Server) UpsertJob(c *gin.Context) {
	data, ok := bindJSONMap(c)
	if !ok {
		return
	}
	s.upsertByName(c, "jobs", "job_name", data, s.store.FindJobByName, s.store.UpdateJob, s.store.CreateJob)
}

// GetJob implements GET /api/v1/jobs/{job_id}, optionally narrowed to jobs
// that have a given instance_number.
func (s *Server) GetJob(c *gin.Context, jobID openapi.JobID, params openapi.GetJobParams) {
	if !isValidObjectID(jobID) {
		abortBadRequest(c)
		return
	}

	query := map[string]any{}
	if n, present, err := queryInt("instance_number", params.InstanceNumber); err != nil {
		abortInvalidQuery(c, "instance_number", err.Error())
		return
	} else if present {
		query["instance_number"] = n
	}
	filter := db.BuildJobFilter(query)

	job, err := s.store.FindJobByID(c.Request.Context(), jobID, filter)
	if abortOnError(c, err) {
		return
	}

	writeJSON(c, http.StatusOK, job)
}

// PatchJob implements PATCH /api/v1/jobs/{job_id}.
func (s *Server) PatchJob(c *gin.Context, jobID openapi.JobID) {
	data, ok := bindJSONMap(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	data["_id"] = jobID
	data = s.hooks.PreUpdate(ctx, "jobs", data)

	updated, err := s.store.UpdateJob(ctx, jobID, bson.M(data))
	if abortOnError(c, err) {
		return
	}
	s.hooks.PostUpdate("jobs", db.ExtractID(updated))

	writeJSON(c, http.StatusOK, updated)
}

// DeleteJob implements DELETE /api/v1/jobs/{job_id}.
func (s *Server) DeleteJob(c *gin.Context, jobID openapi.JobID) {
	ctx := c.Request.Context()

	deleted, err := s.store.DeleteJob(ctx, jobID)
	if abortOnError(c, err) {
		return
	}
	s.hooks.PostDelete("jobs", jobID)

	writeJSON(c, http.StatusOK, deleted)
}

// GetJobInstance implements GET /api/v1/jobs/{job_id}/{instance_id}.
func (s *Server) GetJobInstance(c *gin.Context, jobID openapi.JobID, instanceNumber openapi.InstanceID) {
	if !isValidObjectID(jobID) {
		abortBadRequest(c)
		return
	}

	result, err := s.store.FindJobInstance(c.Request.Context(), jobID, instanceNumber)
	if abortOnError(c, err) {
		return
	}

	writeJSON(c, http.StatusOK, result)
}

// AppendJobInstance implements PUT /api/v1/jobs/{job_id}/{instance_id}:
// appends the last element of the request body's instance_list to the job.
func (s *Server) AppendJobInstance(c *gin.Context, jobID openapi.JobID, instanceNumber openapi.InstanceID) {
	if !isValidObjectID(jobID) {
		abortBadRequest(c)
		return
	}

	data, ok := bindJSONMap(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	data["_id"] = jobID
	data = s.hooks.PreCreate(ctx, "jobs", data)

	updated, err := s.store.AppendJobInstance(ctx, jobID, instanceNumber, bson.M(data))
	if err != nil {
		if errors.Is(err, db.ErrInstanceConflict) {
			c.AbortWithStatusJSON(http.StatusBadRequest, openapi.Message{Message: "Instance already exists"})
			return
		}
		abortInternalError(c, err)
		return
	}
	s.hooks.PostCreate("jobs", db.ExtractID(updated))

	writeJSON(c, http.StatusOK, updated)
}

// PatchJobInstance implements PATCH /api/v1/jobs/{job_id}/{instance_id}.
func (s *Server) PatchJobInstance(c *gin.Context, jobID openapi.JobID, instanceNumber openapi.InstanceID) {
	if !isValidObjectID(jobID) {
		abortBadRequest(c)
		return
	}

	data, ok := bindJSONMap(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	data["_id"] = jobID
	data = s.hooks.PreUpdate(ctx, "jobs", data)

	updated, err := s.store.UpdateJobInstance(ctx, jobID, instanceNumber, bson.M(data))
	if abortOnError(c, err) {
		return
	}
	s.hooks.PostUpdate("jobs", db.ExtractID(updated))

	writeJSON(c, http.StatusOK, updated)
}

// DeleteJobInstance implements DELETE /api/v1/jobs/{job_id}/{instance_id}.
func (s *Server) DeleteJobInstance(c *gin.Context, jobID openapi.JobID, instanceNumber openapi.InstanceID) {
	if !isValidObjectID(jobID) {
		abortBadRequest(c)
		return
	}
	ctx := c.Request.Context()

	updated, err := s.store.DeleteJobInstance(ctx, jobID, instanceNumber)
	if abortOnError(c, err) {
		return
	}
	s.hooks.PostDelete("jobs", jobID)

	writeJSON(c, http.StatusOK, updated)
}
