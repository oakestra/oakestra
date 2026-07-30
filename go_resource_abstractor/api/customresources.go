package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/jsonschema-go/jsonschema"
	"go.mongodb.org/mongo-driver/v2/bson"

	"go_resource_abstractor/db"
	"go_resource_abstractor/openapi"
)

// ListCustomResourceDefinitions implements GET /api/v1/custom-resources.
func (s *Server) ListCustomResourceDefinitions(c *gin.Context) {
	defs, err := s.store.FindCustomResources(c.Request.Context())
	if err != nil {
		abortInternalError(c, err)
		return
	}
	writeJSON(c, http.StatusOK, defs)
}

// CreateCustomResourceDefinition implements POST /api/v1/custom-resources,
// registering a new resource type. resource_type is required, the same as the
// spec's CustomResourceDefinition schema declares.
func (s *Server) CreateCustomResourceDefinition(c *gin.Context) {
	data, ok := bindOptionalJSONMap(c)
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

// DeleteCustomResourceDefinition implements DELETE
// /api/v1/custom-resources/{resource}: a cascading delete of the definition
// and every instance of that type. Returns 404 if the type isn't registered.
func (s *Server) DeleteCustomResourceDefinition(c *gin.Context, resourceType openapi.CustomResourceType) {
	ctx := c.Request.Context()

	// Match the Python service's order: confirm the definition exists, drop
	// every instance first, then the definition itself. Deleting instances
	// before the definition means a failure of the second step can't leave
	// orphaned instances behind with no definition to reach them (the earlier
	// order here deleted the definition first, purely to reuse it as the
	// not-found check).
	if _, ok := s.findCustomResourceType(c, resourceType); !ok {
		return
	}

	if _, err := s.store.DeleteAllResources(ctx, resourceType); err != nil {
		abortInternalError(c, err)
		return
	}

	if _, err := s.store.DeleteCustomResourceByType(ctx, resourceType); err != nil && !isNotFound(err) {
		abortInternalError(c, err)
		return
	}

	writeJSON(c, http.StatusOK, openapi.Message{
		Message: "Resource type '" + resourceType + "' and all its instances deleted",
	})
}

// ListCustomResourceInstances implements GET
// /api/v1/custom-resources/{resource}. Every query param is passed straight
// through as a MongoDB filter, including dotted nested-field keys (e.g.
// ?parent.child=value) - which is why the spec declares no query parameters
// for this operation and the raw query string is read here instead.
func (s *Server) ListCustomResourceInstances(c *gin.Context, resourceType openapi.CustomResourceType) {
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

// CreateCustomResourceInstance implements POST
// /api/v1/custom-resources/{resource}, validating the body against the type's
// stored JSON Schema.
func (s *Server) CreateCustomResourceInstance(c *gin.Context, resourceType openapi.CustomResourceType) {
	data, ok := bindOptionalJSONMap(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	def, ok := s.findCustomResourceType(c, resourceType)
	if !ok {
		return
	}
	if !abortIfInvalidSchema(c, def, data) {
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

// GetCustomResourceInstance implements GET
// /api/v1/custom-resources/{resource}/{id}.
func (s *Server) GetCustomResourceInstance(c *gin.Context, resourceType openapi.CustomResourceType, id openapi.ObjectID) {
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

// PatchCustomResourceInstance implements PATCH
// /api/v1/custom-resources/{resource}/{id}, re-validating the body against
// the type's stored JSON Schema.
func (s *Server) PatchCustomResourceInstance(c *gin.Context, resourceType openapi.CustomResourceType, id openapi.ObjectID) {
	data, ok := bindOptionalJSONMap(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	def, ok := s.findCustomResourceType(c, resourceType)
	if !ok {
		return
	}
	if !abortIfInvalidSchema(c, def, data) {
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

// DeleteCustomResourceInstance implements DELETE
// /api/v1/custom-resources/{resource}/{id}.
func (s *Server) DeleteCustomResourceInstance(c *gin.Context, resourceType openapi.CustomResourceType, id openapi.ObjectID) {
	ctx := c.Request.Context()

	if _, ok := s.findCustomResourceType(c, resourceType); !ok {
		return
	}

	if _, err := s.store.DeleteResource(ctx, resourceType, id); err != nil && !isNotFound(err) {
		abortInternalError(c, err)
		return
	}
	s.hooks.PostDelete(resourceType, id)

	writeJSON(c, http.StatusOK, openapi.DeletedID{ID: id})
}

// findCustomResourceType looks up resourceType's definition, aborting the
// request (404/500) and reporting ok=false if it doesn't exist or the
// lookup fails. Shared by every /custom-resources/{resource}[/...] handler,
// which all require the type to be registered before touching its instances.
func (s *Server) findCustomResourceType(c *gin.Context, resourceType string) (def bson.M, ok bool) {
	def, err := s.store.FindCustomResourceByType(c.Request.Context(), resourceType)
	if abortOnError(c, err) {
		return nil, false
	}
	return def, true
}

// validateAgainstSchema validates data against the JSON Schema stored in
// def["schema"], the Go equivalent of jsonschema.validate(data,
// meta_data["schema"]) in the Python service. A missing/empty schema is "no
// constraint", the same default Python's meta_data.get("schema", {}) gives. A schema that
// fails to compile is reported as a server error rather than silently
// accepting any payload: Python's jsonschema.validate call has the same
// failure mode there (a malformed schema raises jsonschema.SchemaError,
// which isn't caught by the blueprint's except ValidationError clause and
// surfaces as an uncaught 500), so this keeps the same "don't persist
// against a broken schema" outcome instead of failing open.
func validateAgainstSchema(def bson.M, data map[string]any) (message string, valid bool, err error) {
	rawSchema, ok := def["schema"]
	if !ok || rawSchema == nil {
		return "", true, nil
	}

	body, err := json.Marshal(db.Normalize(rawSchema))
	if err != nil {
		return "", false, fmt.Errorf("marshal stored schema: %w", err)
	}

	var schema jsonschema.Schema
	if err := json.Unmarshal(body, &schema); err != nil {
		return "", false, fmt.Errorf("decode stored schema: %w", err)
	}

	resolved, err := schema.Resolve(nil)
	if err != nil {
		return "", false, fmt.Errorf("resolve stored schema: %w", err)
	}

	if err := resolved.Validate(data); err != nil {
		return err.Error(), false, nil
	}
	return "", true, nil
}

// abortIfInvalidSchema validates data against def's stored JSON Schema and
// aborts the request on either outcome validateAgainstSchema can report: a
// broken stored schema (500, via abortInternalError) or a schema-invalid
// payload (400). Reports ok=false if the request was aborted.
func abortIfInvalidSchema(c *gin.Context, def bson.M, data map[string]any) (ok bool) {
	msg, valid, err := validateAgainstSchema(def, data)
	if err != nil {
		abortInternalError(c, err)
		return false
	}
	if !valid {
		c.AbortWithStatusJSON(http.StatusBadRequest, openapi.Message{Message: msg})
		return false
	}
	return true
}
