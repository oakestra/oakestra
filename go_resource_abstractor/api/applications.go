package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"

	"go_resource_abstractor/openapi"
)

// ListApplications implements GET /api/v1/applications.
func (s *Server) ListApplications(c *gin.Context, params openapi.ListApplicationsParams) {
	filter := applicationFilter(params.ApplicationName, params.ApplicationNamespace, params.UserId)

	results, err := s.store.FindApps(c.Request.Context(), filter)
	if err != nil {
		abortInternalError(c, err)
		return
	}

	writeJSON(c, http.StatusOK, results)
}

// CreateApplication implements POST /api/v1/applications. CreateApp populates
// applicationID with the new document's own stringified _id.
func (s *Server) CreateApplication(c *gin.Context) {
	data, ok := bindJSONMap(c)
	if !ok {
		return
	}

	created, err := s.apps.Create(c.Request.Context(), data, s.store.CreateApp)
	if err != nil {
		abortInternalError(c, err)
		return
	}

	writeJSON(c, http.StatusOK, created)
}

// GetApplication implements GET /api/v1/applications/{id}.
func (s *Server) GetApplication(c *gin.Context, id openapi.ObjectID, params openapi.GetApplicationParams) {
	filter := applicationFilter(params.ApplicationName, params.ApplicationNamespace, params.UserId)

	app, err := s.store.FindAppByID(c.Request.Context(), id, filter)
	if abortOnError(c, err) {
		return
	}

	writeJSON(c, http.StatusOK, app)
}

// PatchApplication implements PATCH /api/v1/applications/{id}.
func (s *Server) PatchApplication(c *gin.Context, id openapi.ObjectID) {
	data, ok := bindJSONMap(c)
	if !ok {
		return
	}

	updated, err := s.apps.Update(c.Request.Context(), id, data, s.store.UpdateApp)
	if abortOnError(c, err) {
		return
	}

	writeJSON(c, http.StatusOK, updated)
}

// DeleteApplication implements DELETE /api/v1/applications/{id}.
func (s *Server) DeleteApplication(c *gin.Context, id openapi.ObjectID) {
	deleted, err := s.apps.Delete(c.Request.Context(), id, s.store.DeleteApp)
	if abortOnError(c, err) {
		return
	}

	writeJSON(c, http.StatusOK, deleted)
}

// applicationFilter builds the Mongo filter shared by the list and fetch
// routes, which accept the same three query parameters.
func applicationFilter(name, namespace, userID *string) bson.M {
	filter := map[string]any{}
	addFilter(filter, "application_name", name)
	addFilter(filter, "application_namespace", namespace)
	addFilter(filter, "userId", userID)
	return bson.M(filter)
}
