package api

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/jsonschema-go/jsonschema"
	"go.mongodb.org/mongo-driver/v2/bson"

	"go_resource_abstractor/db"
)

// registerCustomResourceRoutes wires up /api/v1/custom-resources, mirroring
// custom_resources_blueprint.py. Not used by the scheduler or root/cluster
// managers, but part of the public API surface.
func (s *Server) registerCustomResourceRoutes(v1 *gin.RouterGroup) {
	group := v1.Group("/custom-resources")

	bothSlashes(group, http.MethodGet, s.listCustomResourceDefinitions)
	bothSlashes(group, http.MethodPost, s.createCustomResourceDefinition)

	// The Python service registers two separate MethodViews on the same
	// single-segment path - CustomResourceDefinitionController for DELETE
	// /<resource_type>, ResourcesController for GET/POST /<resource> -
	// which Flask resolves purely by HTTP method. gin can't register two
	// handlers for the identical path pattern, so both are consolidated
	// into this one route group keyed by method: same effective dispatch,
	// one place to read it.
	group.GET("/:resource", s.listCustomResourceInstances)
	group.POST("/:resource", s.createCustomResourceInstance)
	group.DELETE("/:resource", s.deleteCustomResourceDefinition)

	group.GET("/:resource/:id", s.getCustomResourceInstance)
	group.PATCH("/:resource/:id", s.patchCustomResourceInstance)
	group.DELETE("/:resource/:id", s.deleteCustomResourceInstance)
}

// listCustomResourceDefinitions implements GET /custom-resources/.
func (s *Server) listCustomResourceDefinitions(c *gin.Context) {
	defs, err := s.store.FindCustomResources(c.Request.Context())
	if err != nil {
		abortInternalError(c, err)
		return
	}
	writeJSON(c, http.StatusOK, defs)
}

// createCustomResourceDefinition implements POST /custom-resources/,
// registering a new resource type. resource_type is required, mirroring
// CustomResourceSchema.
func (s *Server) createCustomResourceDefinition(c *gin.Context) {
	data, ok := bindJSONMap(c)
	if !ok {
		return
	}
	if resourceType, _ := data["resource_type"].(string); resourceType == "" {
		abortBadRequest(c)
		return
	}

	created, err := s.store.CreateCustomResource(c.Request.Context(), bson.M(data))
	if err != nil {
		abortInternalError(c, err)
		return
	}

	writeJSON(c, http.StatusCreated, created)
}

// deleteCustomResourceDefinition implements DELETE
// /custom-resources/<type>: a cascading delete of the definition and every
// instance of that type. The definition delete itself supplies the
// not-found check, rather than a separate lookup beforehand.
func (s *Server) deleteCustomResourceDefinition(c *gin.Context) {
	resourceType := c.Param("resource")
	ctx := c.Request.Context()

	if _, err := s.store.DeleteCustomResourceByType(ctx, resourceType); abortOnError(c, err) {
		return
	}

	if _, err := s.store.DeleteAllResources(ctx, resourceType); err != nil {
		abortInternalError(c, err)
		return
	}

	writeJSON(c, http.StatusOK, gin.H{
		"message": "Resource type '" + resourceType + "' and all its instances deleted",
	})
}

// listCustomResourceInstances implements GET /custom-resources/<resource>.
// Every query param is passed straight through as a MongoDB filter,
// including dotted nested-field keys (e.g. ?parent.child=value).
func (s *Server) listCustomResourceInstances(c *gin.Context) {
	resourceType := c.Param("resource")
	ctx := c.Request.Context()

	if _, ok := s.findCustomResourceType(c, resourceType); !ok {
		return
	}

	filter := bson.M{}
	for key, values := range c.Request.URL.Query() {
		if len(values) > 0 {
			filter[key] = values[0]
		}
	}

	results, err := s.store.FindResources(ctx, resourceType, filter)
	if err != nil {
		abortInternalError(c, err)
		return
	}

	writeJSON(c, http.StatusOK, results)
}

// createCustomResourceInstance implements POST /custom-resources/<resource>,
// validating the body against the type's stored JSON Schema.
func (s *Server) createCustomResourceInstance(c *gin.Context) {
	resourceType := c.Param("resource")
	data, ok := bindJSONMap(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	def, ok := s.findCustomResourceType(c, resourceType)
	if !ok {
		return
	}

	if msg, valid := validateAgainstSchema(def, data); !valid {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"message": msg})
		return
	}

	data = s.hooks.PreCreate(ctx, resourceType, data)

	created, err := s.store.CreateResource(ctx, resourceType, bson.M(data))
	if err != nil {
		abortInternalError(c, err)
		return
	}
	s.hooks.PostCreate(resourceType, db.ExtractID(created))

	writeJSON(c, http.StatusOK, created)
}

// getCustomResourceInstance implements GET /custom-resources/<resource>/<id>.
func (s *Server) getCustomResourceInstance(c *gin.Context) {
	resourceType := c.Param("resource")
	id := c.Param("id")
	ctx := c.Request.Context()

	if _, ok := s.findCustomResourceType(c, resourceType); !ok {
		return
	}

	result, err := s.store.FindResourceByID(ctx, resourceType, id)
	if abortOnError(c, err) {
		return
	}

	writeJSON(c, http.StatusOK, result)
}

// patchCustomResourceInstance implements PATCH
// /custom-resources/<resource>/<id>, re-validating the body against the
// type's stored JSON Schema.
func (s *Server) patchCustomResourceInstance(c *gin.Context) {
	resourceType := c.Param("resource")
	id := c.Param("id")
	data, ok := bindJSONMap(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	def, ok := s.findCustomResourceType(c, resourceType)
	if !ok {
		return
	}

	if msg, valid := validateAgainstSchema(def, data); !valid {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"message": msg})
		return
	}

	data["_id"] = id
	data = s.hooks.PreUpdate(ctx, resourceType, data)

	updated, err := s.store.UpdateResource(ctx, resourceType, id, bson.M(data))
	if abortOnError(c, err) {
		return
	}
	s.hooks.PostUpdate(resourceType, db.ExtractID(updated))

	writeJSON(c, http.StatusOK, updated)
}

// deleteCustomResourceInstance implements DELETE
// /custom-resources/<resource>/<id>.
func (s *Server) deleteCustomResourceInstance(c *gin.Context) {
	resourceType := c.Param("resource")
	id := c.Param("id")
	ctx := c.Request.Context()

	if _, ok := s.findCustomResourceType(c, resourceType); !ok {
		return
	}

	if _, err := s.store.DeleteResource(ctx, resourceType, id); err != nil && !isNotFound(err) {
		abortInternalError(c, err)
		return
	}
	s.hooks.PostDelete(resourceType, id)

	writeJSON(c, http.StatusOK, gin.H{"_id": id})
}

// findCustomResourceType looks up resourceType's definition, aborting the
// request (404/500) and reporting ok=false if it doesn't exist or the
// lookup fails. Shared by every /custom-resources/<resource>[/...] handler,
// which all require the type to be registered before touching its instances.
func (s *Server) findCustomResourceType(c *gin.Context, resourceType string) (def bson.M, ok bool) {
	def, err := s.store.FindCustomResourceByType(c.Request.Context(), resourceType)
	if abortOnError(c, err) {
		return nil, false
	}
	return def, true
}

// validateAgainstSchema validates data against the JSON Schema stored in
// def["schema"], mirroring jsonschema.validate(data, meta_data["schema"])
// in the Python service. A missing/empty schema, or one that fails to
// compile, is treated as "no constraint" rather than a 500 - the Python
// service doesn't guard against a malformed stored schema either, but
// failing open here is friendlier than crashing the request.
func validateAgainstSchema(def bson.M, data map[string]any) (message string, valid bool) {
	rawSchema, ok := def["schema"]
	if !ok || rawSchema == nil {
		return "", true
	}

	body, err := json.Marshal(db.Normalize(rawSchema))
	if err != nil {
		return "", true
	}

	var schema jsonschema.Schema
	if err := json.Unmarshal(body, &schema); err != nil {
		return "", true
	}

	resolved, err := schema.Resolve(nil)
	if err != nil {
		return "", true
	}

	if err := resolved.Validate(data); err != nil {
		return err.Error(), false
	}
	return "", true
}
