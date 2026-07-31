package db

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Normalize recursively rewrites a decoded BSON value so it JSON-marshals
// like the Python service's responses do: ObjectId and datetime values as
// plain strings, not MongoDB's Extended JSON ({"$oid": ...} / {"$date": ...})
// that the Go driver would otherwise produce.
//
// Apply this to every document before writing a gin response - it's what
// keeps the wire format compatible with consumers (the Go scheduler, the
// Python resource_abstractor_client) that expect `_id` as a bare string.
func Normalize(v any) any {
	switch val := v.(type) {
	case bson.ObjectID:
		return val.Hex()
	case bson.DateTime:
		return val.Time().UTC().Format(time.RFC3339Nano)
	case time.Time:
		return val.UTC().Format(time.RFC3339Nano)
	case bson.M:
		return normalizeMap(val)
	case bson.D:
		out := make(map[string]any, len(val))
		for _, e := range val {
			out[e.Key] = Normalize(e.Value)
		}
		return out
	case map[string]any:
		return normalizeMap(val)
	case bson.A:
		return NormalizeSlice([]any(val))
	case []any:
		return NormalizeSlice(val)
	case []bson.M:
		return NormalizeSlice(val)
	default:
		return v
	}
}

// normalizeMap applies Normalize to every value of a decoded document map,
// shared by the bson.M and map[string]any cases in Normalize above.
func normalizeMap[T any](m map[string]T) map[string]any {
	out := make(map[string]any, len(m))
	for k, elem := range m {
		out[k] = Normalize(elem)
	}
	return out
}

// NormalizeSlice applies Normalize to every element of a document slice,
// used for list endpoints (e.g. GET /resources/, GET /jobs/).
func NormalizeSlice[T any](docs []T) []any {
	out := make([]any, len(docs))
	for i, d := range docs {
		out[i] = Normalize(d)
	}
	return out
}

// ExtractID pulls a stringified _id out of a decoded document, accepting
// either a driver-native bson.ObjectID or an already-stringified value.
// Used to derive the entity_id passed to post-write webhooks.
func ExtractID(doc bson.M) string {
	switch id := doc["_id"].(type) {
	case bson.ObjectID:
		return id.Hex()
	case string:
		return id
	default:
		return ""
	}
}
