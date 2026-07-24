// Package api implements the HTTP surface of the resource abstractor with
// gin, porting the five Flask blueprints under resource-abstractor/api/v1/.
package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"go_resource_abstractor/db"
	"go_resource_abstractor/services"
)

// Server bundles the dependencies HTTP handlers need: the Mongo-backed
// store and the webhook dispatcher.
type Server struct {
	store *db.Store
	hooks *services.Hooks
}

// isValidObjectID reports whether id is a valid hex-encoded ObjectID,
// mirroring the ObjectId.is_valid checks scattered through the Python
// blueprints.
func isValidObjectID(id string) bool {
	_, err := bson.ObjectIDFromHex(id)
	return err == nil
}

// isNotFound reports whether err represents a "no matching document"
// result. db.ErrNotFound is itself an alias for mongo.ErrNoDocuments, so a
// single comparison covers every db function that can return either.
func isNotFound(err error) bool {
	return errors.Is(err, mongo.ErrNoDocuments)
}

func abortBadRequest(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"message": "Bad Request"})
}

func abortNotFound(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"message": "Not Found"})
}

func abortInternalError(c *gin.Context, err error) {
	slog.Error("request failed", "path", c.Request.URL.Path, "method", c.Request.Method, "error", err)
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"message": "Internal Server Error"})
}

// abortInvalidInput aborts the request with resources_blueprint.py's
// errorhandler(422) shape ({"message": "Invalid input", "details": ...}),
// shared by every validation-failure path in this package: malformed JSON
// bodies (bindResourceJSONMap), invalid query params (abortInvalidQuery),
// and invalid resource body fields (abortIfInvalidResourceFields). details
// is whatever shape that specific validation failure needs to report.
func abortInvalidInput(c *gin.Context, details gin.H) {
	c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{
		"message": "Invalid input",
		"details": details,
	})
}

// abortInvalidQuery aborts the request with a 422 reporting field as
// invalid, matching flask-smorest's default response when a query-arguments
// schema fails marshmallow validation (e.g. JobFilterSchema.instance_number,
// ResourceFilterSchema.active) instead of the value being silently dropped.
func abortInvalidQuery(c *gin.Context, field, message string) {
	abortInvalidInput(c, gin.H{"query": gin.H{field: []string{message}}})
}

// abortOnError maps a store error to the matching HTTP response - 404 via
// abortNotFound, anything else via abortInternalError - and reports whether
// it aborted the request, so callers can write `if abortOnError(c, err) {
// return }` instead of the isNotFound/abortNotFound/abortInternalError
// three-step repeated across every handler.
func abortOnError(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false
	case isNotFound(err):
		abortNotFound(c)
	default:
		abortInternalError(c, err)
	}
	return true
}

// writeJSON normalizes v (translating ObjectIDs/dates to plain strings, see
// db.Normalize) and writes it as the JSON response body.
func writeJSON(c *gin.Context, status int, v any) {
	c.JSON(status, db.Normalize(v))
}

// bindJSONMap decodes the request body into a generic map, matching the
// Python service's liberal (unknown=INCLUDE) schemas. On malformed JSON it
// aborts the request with 400.
func bindJSONMap(c *gin.Context) (map[string]any, bool) {
	data, err := decodeJSONMap(c)
	if err != nil {
		abortBadRequest(c)
		return nil, false
	}
	return data, true
}

// bindResourceJSONMap is bindJSONMap's counterpart for the resources
// blueprint, which registers a custom 422 error handler
// (@resourcesblp.errorhandler(422)) returning
// {"message": "Invalid input", "details": {...}} instead of a plain 400.
func bindResourceJSONMap(c *gin.Context) (map[string]any, bool) {
	data, err := decodeJSONMap(c)
	if err != nil {
		abortInvalidInput(c, gin.H{"body": err.Error()})
		return nil, false
	}
	return data, true
}

func decodeJSONMap(c *gin.Context) (map[string]any, error) {
	var data map[string]any
	if c.Request.ContentLength == 0 {
		return map[string]any{}, nil
	}
	if err := c.ShouldBindJSON(&data); err != nil {
		return nil, err
	}
	if data == nil {
		data = map[string]any{}
	}
	return data, nil
}

// queryFilter builds a filter map containing only the given allowed query
// keys that are actually present, mirroring marshmallow's optional filter
// schemas (e.g. ResourceFilterSchema, ApplicationFilterSchema).
func queryFilter(c *gin.Context, keys ...string) map[string]any {
	filter := map[string]any{}
	for _, k := range keys {
		if v := c.Query(k); v != "" {
			filter[k] = v
		}
	}
	return filter
}

// booleanTruthy and booleanFalsy mirror marshmallow's fields.Boolean truthy/
// falsy sets (verified against the pinned marshmallow~=3.15.0: {"1", "t",
// "true", "on", "y", "yes"} and their case variants, and the false
// counterparts), lowercased here since query values are compared
// case-insensitively.
var (
	booleanTruthy = map[string]bool{"1": true, "t": true, "true": true, "on": true, "y": true, "yes": true}
	booleanFalsy  = map[string]bool{"0": true, "f": true, "false": true, "off": true, "n": true, "no": true}
)

// queryBool parses a query param as a bool, mirroring marshmallow's
// fields.Boolean coercion. present reports whether the key was supplied at
// all - including with an empty value (e.g. "?active="), which
// marshmallow also treats as present-but-invalid rather than absent, hence
// c.GetQuery (which distinguishes "absent" from "present but empty") rather
// than c.Query (which conflates the two). If present is true and err is
// non-nil, the value isn't a valid bool and the caller should reject the
// request (422, via abortInvalidQuery) rather than silently drop the
// filter, matching the validation ResourceFilterSchema applies to ?active=.
func queryBool(c *gin.Context, key string) (value, present bool, err error) {
	v, exists := c.GetQuery(key)
	if !exists {
		return false, false, nil
	}
	lower := strings.ToLower(v)
	switch {
	case booleanTruthy[lower]:
		return true, true, nil
	case booleanFalsy[lower]:
		return false, true, nil
	default:
		return false, true, fmt.Errorf("%s must be a valid boolean", key)
	}
}

// queryInt parses a query param as an int, mirroring marshmallow's
// fields.Integer coercion. present/err behave as in queryBool (including
// using c.GetQuery so "?instance_number=" - present but empty - isn't
// silently treated as absent), matching the validation JobFilterSchema
// applies to ?instance_number=.
func queryInt(c *gin.Context, key string) (value int, present bool, err error) {
	v, exists := c.GetQuery(key)
	if !exists {
		return 0, false, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, true, fmt.Errorf("%s must be a valid integer", key)
	}
	return n, true, nil
}

// upsertByName implements the find-by-name-or-create control flow shared by
// PUT /jobs/ and PUT /resources/: update the existing document if lookup by
// nameField succeeds, otherwise create a new one. entity names the hook
// channel to fire PreCreate/PreUpdate/PostCreate/PostUpdate against.
func (s *Server) upsertByName(
	c *gin.Context,
	entity, nameField string,
	data map[string]any,
	findByName func(ctx context.Context, name string) (bson.M, error),
	update func(ctx context.Context, id string, data bson.M) (bson.M, error),
	create func(ctx context.Context, data bson.M) (bson.M, error),
) {
	ctx := c.Request.Context()

	if name, _ := data[nameField].(string); name != "" {
		existing, err := findByName(ctx, name)
		switch {
		case err == nil:
			id := db.ExtractID(existing)
			data = s.hooks.PreUpdate(ctx, entity, data)

			updated, err := update(ctx, id, bson.M(data))
			if abortOnError(c, err) {
				return
			}
			s.hooks.PostUpdate(entity, db.ExtractID(updated))
			writeJSON(c, http.StatusOK, updated)
			return
		case !isNotFound(err):
			abortInternalError(c, err)
			return
		}
	}

	data = s.hooks.PreCreate(ctx, entity, data)
	created, err := create(ctx, data)
	if abortOnError(c, err) {
		return
	}
	s.hooks.PostCreate(entity, db.ExtractID(created))
	writeJSON(c, http.StatusOK, created)
}
