package model

import (
	"encoding/json"
	"math"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestFloatUnmarshalJSON covers the shapes Float must accept: a JSON number,
// a numeric string (what NodeEngine/cluster_manager actually send), and the
// invalid inputs that must be rejected instead of silently becoming 0.
func TestFloatUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Float
		wantErr bool
	}{
		{"number", `55.5`, 55.5, false},
		{"integral number", `7`, 7, false},
		{"numeric string", `"55.500000"`, 55.5, false},
		{"integral numeric string", `"7"`, 7, false},
		{"non-numeric string", `"not-a-number"`, 0, true},
		{"empty string", `""`, 0, true},
		{"bool", `true`, 0, true},
		// strconv.ParseFloat happily parses these, but encoding/json can't
		// marshal a non-finite float, so they must be rejected on decode
		// rather than fail later on whatever response includes this field.
		{"NaN string", `"NaN"`, 0, true},
		{"Inf string", `"Inf"`, 0, true},
		{"-Infinity string", `"-Infinity"`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f Float
			err := json.Unmarshal([]byte(tt.input), &f)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error decoding %s, got f = %v", tt.input, f)
				}
				return
			}
			if err != nil {
				t.Fatalf("unmarshal %s: %v", tt.input, err)
			}
			if f != tt.want {
				t.Errorf("f = %v, want %v", f, tt.want)
			}
		})
	}
}

// TestIntUnmarshalJSON covers the same shapes as TestFloatUnmarshalJSON plus
// the integral check host_port needs: a float-formatted string or number is
// fine as long as it has no fractional part.
func TestIntUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Int
		wantErr bool
	}{
		{"integer", `50011`, 50011, false},
		{"integral float", `50011.0`, 50011, false},
		{"numeric string", `"50011"`, 50011, false},
		{"integral float string", `"50011.0"`, 50011, false},
		{"non-integral float", `50011.5`, 0, true},
		{"non-integral string", `"50011.5"`, 0, true},
		{"non-numeric string", `"not-a-port"`, 0, true},
		{"empty string", `""`, 0, true},
		{"NaN string", `"NaN"`, 0, true},
		{"Inf string", `"Inf"`, 0, true},
		{"-Infinity string", `"-Infinity"`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var i Int
			err := json.Unmarshal([]byte(tt.input), &i)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error decoding %s, got i = %v", tt.input, i)
				}
				return
			}
			if err != nil {
				t.Fatalf("unmarshal %s: %v", tt.input, err)
			}
			if i != tt.want {
				t.Errorf("i = %v, want %v", i, tt.want)
			}
		})
	}
}

// TestFloatMarshalsAsJSONNumber checks that Float always re-encodes as a
// plain JSON number, regardless of whether it was decoded from a number or
// a string - the normalization the model package promises on rewrite.
func TestFloatMarshalsAsJSONNumber(t *testing.T) {
	var f Float
	if err := json.Unmarshal([]byte(`"55.500000"`), &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(out) != "55.5" {
		t.Errorf("marshaled = %s, want a bare number 55.5", out)
	}
}

// bsonValue builds the (type, data) pair UnmarshalBSONValue receives, using
// the driver's own MarshalValue so the encoding matches what a real
// document would contain.
func bsonValue(t *testing.T, v any) (byte, []byte) {
	t.Helper()
	typ, data, err := bson.MarshalValue(v)
	if err != nil {
		t.Fatalf("bson.MarshalValue(%#v): %v", v, err)
	}
	return byte(typ), data
}

// TestFloatUnmarshalBSONValue covers every BSON numeric representation Float
// must accept - double, int32, int64, decimal128 - plus the string
// representation the Python service actually wrote, and rejects a
// non-numeric string instead of decoding it as 0.
func TestFloatUnmarshalBSONValue(t *testing.T) {
	dec128, err := bson.ParseDecimal128("12.5")
	if err != nil {
		t.Fatalf("ParseDecimal128: %v", err)
	}

	tests := []struct {
		name    string
		value   any
		want    Float
		wantErr bool
	}{
		{"double", float64(12.5), 12.5, false},
		{"int32", int32(7), 7, false},
		{"int64", int64(7), 7, false},
		{"decimal128", dec128, 12.5, false},
		{"string", "12.500000", 12.5, false},
		{"non-numeric string", "not-a-number", 0, true},
		// A BSON double can hold NaN/Inf directly (an IEEE-754 double
		// always can), and the string/decimal128 paths go through
		// strconv.ParseFloat, which parses these tokens too - all three
		// must be rejected the same way a malformed number is.
		{"NaN double", math.NaN(), 0, true},
		{"+Inf double", math.Inf(1), 0, true},
		{"-Inf double", math.Inf(-1), 0, true},
		{"NaN string", "NaN", 0, true},
		{"Infinity string", "Infinity", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typ, data := bsonValue(t, tt.value)
			var f Float
			err := f.UnmarshalBSONValue(typ, data)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got f = %v", f)
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalBSONValue: %v", err)
			}
			if f != tt.want {
				t.Errorf("f = %v, want %v", f, tt.want)
			}
		})
	}
}

// TestIntUnmarshalBSONValue covers the same BSON shapes as
// TestFloatUnmarshalBSONValue, plus the integral check: a BSON double or
// decimal128 or string with a fractional part must be rejected, not
// truncated.
func TestIntUnmarshalBSONValue(t *testing.T) {
	dec128, err := bson.ParseDecimal128("50011")
	if err != nil {
		t.Fatalf("ParseDecimal128: %v", err)
	}

	tests := []struct {
		name    string
		value   any
		want    Int
		wantErr bool
	}{
		{"int32", int32(50011), 50011, false},
		{"int64", int64(50011), 50011, false},
		{"integral double", float64(50011), 50011, false},
		{"non-integral double", float64(50011.5), 0, true},
		{"decimal128", dec128, 50011, false},
		{"string", "50011", 50011, false},
		{"non-integral string", "50011.5", 0, true},
		{"NaN double", math.NaN(), 0, true},
		{"+Inf double", math.Inf(1), 0, true},
		{"NaN string", "NaN", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typ, data := bsonValue(t, tt.value)
			var i Int
			err := i.UnmarshalBSONValue(typ, data)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got i = %v", i)
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalBSONValue: %v", err)
			}
			if i != tt.want {
				t.Errorf("i = %v, want %v", i, tt.want)
			}
		})
	}
}

// TestJobInstanceDecodesStringNumbersFromJSON checks the actual bug report:
// a PATCH body with cpu_percent/memory_percent/disk/host_port as strings -
// what NodeEngine and cluster_manager send - must decode instead of 400ing.
func TestJobInstanceDecodesStringNumbersFromJSON(t *testing.T) {
	input := []byte(`{
		"cpu_percent": "12.345600",
		"memory_percent": "34.560000",
		"disk": "0",
		"host_port": "50011"
	}`)

	var instance JobInstance
	if err := json.Unmarshal(input, &instance); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if instance.CPUPercent == nil || *instance.CPUPercent != Float(12.3456) {
		t.Errorf("cpu_percent = %v, want 12.3456", instance.CPUPercent)
	}
	if instance.MemoryPercent == nil || *instance.MemoryPercent != Float(34.56) {
		t.Errorf("memory_percent = %v, want 34.56", instance.MemoryPercent)
	}
	if instance.Disk == nil || *instance.Disk != Float(0) {
		t.Errorf("disk = %v, want 0", instance.Disk)
	}
	if instance.HostPort == nil || *instance.HostPort != Int(50011) {
		t.Errorf("host_port = %v, want 50011", instance.HostPort)
	}

	// Re-marshaling normalizes every field back to a plain number.
	out, err := json.Marshal(instance)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("unmarshal to raw map: %v", err)
	}
	for _, key := range []string{"cpu_percent", "memory_percent", "disk", "host_port"} {
		raw, ok := decoded[key]
		if !ok {
			t.Fatalf("marshaled output missing %q", key)
		}
		if raw[0] == '"' {
			t.Errorf("%s marshaled as a string (%s), want a bare number", key, raw)
		}
	}
}

// TestJobInstanceDecodesStringNumbersFromBSON simulates a document the
// Python service wrote: cpu_percent/memory_percent/disk/host_port and a
// cpu_history entry's value stored as strings. bson.Unmarshal into
// JobInstance must succeed, and re-marshaling must normalize every field
// back to a BSON number - the read-patch-write cycle that used to 500.
func TestJobInstanceDecodesStringNumbersFromBSON(t *testing.T) {
	raw := bson.D{
		{Key: "cpu_percent", Value: "12.345600"},
		{Key: "memory_percent", Value: "34.560000"},
		{Key: "disk", Value: "0"},
		{Key: "host_port", Value: "50011"},
		{Key: "cpu_history", Value: bson.A{
			bson.D{{Key: "value", Value: "12.345600"}, {Key: "timestamp", Value: "2024-01-01T00:00:00.000000+00:00"}},
		}},
	}
	data, err := bson.Marshal(raw)
	if err != nil {
		t.Fatalf("bson marshal raw doc: %v", err)
	}

	var instance JobInstance
	if err := bson.Unmarshal(data, &instance); err != nil {
		t.Fatalf("bson unmarshal: %v", err)
	}

	if instance.CPUPercent == nil || *instance.CPUPercent != Float(12.3456) {
		t.Errorf("cpu_percent = %v, want 12.3456", instance.CPUPercent)
	}
	if instance.MemoryPercent == nil || *instance.MemoryPercent != Float(34.56) {
		t.Errorf("memory_percent = %v, want 34.56", instance.MemoryPercent)
	}
	if instance.Disk == nil || *instance.Disk != Float(0) {
		t.Errorf("disk = %v, want 0", instance.Disk)
	}
	if instance.HostPort == nil || *instance.HostPort != Int(50011) {
		t.Errorf("host_port = %v, want 50011", instance.HostPort)
	}
	if instance.CPUHistory == nil || len(*instance.CPUHistory) != 1 {
		t.Fatalf("cpu_history = %+v, want one sample", instance.CPUHistory)
	}
	if v := (*instance.CPUHistory)[0].Value; v == nil || *v != Float(12.3456) {
		t.Errorf("cpu_history[0].value = %v, want 12.3456", v)
	}

	// Re-marshal and inspect the raw document: every numeric field must
	// come back as a BSON number, not the string the source document held.
	normalized, err := bson.Marshal(instance)
	if err != nil {
		t.Fatalf("bson marshal normalized: %v", err)
	}
	var doc bson.M
	if err := bson.Unmarshal(normalized, &doc); err != nil {
		t.Fatalf("bson unmarshal to raw doc: %v", err)
	}
	for _, key := range []string{"cpu_percent", "memory_percent", "disk"} {
		if _, ok := doc[key].(string); ok {
			t.Errorf("%s stored as a string after re-marshal, want a number", key)
		}
	}
	if _, ok := doc["host_port"].(string); ok {
		t.Error("host_port stored as a string after re-marshal, want a number")
	}
}
