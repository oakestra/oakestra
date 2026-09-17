package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/oakestra/oakestra/go_resource_abstractor/abstractor"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
	"github.com/oakestra/oakestra/go_resource_abstractor/openapi"
)

// ListHooks implements GET /api/v1/hooks.
func (s *Server) ListHooks(c *gin.Context) {
	hooks, err := s.svc.Hooks.List(c.Request.Context())
	if err != nil {
		s.abortInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, hooks)
}

// CreateHook implements POST /api/v1/hooks. events entries are validated
// against the HookEvent enum openapi.yaml declares, the same check as the
// OneOf validator on APIObjectPostHookSchema.events.
func (s *Server) CreateHook(c *gin.Context) {
	data, ok := bindModel[model.Hook](c, bindOptionalJSONMap)
	if !ok {
		return
	}

	created, err := s.svc.Hooks.Create(c.Request.Context(), data)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusCreated, created)
}

// GetHook implements GET /api/v1/hooks/{id}.
func (s *Server) GetHook(c *gin.Context, id openapi.ObjectID) {
	if !abstractor.IsValidID(id) {
		abortNotFound(c)
		return
	}

	hook, err := s.svc.Hooks.Get(c.Request.Context(), id)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, hook)
}

// PatchHook implements PATCH /api/v1/hooks/{id}.
//
// SingleHookController.patch passes validate=False, but that only skips
// whole-object validators; the field-level OneOf on
// APIObjectPostHookSchema.events still runs, so an invalid event name is
// still rejected there. abstractor.Hooks.Update validates here too, with
// the same 400 as CreateHook.
func (s *Server) PatchHook(c *gin.Context, id openapi.ObjectID) {
	data, ok := bindModel[model.Hook](c, bindOptionalJSONMap)
	if !ok {
		return
	}

	updated, err := s.svc.Hooks.Update(c.Request.Context(), id, data)
	if s.abortOnError(c, err) {
		return
	}

	c.JSON(http.StatusOK, updated)
}

// DeleteHook implements DELETE /api/v1/hooks/{id}.
func (s *Server) DeleteHook(c *gin.Context, id openapi.ObjectID) {
	if s.abortOnError(c, s.svc.Hooks.Delete(c.Request.Context(), id)) {
		return
	}

	c.Status(http.StatusNoContent)
}
