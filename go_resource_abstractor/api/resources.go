package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"

	"go_resource_abstractor/db"
)

// registerResourceRoutes wires up /api/v1/resources, mirroring
// resources_blueprint.py (backed by the candidates collection).
func (s *Server) registerResourceRoutes(v1 *gin.RouterGroup) {
	group := v1.Group("/resources")

	bothSlashes(group, http.MethodGet, s.listResources)
	bothSlashes(group, http.MethodPost, s.createResource)
	bothSlashes(group, http.MethodPut, s.upsertResource)

	itemBothSlashes(group, http.MethodGet, "/:id", s.getResource)
	itemBothSlashes(group, http.MethodPatch, "/:id", s.patchResource)
	itemBothSlashes(group, http.MethodDelete, "/:id", s.deleteResource)
}

// listResources implements GET /resources/. It supports the active,
// job_id, candidate_name and ip filters plus a ?resources=a,b,c projection
// extension, mirroring AllResourcesController.get.
func (s *Server) listResources(c *gin.Context) {
	filter := queryFilter(c, "job_id", "candidate_name", "ip")
	if active, present, err := queryBool(c, "active"); err != nil {
		abortInvalidQuery(c, "active", err.Error())
		return
	} else if present {
		filter["active"] = active
	}

	ctx := c.Request.Context()

	if jobID, ok := filter["job_id"].(string); ok && jobID != "" {
		if !isValidObjectID(jobID) {
			abortBadRequest(c)
			return
		}

		candidateID, err := s.store.ResolveJobCandidate(ctx, jobID)
		if abortOnError(c, err) {
			return
		}
		filter["candidate_id"] = candidateID
	}

	mongoFilter, err := db.BuildCandidateFilter(filter)
	if err != nil {
		abortBadRequest(c)
		return
	}

	var resources []string
	if raw := c.Query("resources"); raw != "" {
		resources = strings.Split(raw, ",")
	}

	results, err := s.store.FindCandidates(ctx, mongoFilter, resources)
	if err != nil {
		abortInternalError(c, err)
		return
	}

	writeJSON(c, http.StatusOK, results)
}

// createResource implements POST /resources/.
func (s *Server) createResource(c *gin.Context) {
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

// upsertResource implements PUT /resources/: update-by-candidate_name if a
// match exists, else create. Mirrors AllResourcesController.put.
func (s *Server) upsertResource(c *gin.Context) {
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

// getResource implements GET /resources/<id>.
func (s *Server) getResource(c *gin.Context) {
	id := c.Param("id")
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

// patchResource implements PATCH /resources/<id>, persisting an aggregated
// usage report via UpdateCandidateInformation. Note: unlike getResource, an
// invalid id here maps to 404 (not 400), matching ResourceController.patch
// in the Python service.
func (s *Server) patchResource(c *gin.Context) {
	id := c.Param("id")
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

// deleteResource implements DELETE /resources/<id>.
func (s *Server) deleteResource(c *gin.Context) {
	id := c.Param("id")
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
