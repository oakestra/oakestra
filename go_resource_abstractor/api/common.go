// Package api implements the HTTP surface of the resource abstractor with
// gin, porting the five Flask blueprints under resource-abstractor/api/v1/.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// errEmptyBody signals an absent/empty request body, kept distinct from
// malformed JSON so each binder can decide how to treat it. The Python
// service is itself split: handlers that read request.json directly (apps,
// jobs, job instances) reject an empty body, while those backed by a
// marshmallow @arguments schema (resources, hooks, custom resources) have
// webargs load a missing body as an empty mapping and accept it as {}.
var errEmptyBody = errors.New("empty request body")

// bindJSONMap decodes a required JSON-object request body into a generic map,
// mirroring the Python handlers that read request.json directly (apps, jobs,
// job instances): an absent/empty body, a literal null, or malformed JSON is
// rejected with 400, so a bodyless request can't silently create or update a
// document from {}.
func bindJSONMap(c *gin.Context) (map[string]any, bool) {
	data, err := decodeJSONMap(c)
	if err != nil {
		abortBadRequest(c)
		return nil, false
	}
	return data, true
}

// bindOptionalJSONMap is bindJSONMap's counterpart for handlers the Python
// service backs with a marshmallow @arguments schema (hooks, custom
// resources): webargs loads an absent/empty body as an empty mapping, so an
// empty body is accepted as {} here rather than rejected. A literal null or
// otherwise malformed JSON is still a 400.
func bindOptionalJSONMap(c *gin.Context) (map[string]any, bool) {
	data, err := decodeJSONMap(c)
	if errors.Is(err, errEmptyBody) {
		return map[string]any{}, true
	}
	if err != nil {
		abortBadRequest(c)
		return nil, false
	}
	return data, true
}

// bindResourceJSONMap is the resources blueprint's counterpart: like
// bindOptionalJSONMap it accepts an empty body as {} (ResourceSchema is an
// @arguments schema), but it reports a literal null or malformed JSON with
// the custom 422 shape (@resourcesblp.errorhandler(422)) rather than 400.
func bindResourceJSONMap(c *gin.Context) (map[string]any, bool) {
	data, err := decodeJSONMap(c)
	if errors.Is(err, errEmptyBody) {
		return map[string]any{}, true
	}
	if err != nil {
		abortInvalidInput(c, gin.H{"body": err.Error()})
		return nil, false
	}
	return data, true
}

// decodeJSONMap decodes the request body into a generic map, matching the
// Python service's liberal (unknown=INCLUDE) schemas.
//
// Two details keep it compatible with the Python service:
//
//   - Numbers are decoded via json.Number and then resolved to an int64 when
//     they have no fractional/exponent part, else a float64 (see
//     resolveJSONNumber). Go's encoding/json would otherwise decode every
//     JSON number to float64, so an integer like 7 would land in MongoDB as a
//     BSON double where Python's json - and therefore pymongo - stores a BSON
//     integer. Consumers and BSON round-trips are sensitive to that type
//     difference; the resources blueprint's typed fields hid it, but jobs,
//     apps, custom resources and unknown fields are stored verbatim.
//   - An empty body is reported as errEmptyBody (not silently as {}), and a
//     literal JSON null is malformed rather than an empty object, so the
//     binders can apply each endpoint's Python-matching policy (see
//     bindJSONMap / bindOptionalJSONMap).
func decodeJSONMap(c *gin.Context) (map[string]any, error) {
	if c.Request.Body == nil {
		return nil, errEmptyBody
	}
	dec := json.NewDecoder(c.Request.Body)
	dec.UseNumber()

	var data map[string]any
	if err := dec.Decode(&data); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errEmptyBody
		}
		return nil, err
	}
	if data == nil {
		// A literal JSON null: present but not an object. Python's schema
		// load and its insert path both reject it, so treat it as malformed
		// rather than as an empty object.
		return nil, errors.New("request body must be a JSON object")
	}
	resolveJSONNumbers(data)
	return data, nil
}

// resolveJSONNumbers walks a decoded body in place, replacing every
// json.Number with an int64 (integer form) or float64 (fractional/exponent
// form), mirroring how Python's json.loads yields int vs float. See
// decodeJSONMap for why this matters.
func resolveJSONNumbers(m map[string]any) {
	for k, v := range m {
		m[k] = resolveJSONNumber(v)
	}
}

func resolveJSONNumber(v any) any {
	switch val := v.(type) {
	case map[string]any:
		resolveJSONNumbers(val)
		return val
	case []any:
		for i, e := range val {
			val[i] = resolveJSONNumber(e)
		}
		return val
	case json.Number:
		s := val.String()
		if !strings.ContainsAny(s, ".eE") {
			if i, err := strconv.ParseInt(s, 10, 64); err == nil {
				return i
			}
		}
		if f, err := val.Float64(); err == nil {
			return f
		}
		return s
	default:
		return v
	}
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
