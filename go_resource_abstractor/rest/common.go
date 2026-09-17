// Package rest implements the HTTP surface of the resource abstractor.
// Handlers bind requests, call abstractor.Service, and map its errors to
// status codes; the domain logic lives in package abstractor.
//
// Server implements openapi.ServerInterface, generated from
// openapi/openapi.yaml, so routes, path/query parameter binding and response
// schemas come from that spec. Request bodies don't, which is what lets
// handlers keep storing unknown fields verbatim.
package rest

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/oakestra/oakestra/go_resource_abstractor/abstractor"
	"github.com/oakestra/oakestra/go_resource_abstractor/internal/jsonutil"
	"github.com/oakestra/oakestra/go_resource_abstractor/openapi"
)

// Server bundles the dependencies HTTP handlers need: the abstractor
// service and a logger for request-failure diagnostics.
type Server struct {
	svc    *abstractor.Service
	logger *slog.Logger
}

func abortBadRequest(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusBadRequest, openapi.Message{Message: "Bad Request"})
}

func abortNotFound(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusNotFound, openapi.Message{Message: "Not Found"})
}

func (s *Server) abortInternalError(c *gin.Context, err error) {
	s.logger.Error("request failed", "path", c.Request.URL.Path, "method", c.Request.Method, "error", err)
	c.AbortWithStatusJSON(http.StatusInternalServerError, openapi.Message{Message: "Internal Server Error"})
}

// abortInvalidInput aborts with the spec's ValidationError shape
// ({"message": "Invalid input", "details": ...}), shared by every
// marshmallow-style validation failure. details is whatever shape that
// specific failure needs to report.
func abortInvalidInput(c *gin.Context, details map[string]any) {
	c.AbortWithStatusJSON(http.StatusUnprocessableEntity, openapi.ValidationError{
		Message: "Invalid input",
		Details: details,
	})
}

// abortInvalidQuery aborts with a 422 reporting field as invalid, matching
// flask-smorest's response when a query-arguments schema fails marshmallow
// validation instead of silently dropping the value.
func abortInvalidQuery(c *gin.Context, field, message string) {
	abortInvalidInput(c, map[string]any{"query": map[string]any{field: []string{message}}})
}

// abortOnError maps a service error to the matching HTTP response and
// reports whether it aborted, so callers can write
// `if s.abortOnError(c, err) { return }` instead of repeating the
// errors.Is/abort dance in every handler. A few routes need a different
// mapping for one specific error (PatchResource/GetHook's invalid id,
// AppendJobInstance's instance conflict, the custom-resource errors) and
// check for those themselves before falling back to this.
func (s *Server) abortOnError(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, abstractor.ErrNotFound):
		abortNotFound(c)
	case errors.Is(err, abstractor.ErrInvalidID):
		// A malformed id must not reach the store and come back as a raw
		// bson parse error and a 500; it's a client error, so 400 here.
		// PatchResource and GetHook check abstractor.IsValidID themselves first
		// and answer 404 instead.
		abortBadRequest(c)
	case errors.Is(err, abstractor.ErrInvalidResourceType),
		errors.Is(err, abstractor.ErrInvalidFilterKey),
		errors.Is(err, abstractor.ErrSchemaInvalid):
		c.AbortWithStatusJSON(http.StatusBadRequest, openapi.Message{Message: err.Error()})
	case errors.Is(err, abstractor.ErrInvalidHookEvent):
		abortBadRequest(c)
	default:
		s.abortInternalError(c, err)
	}
	return true
}

// errEmptyBody signals an absent/empty request body, kept distinct from
// malformed JSON so each binder can decide how to treat it.
var errEmptyBody = errors.New("empty request body")

// bindJSONMap decodes a required JSON-object body into a generic map. An
// absent/empty body, a literal null, or malformed JSON all get a 400, so a
// bodyless request can't silently create or update a document from {}. This
// matches the Python handlers that read request.json directly (apps, jobs,
// job instances).
func bindJSONMap(c *gin.Context) (map[string]any, bool) {
	data, err := decodeJSONMap(c)
	if err != nil {
		abortBadRequest(c)
		return nil, false
	}
	return data, true
}

// bindOptionalJSONMap accepts an empty body as {} (for the marshmallow
// @arguments-backed handlers: hooks, custom resources) but still rejects a
// literal null or malformed JSON with a 400.
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

// bindResourceJSONMap is the resources blueprint's binder: like
// bindOptionalJSONMap it accepts an empty body as {}, but reports a literal
// null or malformed JSON with the 422 validation shape instead of 400.
func bindResourceJSONMap(c *gin.Context) (map[string]any, bool) {
	data, err := decodeJSONMap(c)
	if errors.Is(err, errEmptyBody) {
		return map[string]any{}, true
	}
	if err != nil {
		abortInvalidInput(c, map[string]any{"body": err.Error()})
		return nil, false
	}
	return data, true
}

// decodeJSONMap decodes the request body into a generic map, staying as
// liberal as the Python service's unknown=INCLUDE schemas.
//
// Numbers go through jsonutil.Decode. Without that, Go would decode every
// number as float64, so an integer like 7 would land in MongoDB as a BSON
// double instead of the BSON integer Python stores.
//
// An empty body comes back as errEmptyBody instead of {}, and a literal
// null is treated as malformed rather than empty, so each binder above can
// apply its own policy.
func decodeJSONMap(c *gin.Context) (map[string]any, error) {
	if c.Request.Body == nil {
		return nil, errEmptyBody
	}

	var data map[string]any
	if err := jsonutil.Decode(c.Request.Body, &data); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errEmptyBody
		}
		return nil, err
	}
	if data == nil {
		// A literal JSON null decodes successfully but isn't an object;
		// Python rejects it too rather than treating it as empty.
		return nil, errors.New("request body must be a JSON object")
	}
	return data, nil
}

// decodeModel converts a bound request map into T via T's own UnmarshalJSON.
// A type mismatch on a field the model declares (e.g. a non-string
// job_name) is treated as a 400, same as invalid JSON.
func decodeModel[T any](c *gin.Context, data map[string]any) (T, bool) {
	v, err := jsonutil.Convert[T](data)
	if err != nil {
		abortBadRequest(c)
		return v, false
	}
	return v, true
}

// strOrEmpty dereferences an optional query parameter, mapping an absent one
// to "". The generated parameter structs give a nil pointer for an absent
// parameter and a pointer to "" for a present-but-empty one ("?ip="); both
// collapse to the same empty value here, matching marshmallow's optional
// filter schemas.
func strOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// booleanTruthy and booleanFalsy reproduce marshmallow's fields.Boolean
// truthy/falsy sets (pinned marshmallow~=3.15.0), lowercased since query
// values are compared case-insensitively.
var (
	booleanTruthy = map[string]bool{"1": true, "t": true, "true": true, "on": true, "y": true, "yes": true}
	booleanFalsy  = map[string]bool{"0": true, "f": true, "false": true, "off": true, "n": true, "no": true}
)

// queryBool parses a query parameter as a bool with marshmallow's
// fields.Boolean coercion rules. raw is nil when the parameter is absent,
// otherwise its literal text, including "" for "?active=" which marshmallow
// treats as present-but-invalid rather than absent. When present is true
// and err is non-nil, the caller should reject the request with a 422
// rather than dropping the filter silently; openapi.yaml types the
// parameter as a string instead of a boolean so that path stays reachable.
func queryBool(key string, raw *string) (value, present bool, err error) {
	if raw == nil {
		return false, false, nil
	}
	switch lower := strings.ToLower(*raw); {
	case booleanTruthy[lower]:
		return true, true, nil
	case booleanFalsy[lower]:
		return false, true, nil
	default:
		return false, true, fmt.Errorf("%s must be a valid boolean", key)
	}
}

// queryInt parses a query parameter as an int with marshmallow's
// fields.Integer coercion rules. raw/present/err behave as in queryBool.
// A non-numeric value should get a 422, not the 400 a generated integer
// binding would produce, so this parameter is also typed as a string in
// openapi.yaml.
func queryInt(key string, raw *string) (value int, present bool, err error) {
	if raw == nil {
		return 0, false, nil
	}
	n, err := strconv.Atoi(*raw)
	if err != nil {
		return 0, true, fmt.Errorf("%s must be a valid integer", key)
	}
	return n, true, nil
}
