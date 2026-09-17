package jsonutil

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestResolveNumbers(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want any
	}{
		{
			name: "integer",
			in:   json.Number("7"),
			want: int64(7),
		},
		{
			name: "float",
			in:   json.Number("7.5"),
			want: float64(7.5),
		},
		{
			name: "exponent",
			in:   json.Number("1e3"),
			want: float64(1000),
		},
		{
			name: "nested map",
			in: map[string]any{
				"a": json.Number("1"),
				"b": map[string]any{"c": json.Number("2.5")},
			},
			want: map[string]any{
				"a": int64(1),
				"b": map[string]any{"c": float64(2.5)},
			},
		},
		{
			name: "nested slice",
			in:   []any{json.Number("1"), json.Number("2.5"), []any{json.Number("3")}},
			want: []any{int64(1), float64(2.5), []any{int64(3)}},
		},
		{
			name: "non-number passthrough",
			in:   "hello",
			want: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveNumbers(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ResolveNumbers(%#v) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}
