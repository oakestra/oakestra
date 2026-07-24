// Package api implements the HTTP surface of the resource abstractor with
// gin, porting the five Flask blueprints under resource-abstractor/api/v1/.
package api

import (
	"context"
	"errors"
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
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{
			"message": "Invalid input",
			"details": gin.H{"body": err.Error()},
		})
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

// queryBool parses a query param as a bool, mirroring marshmallow's
// fields.Boolean coercion. ok is false if the key is absent or unparsable.
func queryBool(c *gin.Context, key string) (value bool, ok bool) {
	v := c.Query(key)
	if v == "" {
		return false, false
	}
	b, err := strconv.ParseBool(strings.ToLower(v))
	if err != nil {
		return false, false
	}
	return b, true
}

// queryInt parses a query param as an int. ok is false if the key is
// absent or unparsable.
func queryInt(c *gin.Context, key string) (value int, ok bool) {
	v := c.Query(key)
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
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
