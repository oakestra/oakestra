package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"

	"go_resource_abstractor/db"
)

// applicationFilterKeys are the query params accepted as an applications
// filter by both GET /applications/ and GET /applications/<id>.
var applicationFilterKeys = []string{"application_name", "application_namespace", "userId"}

// registerApplicationRoutes wires up /api/v1/applications, mirroring
// apps_blueprint.py (backed by the apps collection).
func (s *Server) registerApplicationRoutes(v1 *gin.RouterGroup) {
	group := v1.Group("/applications")

	bothSlashes(group, http.MethodGet, s.listApplications)
	bothSlashes(group, http.MethodPost, s.createApplication)

	group.GET("/:id", s.getApplication)
	group.PATCH("/:id", s.patchApplication)
	group.DELETE("/:id", s.deleteApplication)
}

// listApplications implements GET /applications/, filterable by
// application_name, application_namespace and userId.
func (s *Server) listApplications(c *gin.Context) {
	filter := queryFilter(c, applicationFilterKeys...)

	results, err := s.store.FindApps(c.Request.Context(), bson.M(filter))
	if err != nil {
		abortInternalError(c, err)
		return
	}

	writeJSON(c, http.StatusOK, results)
}

// createApplication implements POST /applications/. CreateApp populates
// applicationID with the new document's own stringified _id.
func (s *Server) createApplication(c *gin.Context) {
	data, ok := bindJSONMap(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	data = s.hooks.PreCreate(ctx, "applications", data)

	created, err := s.store.CreateApp(ctx, bson.M(data))
	if err != nil {
		abortInternalError(c, err)
		return
	}
	s.hooks.PostCreate("applications", db.ExtractID(created))

	writeJSON(c, http.StatusOK, created)
}

// getApplication implements GET /applications/<id>.
func (s *Server) getApplication(c *gin.Context) {
	id := c.Param("id")
	filter := queryFilter(c, applicationFilterKeys...)

	app, err := s.store.FindAppByID(c.Request.Context(), id, bson.M(filter))
	if abortOnError(c, err) {
		return
	}

	writeJSON(c, http.StatusOK, app)
}

// patchApplication implements PATCH /applications/<id>.
func (s *Server) patchApplication(c *gin.Context) {
	id := c.Param("id")
	data, ok := bindJSONMap(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	data["_id"] = id
	data = s.hooks.PreUpdate(ctx, "applications", data)

	updated, err := s.store.UpdateApp(ctx, id, bson.M(data))
	if abortOnError(c, err) {
		return
	}
	s.hooks.PostUpdate("applications", db.ExtractID(updated))

	writeJSON(c, http.StatusOK, updated)
}

// deleteApplication implements DELETE /applications/<id>.
func (s *Server) deleteApplication(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()

	deleted, err := s.store.DeleteApp(ctx, id)
	if abortOnError(c, err) {
		return
	}
	s.hooks.PostDelete("applications", id)

	writeJSON(c, http.StatusOK, deleted)
}
