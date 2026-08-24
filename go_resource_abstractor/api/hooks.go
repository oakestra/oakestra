package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"

	"go_resource_abstractor/openapi"
)

// ListHooks implements GET /api/v1/hooks.
func (s *Server) ListHooks(c *gin.Context) {
	hooks, err := s.store.FindHooks(c.Request.Context(), nil)
	if err != nil {
		abortInternalError(c, err)
		return
	}
	writeJSON(c, http.StatusOK, hooks)
}

// CreateHook implements POST /api/v1/hooks. events entries are validated
// against the HookEvent enum openapi.yaml declares, the same check as the
// OneOf validator on APIObjectPostHookSchema.events.
func (s *Server) CreateHook(c *gin.Context) {
	data, ok := bindOptionalJSONMap(c)
	if !ok {
		return
	}
	if !validHookEvents(data) {
		abortBadRequest(c)
		return
	}

	created, err := s.store.CreateHook(c.Request.Context(), bson.M(data))
	if err != nil {
		abortInternalError(c, err)
		return
	}

	writeJSON(c, http.StatusCreated, created)
}

// GetHook implements GET /api/v1/hooks/{id}.
func (s *Server) GetHook(c *gin.Context, id openapi.ObjectID) {
	if !isValidObjectID(id) {
		abortNotFound(c)
		return
	}

	hook, err := s.store.FindHookByID(c.Request.Context(), id)
	if abortOnError(c, err) {
		return
	}

	writeJSON(c, http.StatusOK, hook)
}

// PatchHook implements PATCH /api/v1/hooks/{id}.
//
// SingleHookController.patch passes validate=False, but that only skips
// whole-object validators - the field-level OneOf on
// APIObjectPostHookSchema.events still runs, so an invalid event name is
// still rejected there. Validated here to match, with the same 400 as
// CreateHook.
func (s *Server) PatchHook(c *gin.Context, id openapi.ObjectID) {
	data, ok := bindOptionalJSONMap(c)
	if !ok {
		return
	}
	if !validHookEvents(data) {
		abortBadRequest(c)
		return
	}

	updated, err := s.store.UpdateHook(c.Request.Context(), id, bson.M(data))
	if abortOnError(c, err) {
		return
	}

	writeJSON(c, http.StatusOK, updated)
}

// DeleteHook implements DELETE /api/v1/hooks/{id}.
func (s *Server) DeleteHook(c *gin.Context, id openapi.ObjectID) {
	if !isValidObjectID(id) {
		abortBadRequest(c)
		return
	}

	if err := s.store.DeleteHook(c.Request.Context(), id); err != nil {
		abortInternalError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// validHookEvents checks that every entry in data["events"] (if present) is a
// member of the HookEvent enum, using the membership test generated from the
// spec's enum rather than a second hand-maintained list.
func validHookEvents(data map[string]any) bool {
	raw, ok := data["events"]
	if !ok {
		return true
	}
	events, ok := raw.([]any)
	if !ok {
		return false
	}

	for _, e := range events {
		name, ok := e.(string)
		if !ok || !openapi.HookEvent(name).Valid() {
			return false
		}
	}
	return true
}
