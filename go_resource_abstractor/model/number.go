package model

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Float and Int exist because NodeEngine (go_node_engine/model/Service.go)
// and cluster_manager (cluster_orchestrator/cluster-manager/clients/job_management.py)
// report a job instance's cpu_percent/memory_percent/disk/host_port as
// strings, not numbers, and the Python resource-abstractor stored them
// verbatim - so both live requests and existing MongoDB documents hold these
// as strings. A plain *float64/*int64 field 400s on the request and 500s on
// the read. Both types decode a number or a numeric string and always
// re-encode as a number, normalizing a document the next time it's written.
//
// Neither type defines MarshalJSON/MarshalBSONValue: their underlying kind
// (float64/int64) already marshals as a plain number by default, which is
// exactly the normalization wanted, so only the lenient decode side needs
// overriding.
//
// Both reject a non-finite value (NaN, +Inf, -Inf): strconv.ParseFloat
// happily parses "NaN"/"Inf"/"Infinity" strings, and a BSON double or
// decimal128 can hold one directly, but encoding/json can't marshal a
// non-finite float. Accepting one here would decode fine and only fail
// later, on every subsequent response that happens to include this job
// instance - a confusing place to first learn the stored value was bad.

// Float is a lenient float64: it accepts a JSON/BSON number or a string
// holding one.
type Float float64

// Int is a lenient int64: it accepts a JSON/BSON integer, or a float or
// numeric string that happens to be a whole number. host_port is the only
// field that uses it - cluster_manager forwards a candidate's port field
// verbatim, which is a string in the general case but the literal fallback
// int 50011 when the candidate has none.
type Int int64

// UnmarshalJSON accepts a JSON number or a JSON string holding one. An empty
// string is rejected rather than treated as absent: unlike a missing key or
// a literal null, which decodePtr already maps to "leave the field
// untouched", an explicit "" is a value the caller sent, and it isn't a
// number.
func (f *Float) UnmarshalJSON(data []byte) error {
	v, err := decodeLenientNumber(data)
	if err != nil {
		return fmt.Errorf("model.Float: %w", err)
	}
	*f = Float(v)
	return nil
}

// UnmarshalJSON is Float's UnmarshalJSON plus an integral check: 50011.5
// isn't a valid host_port.
func (i *Int) UnmarshalJSON(data []byte) error {
	v, err := decodeLenientNumber(data)
	if err != nil {
		return fmt.Errorf("model.Int: %w", err)
	}
	n, err := floatToInt(v)
	if err != nil {
		return fmt.Errorf("model.Int: %w", err)
	}
	*i = Int(n)
	return nil
}

// UnmarshalBSONValue accepts a double, int32, int64, decimal128 or a string
// holding a number, covering both what NodeEngine/cluster_manager send today
// and what the Python service already wrote to Mongo.
func (f *Float) UnmarshalBSONValue(typ byte, data []byte) error {
	rv := bson.RawValue{Type: bson.Type(typ), Value: data}
	switch rv.Type {
	case bson.TypeDouble:
		v, err := finite(rv.Double())
		if err != nil {
			return fmt.Errorf("model.Float: %w", err)
		}
		*f = Float(v)
	case bson.TypeInt32:
		*f = Float(rv.Int32())
	case bson.TypeInt64:
		*f = Float(rv.Int64())
	case bson.TypeDecimal128:
		v, err := parseNumberString(rv.Decimal128().String())
		if err != nil {
			return fmt.Errorf("model.Float: %w", err)
		}
		*f = Float(v)
	case bson.TypeString:
		v, err := parseNumberString(rv.StringValue())
		if err != nil {
			return fmt.Errorf("model.Float: %w", err)
		}
		*f = Float(v)
	default:
		return fmt.Errorf("model.Float: cannot decode BSON type %s as a number", rv.Type)
	}
	return nil
}

// UnmarshalBSONValue is Float's UnmarshalBSONValue plus an integral check.
// Unlike Int.UnmarshalJSON's path through decodeLenientNumber, a BSON double
// is read directly (not via a float64->JSON->float64 round trip), so the
// integral check happens right here instead of being shared.
func (i *Int) UnmarshalBSONValue(typ byte, data []byte) error {
	rv := bson.RawValue{Type: bson.Type(typ), Value: data}
	switch rv.Type {
	case bson.TypeInt32:
		*i = Int(rv.Int32())
	case bson.TypeInt64:
		*i = Int(rv.Int64())
	case bson.TypeDouble:
		n, err := floatToInt(rv.Double())
		if err != nil {
			return fmt.Errorf("model.Int: %w", err)
		}
		*i = Int(n)
	case bson.TypeDecimal128:
		n, err := parseIntString(rv.Decimal128().String())
		if err != nil {
			return fmt.Errorf("model.Int: %w", err)
		}
		*i = Int(n)
	case bson.TypeString:
		n, err := parseIntString(rv.StringValue())
		if err != nil {
			return fmt.Errorf("model.Int: %w", err)
		}
		*i = Int(n)
	default:
		return fmt.Errorf("model.Int: cannot decode BSON type %s as an integer", rv.Type)
	}
	return nil
}

// decodeLenientNumber reads a JSON number, or a JSON string holding one,
// as a float64.
func decodeLenientNumber(data []byte) (float64, error) {
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return 0, err
		}
		return parseNumberString(s)
	}
	var f float64
	if err := json.Unmarshal(data, &f); err != nil {
		return 0, fmt.Errorf("%s is not a number", data)
	}
	return finite(f)
}

// parseNumberString parses s as a float64, the shared leaf both JSON string
// values and BSON string/decimal128 values go through.
func parseNumberString(s string) (float64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty string is not a number")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number", s)
	}
	return finite(f)
}

// parseIntString parses s as an int64, rejecting a numeric string with a
// fractional part the same way floatToInt does for a JSON/BSON float.
func parseIntString(s string) (int64, error) {
	f, err := parseNumberString(s)
	if err != nil {
		return 0, err
	}
	return floatToInt(f)
}

// finite rejects NaN and +/-Inf. See the package-level comment above for why:
// in short, encoding/json can't marshal either, so letting one through here
// only defers the failure to the next response that includes this field.
func finite(f float64) (float64, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("%v is not a finite number", f)
	}
	return f, nil
}

// floatToInt rejects a non-integral value instead of silently truncating it,
// so a malformed host_port like 50011.5 is a decode error, not a quietly
// wrong 50011. It also rejects +/-Inf up front: math.Trunc leaves infinity
// unchanged, so the integral check below wouldn't otherwise catch it, and
// int64(+Inf) is an implementation-defined conversion, not a decode error.
func floatToInt(f float64) (int64, error) {
	if _, err := finite(f); err != nil {
		return 0, err
	}
	if math.Trunc(f) != f {
		return 0, fmt.Errorf("%v is not a whole number", f)
	}
	return int64(f), nil
}
