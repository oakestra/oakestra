package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"

	"go_resource_abstractor/db"
	"go_resource_abstractor/openapi"
)

// ListResources implements GET /api/v1/resources.
func (s *Server) ListResources(c *gin.Context, params openapi.ListResourcesParams) {
	active, _, err := queryBool("active", params.Active)
	if err != nil {
		abortInvalidQuery(c, "active", err.Error())
		return
	}

	filter := db.CandidateFilter{
		CandidateName: queryString(params.CandidateName),
		IP:            queryString(params.Ip),
		ActiveOnly:    active,
	}

	ctx := c.Request.Context()

	if jobID := queryString(params.JobId); jobID != "" {
		if !isValidObjectID(jobID) {
			abortBadRequest(c)
			return
		}

		candidateID, err := s.store.ResolveJobCandidate(ctx, jobID)
		if abortOnError(c, err) {
			return
		}
		filter.CandidateID = candidateID
	}

	mongoFilter, err := db.BuildCandidateFilter(filter)
	if err != nil {
		abortBadRequest(c)
		return
	}

	var resources []string
	if params.Resources != nil {
		resources = *params.Resources
	}

	results, err := s.store.FindCandidates(ctx, mongoFilter, resources)
	if err != nil {
		abortInternalError(c, err)
		return
	}

	writeJSON(c, http.StatusOK, results)
}

// CreateResource implements POST /api/v1/resources.
func (s *Server) CreateResource(c *gin.Context) {
	data, ok := bindResourceJSONMap(c)
	if !ok {
		return
	}
	if !abortIfInvalidResourceFields(c, data) {
		return
	}

	ctx := c.Request.Context()
	data = s.hooks.PreCreate(ctx, "resources", data)

	created, err := s.store.CreateCandidate(ctx, bson.M(data))
	if err != nil {
		abortInternalError(c, err)
		return
	}
	s.hooks.PostCreate("resources", db.ExtractID(created))

	writeJSON(c, http.StatusCreated, created)
}

// UpsertResource implements PUT /api/v1/resources: update-by-candidate_name
// if a match exists, else create.
func (s *Server) UpsertResource(c *gin.Context) {
	data, ok := bindResourceJSONMap(c)
	if !ok {
		return
	}
	if !abortIfInvalidResourceFields(c, data) {
		return
	}
	s.upsertByName(c, "resources", "candidate_name", data,
		s.store.FindCandidateByName, s.store.UpdateCandidate, s.store.CreateCandidate)
}

// GetResource implements GET /api/v1/resources/{id}.
func (s *Server) GetResource(c *gin.Context, id openapi.ObjectID) {
	if !isValidObjectID(id) {
		abortBadRequest(c)
		return
	}

	candidate, err := s.store.FindCandidateByID(c.Request.Context(), id)
	if abortOnError(c, err) {
		return
	}

	writeJSON(c, http.StatusOK, candidate)
}

// PatchResource implements PATCH /api/v1/resources/{id}, persisting an
// aggregated usage report via UpdateCandidateInformation. Note: unlike
// GetResource, an invalid id here maps to 404 (not 400) -
// ResourceController.patch in the Python service does the same.
func (s *Server) PatchResource(c *gin.Context, id openapi.ObjectID) {
	if !isValidObjectID(id) {
		abortNotFound(c)
		return
	}

	data, ok := bindResourceJSONMap(c)
	if !ok {
		return
	}
	if !abortIfInvalidResourceFields(c, data) {
		return
	}
	ctx := c.Request.Context()

	data["_id"] = id
	data = s.hooks.PreUpdate(ctx, "resources", data)

	updated, err := s.store.UpdateCandidateInformation(ctx, id, bson.M(data))
	if abortOnError(c, err) {
		return
	}
	s.hooks.PostUpdate("resources", db.ExtractID(updated))

	writeJSON(c, http.StatusOK, updated)
}

// DeleteResource implements DELETE /api/v1/resources/{id}.
func (s *Server) DeleteResource(c *gin.Context, id openapi.ObjectID) {
	if !isValidObjectID(id) {
		abortBadRequest(c)
		return
	}

	ctx := c.Request.Context()
	if err := s.store.DeleteCandidate(ctx, id); err != nil {
		abortInternalError(c, err)
		return
	}
	s.hooks.PostDelete("resources", id)

	c.Status(http.StatusNoContent)
}
