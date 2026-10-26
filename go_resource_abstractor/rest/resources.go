package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/oakestra/oakestra/go_resource_abstractor/abstractor"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
	"github.com/oakestra/oakestra/go_resource_abstractor/openapi"
)

// ListResources implements GET /api/v1/resources.
func (s *Server) ListResources(c *gin.Context, params openapi.ListResourcesParams) {
	active, _, err := queryBool("active", params.Active)
	if err != nil {
		abortInvalidQuery(c, "active", err.Error())
		return
	}

	filter := abstractor.ResourceFilter{
		Name:       strOrEmpty(params.CandidateName),
		IP:         strOrEmpty(params.Ip),
		JobID:      strOrEmpty(params.JobId),
		ActiveOnly: active,
	}
	if params.Resources != nil {
		filter.ExtraFields = *params.Resources
	}

	results, err := s.svc.Resources.List(c.Request.Context(), filter)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, results)
}

// CreateResource implements POST /api/v1/resources.
func (s *Server) CreateResource(c *gin.Context) {
	body, ok := bindResourceJSONMap(c)
	if !ok {
		return
	}
	if abortIfInvalidResourceFields(c, body) {
		return
	}
	data, ok := decodeModel[model.Resource](c, body)
	if !ok {
		return
	}

	created, err := s.svc.Resources.Create(c.Request.Context(), data)
	if err != nil {
		s.abortInternalError(c, err)
		return
	}

	c.JSON(http.StatusCreated, created)
}

// UpsertResource implements PUT /api/v1/resources: update-by-candidate_name
// if a match exists, else create.
func (s *Server) UpsertResource(c *gin.Context) {
	body, ok := bindResourceJSONMap(c)
	if !ok {
		return
	}
	if abortIfInvalidResourceFields(c, body) {
		return
	}
	data, ok := decodeModel[model.Resource](c, body)
	if !ok {
		return
	}

	updated, err := s.svc.Resources.Upsert(c.Request.Context(), data)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, updated)
}

// GetResource implements GET /api/v1/resources/{id}.
func (s *Server) GetResource(c *gin.Context, id openapi.ObjectID) {
	candidate, err := s.svc.Resources.Get(c.Request.Context(), id)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, candidate)
}

// PatchResource implements PATCH /api/v1/resources/{id}, persisting an
// aggregated usage report. Unlike GetResource, an invalid id here maps to
// 404, not 400, matching ResourceController.patch in the Python service.
func (s *Server) PatchResource(c *gin.Context, id openapi.ObjectID) {
	if !abstractor.IsValidID(id) {
		abortNotFound(c)
		return
	}

	body, ok := bindResourceJSONMap(c)
	if !ok {
		return
	}
	if abortIfInvalidResourceFields(c, body) {
		return
	}
	data, ok := decodeModel[model.Resource](c, body)
	if !ok {
		return
	}

	updated, err := s.svc.Resources.Report(c.Request.Context(), id, data)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, updated)
}

// DeleteResource implements DELETE /api/v1/resources/{id}.
func (s *Server) DeleteResource(c *gin.Context, id openapi.ObjectID) {
	err := s.svc.Resources.Delete(c.Request.Context(), id)
	if s.abortOnError(c, err) {
		return
	}

	c.Status(http.StatusNoContent)
}
