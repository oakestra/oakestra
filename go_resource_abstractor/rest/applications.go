package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/oakestra/oakestra/go_resource_abstractor/abstractor"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
	"github.com/oakestra/oakestra/go_resource_abstractor/openapi"
)

// applicationFilter builds the abstractor.ApplicationFilter shared by the
// list and fetch routes, which accept the same three query parameters.
func applicationFilter(name, namespace, userID *string) abstractor.ApplicationFilter {
	return abstractor.ApplicationFilter{
		Name:      strOrEmpty(name),
		Namespace: strOrEmpty(namespace),
		UserID:    strOrEmpty(userID),
	}
}

// ListApplications implements GET /api/v1/applications.
func (s *Server) ListApplications(c *gin.Context, params openapi.ListApplicationsParams) {
	filter := applicationFilter(params.ApplicationName, params.ApplicationNamespace, params.UserId)

	results, err := s.svc.Apps.List(c.Request.Context(), filter)
	if err != nil {
		s.abortInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, results)
}

// CreateApplication implements POST /api/v1/applications. The store always
// populates applicationID with the new document's own stringified _id.
func (s *Server) CreateApplication(c *gin.Context) {
	data, ok := bindModel[model.Application](c, bindJSONMap)
	if !ok {
		return
	}

	created, err := s.svc.Apps.Create(c.Request.Context(), data)
	if err != nil {
		s.abortInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, created)
}

// GetApplication implements GET /api/v1/applications/{id}.
func (s *Server) GetApplication(c *gin.Context, id openapi.ObjectID, params openapi.GetApplicationParams) {
	filter := applicationFilter(params.ApplicationName, params.ApplicationNamespace, params.UserId)

	app, err := s.svc.Apps.Get(c.Request.Context(), id, filter)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, app)
}

// PatchApplication implements PATCH /api/v1/applications/{id}.
func (s *Server) PatchApplication(c *gin.Context, id openapi.ObjectID) {
	data, ok := bindModel[model.Application](c, bindJSONMap)
	if !ok {
		return
	}

	updated, err := s.svc.Apps.Update(c.Request.Context(), id, data)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, updated)
}

// DeleteApplication implements DELETE /api/v1/applications/{id}.
func (s *Server) DeleteApplication(c *gin.Context, id openapi.ObjectID) {
	deleted, err := s.svc.Apps.Delete(c.Request.Context(), id)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, deleted)
}
