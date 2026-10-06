// Package jsonutil holds the JSON decoding rule shared by model, abstractor
// and rest: numbers decode as json.Number and resolve to int64 when they
// have no fractional or exponent part, else float64. A plain json.Unmarshal
// into any would turn every number into float64, so an integer like 7 would
// land in MongoDB as a double where the Python service stores an integer.
package jsonutil

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Decode reads one JSON value from r into v with numbers resolved through
// ResolveNumbers. v is expected to be a *map[string]any or *any; any other
// target decodes normally, since a typed destination has no json.Number to
// resolve.
func Decode(r io.Reader, v any) error {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return err
	}
	switch p := v.(type) {
	case *map[string]any:
		ResolveNumbers(*p)
	case *any:
		*p = ResolveNumbers(*p)
	}
	return nil
}

// Convert re-decodes v into T through a JSON round trip, so T's own
// UnmarshalJSON applies. encoding/json keeps int64 vs float64 identity
// across Marshal/Unmarshal, so a map produced by Decode survives intact.
func Convert[T any](v any) (T, error) {
	var out T
	raw, err := json.Marshal(v)
	if err != nil {
		return out, fmt.Errorf("marshal %T: %w", v, err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		var zero T
		return zero, fmt.Errorf("decode into %T: %w", out, err)
	}
	return out, nil
}

// ResolveNumbers walks v in place (descending into maps and slices),
// replacing every json.Number with an int64 or float64, and returns the
// resolved value.
func ResolveNumbers(v any) any {
	switch val := v.(type) {
	case map[string]any:
		for k, e := range val {
			val[k] = ResolveNumbers(e)
		}
		return val
	case []any:
		for i, e := range val {
			val[i] = ResolveNumbers(e)
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
