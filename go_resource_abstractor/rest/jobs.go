package rest

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/oakestra/oakestra/go_resource_abstractor/abstractor"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
	"github.com/oakestra/oakestra/go_resource_abstractor/openapi"
)

// ListJobs implements GET /api/v1/jobs.
//
// Python also honored a raw ?params= value as the entire Mongo filter, but
// pymongo rejects a string filter, so that branch only ever 500'd. No
// usable behavior to reproduce, so it's dropped here and from the spec.
func (s *Server) ListJobs(c *gin.Context, params openapi.ListJobsParams) {
	filter := abstractor.JobFilter{
		ApplicationID: strOrEmpty(params.ApplicationID),
		JobName:       strOrEmpty(params.JobName),
	}

	results, err := s.svc.Jobs.List(c.Request.Context(), filter)
	if err != nil {
		s.abortInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, results)
}

// CreateJob implements POST /api/v1/jobs.
func (s *Server) CreateJob(c *gin.Context) {
	data, ok := bindModel[model.Job](c, bindJSONMap)
	if !ok {
		return
	}

	created, err := s.svc.Jobs.Create(c.Request.Context(), data)
	if err != nil {
		s.abortInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, created)
}

// UpsertJob implements PUT /api/v1/jobs: update-by-job_name if a match
// exists, else create.
func (s *Server) UpsertJob(c *gin.Context) {
	data, ok := bindModel[model.Job](c, bindJSONMap)
	if !ok {
		return
	}

	updated, err := s.svc.Jobs.Upsert(c.Request.Context(), data)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, updated)
}

// GetJob implements GET /api/v1/jobs/{job_id}, optionally narrowed to jobs
// that have a given instance_number.
func (s *Server) GetJob(c *gin.Context, jobID openapi.JobID, params openapi.GetJobParams) {
	n, present, err := queryInt("instance_number", params.InstanceNumber)
	if err != nil {
		abortInvalidQuery(c, "instance_number", err.Error())
		return
	}
	var instanceNumber *int
	if present {
		instanceNumber = &n
	}

	job, err := s.svc.Jobs.Get(c.Request.Context(), jobID, instanceNumber)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, job)
}

// PatchJob implements PATCH /api/v1/jobs/{job_id}.
func (s *Server) PatchJob(c *gin.Context, jobID openapi.JobID) {
	data, ok := bindModel[model.Job](c, bindJSONMap)
	if !ok {
		return
	}

	updated, err := s.svc.Jobs.Update(c.Request.Context(), jobID, data)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, updated)
}

// DeleteJob implements DELETE /api/v1/jobs/{job_id}.
func (s *Server) DeleteJob(c *gin.Context, jobID openapi.JobID) {
	deleted, err := s.svc.Jobs.Delete(c.Request.Context(), jobID)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, deleted)
}

// GetJobInstance implements GET /api/v1/jobs/{job_id}/{instance_id}.
func (s *Server) GetJobInstance(c *gin.Context, jobID openapi.JobID, instanceNumber openapi.InstanceID) {
	result, err := s.svc.Jobs.GetInstance(c.Request.Context(), jobID, instanceNumber)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, result)
}

// AppendJobInstance implements PUT /api/v1/jobs/{job_id}/{instance_id}:
// appends the last element of the request body's instance_list to the job.
func (s *Server) AppendJobInstance(c *gin.Context, jobID openapi.JobID, instanceNumber openapi.InstanceID) {
	if !abstractor.IsValidID(jobID) {
		abortBadRequest(c)
		return
	}

	body, ok := bindJSONMap(c)
	if !ok {
		return
	}

	// Only instance_list matters here, so decode into a struct that picks
	// just that field out of the body rather than the whole thing.
	list, ok := decodeModel[struct {
		InstanceList []model.JobInstance `json:"instance_list"`
	}](c, body)
	if !ok {
		return
	}

	updated, err := s.svc.Jobs.AppendInstance(c.Request.Context(), jobID, instanceNumber, list.InstanceList)
	if err != nil {
		if errors.Is(err, abstractor.ErrInstanceExists) {
			c.AbortWithStatusJSON(http.StatusBadRequest, openapi.Message{Message: "Instance already exists"})
			return
		}
		s.abortInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, updated)
}

// PatchJobInstance implements PATCH /api/v1/jobs/{job_id}/{instance_id}.
func (s *Server) PatchJobInstance(c *gin.Context, jobID openapi.JobID, instanceNumber openapi.InstanceID) {
	data, ok := bindModel[model.JobInstance](c, bindJSONMap)
	if !ok {
		return
	}

	updated, err := s.svc.Jobs.UpdateInstance(c.Request.Context(), jobID, instanceNumber, data)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, updated)
}

// DeleteJobInstance implements DELETE /api/v1/jobs/{job_id}/{instance_id}.
func (s *Server) DeleteJobInstance(c *gin.Context, jobID openapi.JobID, instanceNumber openapi.InstanceID) {
	updated, err := s.svc.Jobs.DeleteInstance(c.Request.Context(), jobID, instanceNumber)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, updated)
}
