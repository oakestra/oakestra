package api

import (
	"fmt"
	"sort"

	"github.com/gin-gonic/gin"
)

// jsonKind is a coarse JSON value shape, used to check request bodies
// against the typed fields resources_blueprint.py's ResourceSchema declares
// (fields.String/Integer/Float/Boolean/Dict/List).
//
// kindInteger and kindFloat are split because they're validated
// differently: marshmallow's Integer field (verified against the pinned
// marshmallow~=3.15.0) accepts any native int or float value and
// truncates it - Integer().deserialize(5.7) returns 5, it does not
// reject the fractional part - so kindInteger truncates float64 input the
// same way rather than rejecting it. kindFloat has no such coercion since
// encoding/json's float64 is already the right shape.
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
// that don't need coercion. kindInteger is handled separately in
// validateResourceFields since accepting it also means truncating it.
func (k jsonKind) matches(v any) bool {
	switch k {
	case kindString:
		_, ok := v.(string)
		return ok
	case kindBool:
		_, ok := v.(bool)
		return ok
	case kindFloat:
		_, ok := v.(float64)
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

// resourceFields mirrors resources_blueprint.py's ResourceSchema field by
// field: JSON shape, nullability, and (for list fields) element type. Keys
// not listed here are left unvalidated, matching marshmallow's
// unknown=INCLUDE on that schema (POST/PUT/PATCH all pass through
// ResourceSchema(unknown=INCLUDE)).
//
// Every field defaults to marshmallow's allow_none=False and rejects an
// explicit JSON null with "Field may not be null." - only virtualization,
// supported_addons and csi_drivers are declared allow_none=True.
// virtualization and supported_addons are further declared
// List(fields.String()), so their elements must be strings (a null element
// included, since fields.String() also defaults allow_none=False);
// csi_drivers is declared List(fields.Raw()) instead, so its elements are
// left unvalidated.
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

// validateResourceFields checks data's recognized ResourceSchema fields
// against their expected JSON shape, returning the first mismatch found (in
// a deterministic, sorted-key order). It mutates data in place for accepted
// kindInteger fields, replacing the generic float64 encoding/json produces
// with an actual int64, matching marshmallow deserializing an Integer field
// into a native int rather than a float.
func validateResourceFields(data map[string]any) (field, message string, ok bool) {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		spec, known := resourceFields[key]
		if !known {
			continue
		}

		v := data[key]
		if v == nil {
			if spec.nullable {
				continue
			}
			return key, fmt.Sprintf("field %q may not be null", key), false
		}

		switch {
		case spec.kind == kindInteger:
			n, isNumber := v.(float64)
			if !isNumber {
				return key, fmt.Sprintf("field %q must be an integer", key), false
			}
			data[key] = int64(n)

		case spec.kind == kindList && spec.stringElements:
			list, isList := v.([]any)
			if !isList {
				return key, fmt.Sprintf("field %q must be a %s", key, spec.kind), false
			}
			for _, elem := range list {
				if _, ok := elem.(string); !ok {
					return key, fmt.Sprintf("field %q must be a list of strings", key), false
				}
			}

		default:
			if !spec.kind.matches(v) {
				return key, fmt.Sprintf("field %q must be a %s", key, spec.kind), false
			}
		}
	}
	return "", "", true
}

// abortIfInvalidResourceFields validates data against resourceFields and, on
// the first mismatch, aborts the request with the same 422 shape
// bindResourceJSONMap uses for malformed JSON (resources_blueprint.py's
// custom errorhandler(422)). Reports ok=false if the request was aborted.
func abortIfInvalidResourceFields(c *gin.Context, data map[string]any) (ok bool) {
	field, msg, valid := validateResourceFields(data)
	if valid {
		return true
	}
	abortInvalidInput(c, gin.H{"body": gin.H{field: []string{msg}}})
	return false
}
