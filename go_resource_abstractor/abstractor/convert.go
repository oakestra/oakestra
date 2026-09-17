package abstractor

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/jsonutil"
)

// toMap converts a model type into a map[string]any for hook dispatch,
// using the type's own MarshalJSON. Hooks operate on maps, not model types,
// since a sync hook's payload is whatever the caller is about to write,
// before it's known to be valid against any model.
//
// Decoding goes through jsonutil.Decode so a value keeps its int-vs-float
// identity across the round trip instead of collapsing to float64.
func toMap(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal to map: %w", err)
	}

	var m map[string]any
	if err := jsonutil.Decode(bytes.NewReader(raw), &m); err != nil {
		return nil, fmt.Errorf("decode to map: %w", err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// fromMap converts a hook payload map back into T via T's UnmarshalJSON,
// after a sync hook has had its chance to transform the payload.
func fromMap[T any](m map[string]any) (T, error) {
	return jsonutil.Convert[T](m)
}

// idFromMap pulls the stringified _id out of a hook payload map. It's
// always a plain string there, never an ObjectID or other driver type,
// since the map came from a model type's own MarshalJSON.
func idFromMap(m map[string]any) string {
	id, _ := m["_id"].(string)
	return id
}
