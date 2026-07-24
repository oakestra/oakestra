package api

import (
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"

	"go_resource_abstractor/db"
)

// registerHookRoutes wires up /api/v1/hooks, mirroring hooks_blueprint.py.
// Not used by the scheduler or root/cluster managers directly, but consumed
// by the addons engine to register its webhooks.
func (s *Server) registerHookRoutes(v1 *gin.RouterGroup) {
	group := v1.Group("/hooks")

	bothSlashes(group, http.MethodGet, s.listHooks)
	bothSlashes(group, http.MethodPost, s.createHook)

	itemBothSlashes(group, http.MethodGet, "/:id", s.getHook)
	itemBothSlashes(group, http.MethodPatch, "/:id", s.patchHook)
	itemBothSlashes(group, http.MethodDelete, "/:id", s.deleteHook)
}

// listHooks implements GET /hooks/.
func (s *Server) listHooks(c *gin.Context) {
	hooks, err := s.store.FindHooks(c.Request.Context(), nil)
	if err != nil {
		abortInternalError(c, err)
		return
	}
	writeJSON(c, http.StatusOK, hooks)
}

// createHook implements POST /hooks/. events entries are validated against
// the known async/sync event names, mirroring the OneOf validator on
// APIObjectPostHookSchema.events.
func (s *Server) createHook(c *gin.Context) {
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

// getHook implements GET /hooks/<id>.
func (s *Server) getHook(c *gin.Context) {
	id := c.Param("id")
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

// patchHook implements PATCH /hooks/<id>.
//
// SingleHookController.patch passes validate=False to flask-smorest's
// arguments decorator, but that only skips schema-level (whole-object)
// validators - the field-level OneOf validator on
// APIObjectPostHookSchema.events still runs during marshmallow's load, so an
// invalid event name is still rejected there. Validated here to match, using
// the same 400 createHook uses.
func (s *Server) patchHook(c *gin.Context) {
	id := c.Param("id")
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

// deleteHook implements DELETE /hooks/<id>.
func (s *Server) deleteHook(c *gin.Context) {
	id := c.Param("id")
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

// validHookEvents checks that every entry in data["events"] (if present) is
// one of db.AllEvents.
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
		if !ok || !slices.Contains(db.AllEvents, db.HookEvent(name)) {
			return false
		}
	}
	return true
}
