package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"

	"go_resource_abstractor/db"
)

// registerJobRoutes wires up /api/v1/jobs, the Go port of jobs_blueprint.py
// (backed by the jobs collection).
func (s *Server) registerJobRoutes(v1 *gin.RouterGroup) {
	group := v1.Group("/jobs")

	bothSlashes(group, http.MethodGet, s.listJobs)
	bothSlashes(group, http.MethodPost, s.createJob)
	bothSlashes(group, http.MethodPut, s.upsertJob)

	itemBothSlashes(group, http.MethodGet, "/:job_id", s.getJob)
	itemBothSlashes(group, http.MethodPatch, "/:job_id", s.patchJob)
	itemBothSlashes(group, http.MethodDelete, "/:job_id", s.deleteJob)

	itemBothSlashes(group, http.MethodGet, "/:job_id/:instance_id", s.getJobInstance)
	itemBothSlashes(group, http.MethodPut, "/:job_id/:instance_id", s.appendJobInstance)
	itemBothSlashes(group, http.MethodPatch, "/:job_id/:instance_id", s.patchJobInstance)
	itemBothSlashes(group, http.MethodDelete, "/:job_id/:instance_id", s.deleteJobInstance)
}

// listJobs implements GET /jobs/, filterable by applicationID and job_name.
//
// The Python service also honored a raw ?params= value by assigning it as
// the entire Mongo filter, but that value is always a string and pymongo
// rejects a string filter, so the branch only ever produced a 500. It
// carried no usable behavior and is dropped here rather than reproduced.
func (s *Server) listJobs(c *gin.Context) {
	filter := map[string]any{}

	if appID := c.Query("applicationID"); appID != "" {
		filter["applicationID"] = appID
	}
	if jobName := c.Query("job_name"); jobName != "" {
		filter["job_name"] = jobName
	}

	results, err := s.store.FindJobs(c.Request.Context(), bson.M(filter))
	if err != nil {
		abortInternalError(c, err)
		return
	}

	writeJSON(c, http.StatusOK, results)
}

// createJob implements POST /jobs/.
func (s *Server) createJob(c *gin.Context) {
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

// upsertJob implements PUT /jobs/: update-by-job_name if a match exists,
// else create. Go port of AllJobsController.put.
//
// Deviation from the Python service: the original fires hooks under the
// entity name "job" (singular) here, while every other job route uses
// "jobs" - so a hook registered for "jobs" would never see this path's
// events. Normalized to "jobs" throughout so one hook registration covers
// all job writes.
func (s *Server) upsertJob(c *gin.Context) {
	data, ok := bindJSONMap(c)
	if !ok {
		return
	}
	s.upsertByName(c, "jobs", "job_name", data, s.store.FindJobByName, s.store.UpdateJob, s.store.CreateJob)
}

// getJob implements GET /jobs/<job_id>, optionally narrowed to jobs that
// have a given instance_number.
func (s *Server) getJob(c *gin.Context) {
	jobID := c.Param("job_id")
	if !isValidObjectID(jobID) {
		abortBadRequest(c)
		return
	}

	query := map[string]any{}
	if n, present, err := queryInt(c, "instance_number"); err != nil {
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

// patchJob implements PATCH /jobs/<job_id>.
func (s *Server) patchJob(c *gin.Context) {
	jobID := c.Param("job_id")
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

// deleteJob implements DELETE /jobs/<job_id>.
func (s *Server) deleteJob(c *gin.Context) {
	jobID := c.Param("job_id")
	ctx := c.Request.Context()

	deleted, err := s.store.DeleteJob(ctx, jobID)
	if abortOnError(c, err) {
		return
	}
	s.hooks.PostDelete("jobs", jobID)

	writeJSON(c, http.StatusOK, deleted)
}

// getJobInstance implements GET /jobs/<job_id>/<instance_id>.
func (s *Server) getJobInstance(c *gin.Context) {
	jobID := c.Param("job_id")
	if !isValidObjectID(jobID) {
		abortBadRequest(c)
		return
	}

	instanceNumber, ok := instanceIDParam(c)
	if !ok {
		abortBadRequest(c)
		return
	}

	result, err := s.store.FindJobInstance(c.Request.Context(), jobID, instanceNumber)
	if abortOnError(c, err) {
		return
	}

	writeJSON(c, http.StatusOK, result)
}

// appendJobInstance implements PUT /jobs/<job_id>/<instance_id>: appends
// the last element of the request body's instance_list to the job.
func (s *Server) appendJobInstance(c *gin.Context) {
	jobID := c.Param("job_id")
	if !isValidObjectID(jobID) {
		abortBadRequest(c)
		return
	}

	instanceNumber, ok := instanceIDParam(c)
	if !ok {
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
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"message": "Instance already exists"})
			return
		}
		abortInternalError(c, err)
		return
	}
	s.hooks.PostCreate("jobs", db.ExtractID(updated))

	writeJSON(c, http.StatusOK, updated)
}

// patchJobInstance implements PATCH /jobs/<job_id>/<instance_id>.
func (s *Server) patchJobInstance(c *gin.Context) {
	jobID := c.Param("job_id")
	if !isValidObjectID(jobID) {
		abortBadRequest(c)
		return
	}

	instanceNumber, ok := instanceIDParam(c)
	if !ok {
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

// deleteJobInstance implements DELETE /jobs/<job_id>/<instance_id>.
func (s *Server) deleteJobInstance(c *gin.Context) {
	jobID := c.Param("job_id")
	if !isValidObjectID(jobID) {
		abortBadRequest(c)
		return
	}

	instanceNumber, ok := instanceIDParam(c)
	if !ok {
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

// instanceIDParam parses the :instance_id path segment as an int - the same
// int(instance_id) coercion sprinkled through jobs_blueprint.py.
func instanceIDParam(c *gin.Context) (int, bool) {
	n, err := strconv.Atoi(c.Param("instance_id"))
	if err != nil {
		return 0, false
	}
	return n, true
}
