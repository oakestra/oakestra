// Package model defines the typed documents the resource abstractor reads
// and writes: applications, jobs, resources (candidates), hooks and custom
// resources. Every type pairs its known fields with an Extra map for
// whatever the caller sent that it doesn't recognize.
//
// The package only depends on encoding/json and the Mongo driver's bson
// struct tags, not gin or the openapi package, so anything that wants typed
// access to these documents can import it without pulling in an HTTP
// framework.
package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/jsonutil"
)

// Extra holds JSON/BSON fields a document carries outside its fixed schema.
// JSON round-trips it through each type's MarshalJSON/UnmarshalJSON below;
// BSON round-trips it through the `bson:",inline"` tag every type applies to
// its Extra field, which the v2 driver's struct codec handles natively.
type Extra map[string]any

// Ptr returns a pointer to v, for populating optional fields of a document
// literal: model.Job{JobName: model.Ptr("nginx")}.
func Ptr[T any](v T) *T { return &v }

// jsonFields is the map every type's MarshalJSON/UnmarshalJSON builds from
// and decodes into. Known keys get popped off one at a time; whatever's left
// becomes Extra.
type jsonFields = map[string]json.RawMessage

// fieldEncoder accumulates a document's typed fields into raw JSON. It uses
// the sticky-error pattern from net/http and bufio's readers: once an
// encodeXxx call fails, later ones are no-ops and finish reports the first
// error. That way a type's MarshalJSON is just one encode call per field,
// with the error checked once in finish.
type fieldEncoder struct {
	fields jsonFields
	err    error
}

// newFieldEncoder starts an encoder with an empty field set.
func newFieldEncoder() *fieldEncoder {
	return &fieldEncoder{fields: jsonFields{}}
}

// encodeValue always writes key, for fields with no notion of "absent" (e.g.
// CustomResourceDefinition's required ResourceType).
func encodeValue[T any](e *fieldEncoder, key string, v T) {
	if e.err != nil {
		return
	}
	raw, err := json.Marshal(v)
	if err != nil {
		e.err = fmt.Errorf("marshal field %q: %w", key, err)
		return
	}
	e.fields[key] = raw
}

// encodePtr omits key when v is nil, matching the "absent optional field"
// behavior every pointer field wants.
func encodePtr[T any](e *fieldEncoder, key string, v *T) {
	if e.err != nil || v == nil {
		return
	}
	encodeValue(e, key, v)
}

// encodeMap omits key when m is nil or empty: an empty map imposes no more
// of a constraint than an absent one, so there's no reason to write one over
// the other. Without this, encodeValue would marshal a nil map as a literal
// "null" instead of dropping the key.
func encodeMap(e *fieldEncoder, key string, m map[string]any) {
	if e.err != nil || len(m) == 0 {
		return
	}
	encodeValue(e, key, m)
}

// finish merges extra into whatever fields no typed key has claimed, then
// marshals the result. If an earlier encode call failed, that error is
// returned instead and extra is never touched.
func (e *fieldEncoder) finish(extra Extra) ([]byte, error) {
	if e.err != nil {
		return nil, e.err
	}
	if err := encodeExtra(e.fields, extra); err != nil {
		return nil, err
	}
	return marshalFields(e.fields)
}

// fieldDecoder does for UnmarshalJSON what fieldEncoder does for
// MarshalJSON: decodeXxx calls after the first error are no-ops, and finish
// reports it once.
//
// nullKeys tracks known fields whose incoming value was a literal JSON
// null, so finish can carry that into Extra as key: nil. A typed field
// itself can't hold this information - nil already means "absent" to every
// consumer that reads it - so Extra is the only place left to remember a
// key was present but explicitly null, which a stored document (or a PATCH
// body that intends to write one, see internal/store/jobs.go's
// UpdateJobInstance) needs to survive the next read.
type fieldDecoder struct {
	fields   jsonFields
	nullKeys []string
	err      error
}

// markNull records that key arrived as a literal null, for finish to carry
// into Extra.
func (d *fieldDecoder) markNull(key string) {
	d.nullKeys = append(d.nullKeys, key)
}

// newFieldDecoder parses data into a fieldDecoder ready for its typed keys
// to be popped off by decodeValue/decodePtr.
func newFieldDecoder(data []byte) (*fieldDecoder, error) {
	fields, err := unmarshalFields(data)
	if err != nil {
		return nil, err
	}
	return &fieldDecoder{fields: fields}, nil
}

// decodeValue decodes key into *dst, leaving *dst at its zero value when key
// is absent. A literal null is still handed to json.Unmarshal (a no-op for
// most scalar dsts, a nil map/slice for those kinds - unchanged from
// before), but is also recorded via markNull so finish can carry it into
// Extra; see decodePtr for why that matters.
func decodeValue[T any](d *fieldDecoder, key string, dst *T) {
	if d.err != nil {
		return
	}
	raw, ok := d.fields[key]
	if !ok {
		return
	}
	delete(d.fields, key)
	if string(raw) == "null" {
		d.markNull(key)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		d.err = fmt.Errorf("field %q: %w", key, err)
	}
}

// decodePtr allocates *dst and decodes key into it, leaving *dst nil when
// key is absent.
//
// A literal JSON null decodes to *dst = nil, the same as an absent key:
// no consumer of the typed field can tell "the caller sent null" apart from
// "the caller didn't send this key" from the pointer alone, and most don't
// need to. But the two aren't the same document: a job instance report
// (see internal/store/jobs.go's UpdateJobInstance) writes a literal null
// for a field it omits, and HEAD returned that key as "field": null rather
// than dropping it. So the null is also recorded via markNull, which finish
// carries into Extra as key: nil - the one place left to remember it once
// the typed field has collapsed both cases to the same nil. MarshalJSON's
// encodePtr already skips a nil field and lets Extra fill the key back in,
// so this is enough to make the value round-trip; nothing here needs to
// change how *dst behaves.
func decodePtr[T any](d *fieldDecoder, key string, dst **T) {
	if d.err != nil {
		return
	}
	raw, ok := d.fields[key]
	if !ok {
		return
	}
	delete(d.fields, key)
	if string(raw) == "null" {
		d.markNull(key)
		return
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		d.err = fmt.Errorf("field %q: %w", key, err)
		return
	}
	*dst = &v
}

// decodeResolved pops key and decodes it the way decodeExtra decodes an
// unknown field: through jsonutil, so a number nested inside keeps its
// int-vs-float identity. decodeValue/decodePtr can't be used for a field
// whose Go type bottoms out in `any` - plain encoding/json collapses every
// number there to float64, so an integer would land in MongoDB as a double
// while the same integer under an unknown key lands as an int64.
//
// Reports whether key was present and decoded; an explicit null is recorded
// via markNull and reported as absent, leaving the typed field at its zero
// value the way decodePtr does.
func decodeResolved(d *fieldDecoder, key string) (any, bool) {
	if d.err != nil {
		return nil, false
	}
	raw, ok := d.fields[key]
	if !ok {
		return nil, false
	}
	delete(d.fields, key)
	if string(raw) == "null" {
		d.markNull(key)
		return nil, false
	}
	v, err := decodeJSONValue(raw)
	if err != nil {
		d.err = fmt.Errorf("field %q: %w", key, err)
		return nil, false
	}
	return v, true
}

// decodeAnyMap is decodeValue for a map[string]any field, routed through
// decodeResolved. A value that isn't a JSON object fails with the same
// decode error json.Unmarshal into a map would report.
func decodeAnyMap(d *fieldDecoder, key string, dst *map[string]any) {
	v, ok := decodeResolved(d, key)
	if !ok {
		return
	}
	m, isMap := v.(map[string]any)
	if !isMap {
		d.err = fmt.Errorf("field %q: cannot unmarshal %T into map[string]interface {}", key, v)
		return
	}
	*dst = m
}

// decodeAnySlice is decodeAnyMap's counterpart for a *[]any field.
func decodeAnySlice(d *fieldDecoder, key string, dst **[]any) {
	v, ok := decodeResolved(d, key)
	if !ok {
		return
	}
	list, isList := v.([]any)
	if !isList {
		d.err = fmt.Errorf("field %q: cannot unmarshal %T into []interface {}", key, v)
		return
	}
	*dst = &list
}

// finish decodes whatever fields no typed key has claimed into *extra, then
// adds a nil entry for every key markNull recorded - a known field that
// arrived as a literal null - so it survives the next MarshalJSON/BSON
// re-encode as key: null instead of vanishing the way an actually-absent
// field does. If an earlier decode call failed, that error is returned
// instead and extra is never touched.
func (d *fieldDecoder) finish(extra *Extra) error {
	if d.err != nil {
		return d.err
	}
	e, err := decodeExtra(d.fields)
	if err != nil {
		return err
	}
	if len(d.nullKeys) > 0 {
		if e == nil {
			e = make(Extra, len(d.nullKeys))
		}
		for _, key := range d.nullKeys {
			e[key] = nil
		}
	}
	*extra = e
	return nil
}

// decodeExtra turns whatever fields a type didn't claim into an Extra map.
// Numbers decode via json.Number and get resolved by
// jsonutil.ResolveNumbers, so a value keeps its int-vs-float identity
// instead of always landing on float64 the way a plain json.Unmarshal into
// interface{} would.
func decodeExtra(fields jsonFields) (Extra, error) {
	if len(fields) == 0 {
		return nil, nil
	}

	extra := make(Extra, len(fields))
	for key, raw := range fields {
		v, err := decodeJSONValue(raw)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", key, err)
		}
		extra[key] = v
	}
	return extra, nil
}

// encodeExtra adds every entry of extra to fields under its own key, unless
// a typed field already claimed that key. A typed field always wins, so a
// caller can't smuggle a value past validation by hiding it under a
// canonical name in Extra. Extra can legitimately hold a canonical key
// here: decodePtr/decodeValue record an explicit null under its own field
// name (see fieldDecoder.finish), and that only reaches fields if the
// typed field itself left the key unclaimed - a nil pointer omits it, so
// the null in Extra is what puts the key back.
func encodeExtra(fields jsonFields, extra Extra) error {
	for key, v := range extra {
		if _, exists := fields[key]; exists {
			continue
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("marshal extra field %q: %w", key, err)
		}
		fields[key] = raw
	}
	return nil
}

// decodeJSONValue decodes a single JSON value the way decodeExtra needs,
// with numbers resolved to int64/float64 by jsonutil.Decode.
func decodeJSONValue(raw json.RawMessage) (any, error) {
	var v any
	if err := jsonutil.Decode(bytes.NewReader(raw), &v); err != nil {
		return nil, err
	}
	return v, nil
}

// marshalFields is fieldEncoder.finish's last step. Since the map values are
// already-valid JSON (json.RawMessage), it marshals directly, and
// encoding/json sorts map keys alphabetically, so the output stays
// deterministic for free.
func marshalFields(fields jsonFields) ([]byte, error) {
	return json.Marshal(fields)
}

// unmarshalFields is newFieldDecoder's first step: decode the body into a
// map of raw per-field JSON so each field can be popped off by name. A
// literal JSON null decodes to a nil map, which gets normalized to an empty
// one so a type with no required fields treats it as an empty document
// instead of panicking on a nil map.
func unmarshalFields(data []byte) (jsonFields, error) {
	var fields jsonFields
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		fields = jsonFields{}
	}
	return fields, nil
}

// bsonFieldEncoder is fieldEncoder's BSON counterpart, used by a type's
// MarshalBSON. It can't reuse fieldEncoder's json.RawMessage-based fields
// map: internal/store builds Mongo updates straight from the values this
// produces (see toSetDoc), so they need to be native Go values, not
// pre-encoded JSON.
//
// It exists at all because the driver's default struct+inline-map codec
// can't express "write this known field as a literal null": it refuses an
// inline Extra entry whose key names any struct field, present or not
// (mapCodec.encodeMapElements' collisionFn), which is exactly what
// recording an explicit null under a canonical key needs to do. Building
// the document by hand here sidesteps that check entirely.
type bsonFieldEncoder struct {
	fields bson.M
	extra  Extra
}

// newBSONFieldEncoder starts an encoder with an empty field set, holding
// extra so encodeBSONPtr/encodeBSONMap can tell an explicit null recorded
// there (by decodePtr/decodeValue for a JSON body, or markBSONNulls for a
// document read back from Mongo) apart from a field that's simply absent.
func newBSONFieldEncoder(extra Extra) *bsonFieldEncoder {
	return &bsonFieldEncoder{fields: bson.M{}, extra: extra}
}

// explicitNull reports whether extra records key as having arrived null
// rather than simply being unset.
func (e *bsonFieldEncoder) explicitNull(key string) bool {
	if e.extra == nil {
		return false
	}
	v, present := e.extra[key]
	return present && v == nil
}

// encodeBSONValue always writes key, for fields with no notion of "absent"
// (e.g. CustomResourceDefinition's required ResourceType).
func encodeBSONValue[T any](e *bsonFieldEncoder, key string, v T) {
	e.fields[key] = v
}

// encodeBSONPtr writes key's underlying value when v is non-nil. A nil v is
// omitted, unless e.extra marks key explicitly null, in which case it's
// written as a literal BSON null so a document round-trips the way it was
// read (or the way a PATCH body asked to write it - see
// internal/store/jobs.go's UpdateJobInstance for the same null-on-omit
// behavior applied directly, without going through a model type).
func encodeBSONPtr[T any](e *bsonFieldEncoder, key string, v *T) {
	if v != nil {
		e.fields[key] = *v
		return
	}
	if e.explicitNull(key) {
		e.fields[key] = nil
	}
}

// encodeBSONMap omits key when m is nil or empty, the same "empty imposes
// no more of a constraint than absent" reasoning as encodeMap, but still
// writes an explicit null when e.extra records one.
func encodeBSONMap(e *bsonFieldEncoder, key string, m map[string]any) {
	if len(m) > 0 {
		e.fields[key] = m
		return
	}
	if e.explicitNull(key) {
		e.fields[key] = nil
	}
}

// finish merges e.extra into whatever fields no typed key has claimed
// (including a key a typed field left null - see encodeBSONPtr), then
// marshals the result to BSON.
func (e *bsonFieldEncoder) finish() ([]byte, error) {
	for key, v := range e.extra {
		if _, exists := e.fields[key]; exists {
			continue
		}
		e.fields[key] = v
	}
	return bson.Marshal(e.fields)
}

// decodeBSONAlias decodes data into dst with ObjectIDAsHexString enabled,
// matching the BSONOptions internal/store's collections carry (see
// store.hexIDOptions). A type implementing bson.Unmarshaler is decoded by
// the driver calling UnmarshalBSON directly with the raw document bytes,
// bypassing whatever options the surrounding collection or Decoder was
// configured with, so a type whose UnmarshalBSON decodes into a shadow
// struct (to reach the default codec without recursing back into itself)
// has to apply that option itself rather than inherit it.
func decodeBSONAlias(data []byte, dst any) error {
	dec := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(data)))
	dec.ObjectIDAsHexString()
	return dec.Decode(dst)
}

// bsonKeysCache memoizes bsonKeys per type, so the reflection walk happens
// once per document type rather than on every decode.
var bsonKeysCache sync.Map // reflect.Type -> []string

// bsonKeys returns T's named BSON field names, read from its `bson` struct
// tags. The `,inline` Extra field is skipped: it has no key of its own, and
// the keys it carries are by definition the ones the struct doesn't name.
//
// Read from the tags rather than hand-listed per type: a forgotten entry
// fails silently, the field just stops round-tripping explicit nulls.
func bsonKeys[T any]() []string {
	t := reflect.TypeFor[T]()
	if cached, ok := bsonKeysCache.Load(t); ok {
		return cached.([]string)
	}

	keys := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		name, opts, _ := strings.Cut(t.Field(i).Tag.Get("bson"), ",")
		if name == "" || name == "-" || strings.Contains(opts, "inline") {
			continue
		}
		keys = append(keys, name)
	}

	bsonKeysCache.Store(t, keys)
	return keys
}

// markBSONNulls records, in *extra, an explicit nil for every one of T's
// named BSON fields that raw's document holds as a literal BSON null.
//
// A null value for a named field is consumed by that field during the
// ordinary struct decode a type's UnmarshalBSON runs alongside this call -
// the pointer just ends up nil, indistinguishable from the key being absent
// altogether - so without this, Extra would carry no record the key was
// ever there, and the next MarshalBSON/MarshalJSON would drop it instead of
// writing null back. This is BSON's counterpart to what decodePtr/
// decodeValue already do for a JSON body.
func markBSONNulls[T any](raw []byte, extra *Extra) {
	doc := bson.Raw(raw)
	for _, key := range bsonKeys[T]() {
		val, err := doc.LookupErr(key)
		if err != nil || val.Type != bson.TypeNull {
			continue
		}
		if *extra == nil {
			*extra = Extra{}
		}
		(*extra)[key] = nil
	}
}
