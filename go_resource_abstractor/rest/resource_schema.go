package rest

import (
	"fmt"
	"maps"
	"slices"

	"github.com/gin-gonic/gin"
)

// jsonKind is a coarse JSON value shape, used to check request bodies
// against the typed fields ResourceSchema declares
// (fields.String/Integer/Float/Boolean/Dict/List). kindInteger and
// kindFloat are validated and coerced separately, since marshmallow's
// Integer/Float fields both accept either JSON representation.
//
// This validation stays here instead of relying on model.Resource's typed
// decode so we can reproduce resources_blueprint.py's exact 422 messages
// (`field "x" must be an integer`, etc); a decode error from the typed
// model has no marshmallow-shaped equivalent.
type jsonKind int

const (
	kindString jsonKind = iota
	kindBool
	kindInteger
	kindFloat
	kindDict
	kindList
)

func (k jsonKind) String() string {
	switch k {
	case kindString:
		return "string"
	case kindBool:
		return "boolean"
	case kindInteger:
		return "integer"
	case kindFloat:
		return "number"
	case kindDict:
		return "object"
	case kindList:
		return "array"
	default:
		return "value"
	}
}

// matches reports whether v is directly acceptable for k, for the kinds
// that don't need coercion. kindInteger and kindFloat are handled
// separately in validateResourceFields.
func (k jsonKind) matches(v any) bool {
	switch k {
	case kindString:
		_, ok := v.(string)
		return ok
	case kindBool:
		_, ok := v.(bool)
		return ok
	case kindDict:
		_, ok := v.(map[string]any)
		return ok
	case kindList:
		_, ok := v.([]any)
		return ok
	default:
		return true
	}
}

// resourceFieldSpec describes one ResourceSchema field: its JSON shape,
// whether it's declared allow_none=True, and (for kindList fields) whether
// its elements must themselves be strings.
type resourceFieldSpec struct {
	kind           jsonKind
	nullable       bool
	stringElements bool // only meaningful when kind == kindList
}

// resourceFields covers ResourceSchema field by field: JSON shape,
// nullability, and (for list fields) element type. Keys not listed here are
// left unvalidated, matching marshmallow's unknown=INCLUDE on that schema.
//
// Every field defaults to rejecting an explicit null; only virtualization,
// supported_addons and csi_drivers allow it. virtualization and
// supported_addons require string elements; csi_drivers elements are left
// unvalidated.
var resourceFields = map[string]resourceFieldSpec{
	"_id":                          {kind: kindString},
	"candidate_name":               {kind: kindString},
	"candidate_location":           {kind: kindString},
	"ip":                           {kind: kindString},
	"port":                         {kind: kindString},
	"active_nodes":                 {kind: kindInteger},
	"active":                       {kind: kindBool},
	"memory":                       {kind: kindInteger},
	"vcpus":                        {kind: kindInteger},
	"vgpus":                        {kind: kindInteger},
	"cpu_percent":                  {kind: kindFloat},
	"aggregation_per_architecture": {kind: kindDict},
	"memory_percent":               {kind: kindFloat},
	"gpu_percent":                  {kind: kindInteger},
	"virtualization":               {kind: kindList, nullable: true, stringElements: true},
	"supported_addons":             {kind: kindList, nullable: true, stringElements: true},
	"csi_drivers":                  {kind: kindList, nullable: true},
	"architecture":                 {kind: kindString},
	"last_modified_timestamp":      {kind: kindFloat},
}

// validationError reports a single field-level validation failure.
// abortIfInvalidResourceFields renders it as the API's
// {"message": "Invalid input", "details": {...}} shape, which needs Field
// and Message apart, so this deliberately isn't an error type.
type validationError struct {
	Field   string
	Message string
}

// validateResourceFields checks data's recognized ResourceSchema fields
// against their expected JSON shape, returning the first mismatch found (in
// sorted-key order for determinism), or nil if every recognized field
// matches. For accepted kindInteger fields it also mutates data in place,
// replacing the float64 encoding/json produced with an int64.
func validateResourceFields(data map[string]any) *validationError {
	for _, key := range slices.Sorted(maps.Keys(data)) {
		spec, known := resourceFields[key]
		if !known {
			continue
		}

		v := data[key]
		if v == nil {
			if spec.nullable {
				continue
			}
			return &validationError{Field: key, Message: fmt.Sprintf("field %q may not be null", key)}
		}

		switch {
		case spec.kind == kindInteger:
			n, ok := toInt64(v)
			if !ok {
				return &validationError{Field: key, Message: fmt.Sprintf("field %q must be an integer", key)}
			}
			data[key] = n

		case spec.kind == kindFloat:
			f, ok := toFloat64(v)
			if !ok {
				return &validationError{Field: key, Message: fmt.Sprintf("field %q must be a %s", key, spec.kind)}
			}
			data[key] = f

		case spec.kind == kindList && spec.stringElements:
			list, isList := v.([]any)
			if !isList {
				return &validationError{Field: key, Message: fmt.Sprintf("field %q must be a %s", key, spec.kind)}
			}
			for _, elem := range list {
				if _, ok := elem.(string); !ok {
					return &validationError{Field: key, Message: fmt.Sprintf("field %q must be a list of strings", key)}
				}
			}

		default:
			if !spec.kind.matches(v) {
				return &validationError{Field: key, Message: fmt.Sprintf("field %q must be a %s", key, spec.kind)}
			}
		}
	}
	return nil
}

// toInt64 accepts either a decoded JSON integer (int64) or decimal (float64)
// and returns it as an int64, truncating the fractional part, matching
// marshmallow's Integer coercion. Any other type is rejected.
func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

// toFloat64 accepts either a decoded JSON decimal (float64) or integer
// (int64) and returns it as a float64, matching how marshmallow's Float
// field widens an int. Any other type is rejected.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

// abortIfInvalidResourceFields validates data against resourceFields and, on
// the first mismatch, aborts the request with the same 422 shape
// bindResourceJSONMap uses for malformed JSON. Reports whether it aborted,
// so callers can write `if abortIfInvalidResourceFields(c, body) { return }`.
func abortIfInvalidResourceFields(c *gin.Context, data map[string]any) bool {
	fieldErr := validateResourceFields(data)
	if fieldErr == nil {
		return false
	}
	abortInvalidInput(c, map[string]any{"body": map[string]any{fieldErr.Field: []string{fieldErr.Message}}})
	return true
}
