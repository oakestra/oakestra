package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"

	"go_resource_abstractor/db"
	"go_resource_abstractor/openapi"
)

// ListJobs implements GET /api/v1/jobs.
//
// Python also honored a raw ?params= value as the entire Mongo filter, but
// pymongo rejects a string filter, so that branch only ever 500'd. No
// usable behavior to reproduce, so it's dropped here and from the spec.
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

	created, err := s.jobs.Create(c.Request.Context(), data, s.store.CreateJob)
	if err != nil {
		abortInternalError(c, err)
		return
	}

	writeJSON(c, http.StatusOK, created)
}

// UpsertJob implements PUT /api/v1/jobs: update-by-job_name if a match
// exists, else create.
//
// Deviation from Python, which fires hooks under "job" (singular) here
// while every other job route uses "jobs" - so a "jobs" hook would never
// see this path's events. Normalized to "jobs" so one registration covers
// all job writes.
func (s *Server) UpsertJob(c *gin.Context) {
	data, ok := bindJSONMap(c)
	if !ok {
		return
	}
	s.upsertByName(c, s.jobs, "job_name", data, s.store.FindJobByName, s.store.UpdateJob, s.store.CreateJob)
}

// GetJob implements GET /api/v1/jobs/{job_id}, optionally narrowed to jobs
// that have a given instance_number.
func (s *Server) GetJob(c *gin.Context, jobID openapi.JobID, params openapi.GetJobParams) {
	if !isValidObjectID(jobID) {
		abortBadRequest(c)
		return
	}

	var instanceNumber *int
	if n, present, err := queryInt("instance_number", params.InstanceNumber); err != nil {
		abortInvalidQuery(c, "instance_number", err.Error())
		return
	} else if present {
		instanceNumber = &n
	}
	filter := db.BuildJobFilter(instanceNumber)

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

	updated, err := s.jobs.Update(c.Request.Context(), jobID, data, s.store.UpdateJob)
	if abortOnError(c, err) {
		return
	}

	writeJSON(c, http.StatusOK, updated)
}

// DeleteJob implements DELETE /api/v1/jobs/{job_id}.
func (s *Server) DeleteJob(c *gin.Context, jobID openapi.JobID) {
	deleted, err := s.jobs.Delete(c.Request.Context(), jobID, s.store.DeleteJob)
	if abortOnError(c, err) {
		return
	}

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

	// This is the one path that fires a *create* event while also injecting
	// _id: the pre-hook needs to see which job the instance is being
	// appended to, but there's no separate id to hand Create the way
	// Update/UpdateFound take one - so the injection stays here rather than
	// growing a fifth entity method for a single caller.
	data["_id"] = jobID

	updated, err := s.jobs.Create(ctx, data, func(ctx context.Context, data bson.M) (bson.M, error) {
		return s.store.AppendJobInstance(ctx, jobID, instanceNumber, data)
	})
	if err != nil {
		if errors.Is(err, db.ErrInstanceConflict) {
			c.AbortWithStatusJSON(http.StatusBadRequest, openapi.Message{Message: "Instance already exists"})
			return
		}
		abortInternalError(c, err)
		return
	}

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

	updated, err := s.jobs.Update(ctx, jobID, data, func(ctx context.Context, _ string, data bson.M) (bson.M, error) {
		return s.store.UpdateJobInstance(ctx, jobID, instanceNumber, data)
	})
	if abortOnError(c, err) {
		return
	}

	writeJSON(c, http.StatusOK, updated)
}

// DeleteJobInstance implements DELETE /api/v1/jobs/{job_id}/{instance_id}.
func (s *Server) DeleteJobInstance(c *gin.Context, jobID openapi.JobID, instanceNumber openapi.InstanceID) {
	if !isValidObjectID(jobID) {
		abortBadRequest(c)
		return
	}

	updated, err := s.jobs.Delete(c.Request.Context(), jobID, func(ctx context.Context, _ string) (bson.M, error) {
		return s.store.DeleteJobInstance(ctx, jobID, instanceNumber)
	})
	if abortOnError(c, err) {
		return
	}

	writeJSON(c, http.StatusOK, updated)
}
