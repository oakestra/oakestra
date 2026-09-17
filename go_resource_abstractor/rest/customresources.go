package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/oakestra/oakestra/go_resource_abstractor/model"
	"github.com/oakestra/oakestra/go_resource_abstractor/openapi"
)

// ListCustomResourceDefinitions implements GET /api/v1/custom-resources.
func (s *Server) ListCustomResourceDefinitions(c *gin.Context) {
	defs, err := s.svc.CustomResources.ListDefinitions(c.Request.Context())
	if err != nil {
		s.abortInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, defs)
}

// CreateCustomResourceDefinition implements POST /api/v1/custom-resources,
// registering a new resource type. resource_type is required; the service
// enforces that plus the reserved-name/collection-name rules.
func (s *Server) CreateCustomResourceDefinition(c *gin.Context) {
	body, ok := bindOptionalJSONMap(c)
	if !ok {
		return
	}
	data, ok := decodeModel[model.CustomResourceDefinition](c, body)
	if !ok {
		return
	}

	created, err := s.svc.CustomResources.CreateDefinition(c.Request.Context(), data)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusCreated, created)
}

// DeleteCustomResourceDefinition implements DELETE
// /api/v1/custom-resources/{resource}: a cascading delete of the definition
// and every instance of that type. Returns 404 if the type isn't registered.
func (s *Server) DeleteCustomResourceDefinition(c *gin.Context, resourceType openapi.CustomResourceType) {
	_, _, err := s.svc.CustomResources.DeleteDefinition(c.Request.Context(), resourceType)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, openapi.Message{
		Message: "Resource type '" + resourceType + "' and all its instances deleted",
	})
}

// ListCustomResourceInstances implements GET
// /api/v1/custom-resources/{resource}. Every query param is passed straight
// through as a MongoDB filter, including dotted nested-field keys (e.g.
// ?parent.child=value), so the spec declares no query parameters for this
// operation and the raw query string is read here instead. No key may start
// with "$", so a filter key can't double as a MongoDB operator.
func (s *Server) ListCustomResourceInstances(c *gin.Context, resourceType openapi.CustomResourceType) {
	filter := map[string]string{}
	for key, values := range c.Request.URL.Query() {
		if len(values) > 0 {
			filter[key] = values[0]
		}
	}

	results, err := s.svc.CustomResources.ListInstances(c.Request.Context(), resourceType, filter)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, results)
}

// CreateCustomResourceInstance implements POST
// /api/v1/custom-resources/{resource}, validating the body against the
// type's stored JSON Schema.
func (s *Server) CreateCustomResourceInstance(c *gin.Context, resourceType openapi.CustomResourceType) {
	body, ok := bindOptionalJSONMap(c)
	if !ok {
		return
	}
	data, ok := decodeModel[model.CustomResourceInstance](c, body)
	if !ok {
		return
	}

	created, err := s.svc.CustomResources.CreateInstance(c.Request.Context(), resourceType, data)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, created)
}

// GetCustomResourceInstance implements GET
// /api/v1/custom-resources/{resource}/{id}.
func (s *Server) GetCustomResourceInstance(c *gin.Context, resourceType openapi.CustomResourceType, id openapi.ObjectID) {
	result, err := s.svc.CustomResources.GetInstance(c.Request.Context(), resourceType, id)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, result)
}

// PatchCustomResourceInstance implements PATCH
// /api/v1/custom-resources/{resource}/{id}, re-validating the body against
// the type's stored JSON Schema.
func (s *Server) PatchCustomResourceInstance(c *gin.Context, resourceType openapi.CustomResourceType, id openapi.ObjectID) {
	body, ok := bindOptionalJSONMap(c)
	if !ok {
		return
	}
	data, ok := decodeModel[model.CustomResourceInstance](c, body)
	if !ok {
		return
	}

	updated, err := s.svc.CustomResources.UpdateInstance(c.Request.Context(), resourceType, id, data)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, updated)
}

// DeleteCustomResourceInstance implements DELETE
// /api/v1/custom-resources/{resource}/{id}.
func (s *Server) DeleteCustomResourceInstance(c *gin.Context, resourceType openapi.CustomResourceType, id openapi.ObjectID) {
	err := s.svc.CustomResources.DeleteInstance(c.Request.Context(), resourceType, id)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, openapi.DeletedID{ID: id})
}
