package api

import (
	"context"
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

	created, err := s.resources.Create(c.Request.Context(), data, s.store.CreateCandidate)
	if err != nil {
		abortInternalError(c, err)
		return
	}

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
	s.upsertByName(c, s.resources, "candidate_name", data,
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

	updated, err := s.resources.Update(c.Request.Context(), id, data, s.store.UpdateCandidateInformation)
	if abortOnError(c, err) {
		return
	}

	writeJSON(c, http.StatusOK, updated)
}

// DeleteResource implements DELETE /api/v1/resources/{id}.
func (s *Server) DeleteResource(c *gin.Context, id openapi.ObjectID) {
	if !isValidObjectID(id) {
		abortBadRequest(c)
		return
	}

	// DeleteCandidate only returns an error, not the deleted document, so
	// Delete gets nil here and keys post_delete off the path id instead. A
	// candidate that's already gone is not treated as an error.
	_, err := s.resources.Delete(c.Request.Context(), id, func(ctx context.Context, id string) (bson.M, error) {
		return nil, s.store.DeleteCandidate(ctx, id)
	})
	if err != nil {
		abortInternalError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}
