package definition

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"time"

	"sigs.k8s.io/yaml"
)

// IndexSchema is the only index schema version this package understands.
const IndexSchema = 1

// MaxIndexSize caps how much DecodeIndex reads. The index comes from the network, so an
// unbounded read would let a hostile source exhaust memory.
const MaxIndexSize = 5 << 20

// MaxAddonSize caps how much DecodeAddon reads.
const MaxAddonSize = 256 << 10

// Index is a catalog file published by the official source or a tap. It lists the latest
// version of every addon.
type Index struct {
	Schema int `json:"schema"`
	// Source is "official" or the tap name.
	Source string `json:"source"`
	// Serial must increase with every publication. Sync rejects an index with a lower serial
	// than the last good one, which is the only rollback protection until signing exists.
	Serial      uint64    `json:"serial"`
	GeneratedAt time.Time `json:"generated_at"`
	Addons      []Addon   `json:"addons"`
	// Signatures is reserved for signed indexes. Readers ignore its content, so a future
	// writer can fill it without breaking older readers. Writers in v1 emit an empty list.
	Signatures []json.RawMessage `json:"signatures"`
}

// ErrTooLarge is returned when an input exceeds its size limit.
var ErrTooLarge = errors.New("input exceeds the size limit")

// DecodeIndex reads an index strictly: at most MaxIndexSize bytes, no unknown fields anywhere,
// no duplicate keys, field names spelled exactly as tagged (see checkKeys), no trailing data,
// and a schema version this package knows. It stamps the index source on every addon and
// normalizes them. Addons that set their own source must match the index.
//
// It does not run Validate or CheckPolicy; use ValidateIndex for that.
func DecodeIndex(r io.Reader) (*Index, error) {
	var idx Index
	if err := decodeStrict(r, MaxIndexSize, &idx); err != nil {
		return nil, err
	}
	if idx.Schema != IndexSchema {
		return nil, fmt.Errorf("unsupported index schema %d, expected %d", idx.Schema, IndexSchema)
	}
	src, err := SourceFromIndexName(idx.Source)
	if err != nil {
		return nil, fmt.Errorf("index source: %w", err)
	}
	for i := range idx.Addons {
		a := &idx.Addons[i]
		if a.Source != (Source{}) && a.Source != src {
			return nil, fmt.Errorf("addons[%d]: source %+v doesn't match the index source %q", i, a.Source, idx.Source)
		}
		a.Source = src
		a.Normalize()
	}
	if idx.Signatures == nil {
		idx.Signatures = []json.RawMessage{}
	}
	return &idx, nil
}

// ValidateIndex runs Validate (in ModeIndex) and CheckPolicy on every addon and checks the
// index metadata. Paths are prefixed with "addons[i]". Policy violations are included.
func ValidateIndex(idx *Index) Errors {
	var errs Errors
	if idx.Schema != IndexSchema {
		errs = errs.add("schema", "must be %d", IndexSchema)
	}
	if _, err := SourceFromIndexName(idx.Source); err != nil {
		errs = errs.add("source", "%v", err)
	}
	if idx.Serial == 0 {
		errs = errs.add("serial", "must be greater than 0")
	}
	if idx.GeneratedAt.IsZero() {
		errs = errs.add("generated_at", "is required")
	}
	seen := map[string]int{}
	for i := range idx.Addons {
		a := &idx.Addons[i]
		p := fmt.Sprintf("addons[%d]", i)
		if prev, dup := seen[a.Name]; dup {
			errs = errs.add(p+".name", "%q is already listed at addons[%d]", a.Name, prev)
		}
		seen[a.Name] = i
		errs = append(errs, Validate(a, ModeIndex).prefixed(p)...)
		for _, v := range CheckPolicy(a) {
			errs = append(errs, FieldError{Path: p + "." + v.Path, Message: v.Message})
		}
	}
	return errs
}

// NewIndex builds an index with addons sorted by name and the source fields cleared (the index
// carries the source), so the output only depends on its inputs.
func NewIndex(source string, serial uint64, generatedAt time.Time, addons []Addon) *Index {
	sorted := slices.Clone(addons)
	slices.SortFunc(sorted, func(a, b Addon) int { return strings.Compare(a.Name, b.Name) })
	for i := range sorted {
		sorted[i].Source = Source{}
		sorted[i].Normalize()
	}
	return &Index{
		Schema:      IndexSchema,
		Source:      source,
		Serial:      serial,
		GeneratedAt: generatedAt.UTC(),
		Addons:      sorted,
		Signatures:  []json.RawMessage{},
	}
}

// DecodeAddon reads one JSON definition strictly (size limit, no unknown fields, no duplicate
// or miscased keys, no trailing data) and normalizes it. The source is left as given; callers
// set it.
func DecodeAddon(r io.Reader) (*Addon, error) {
	var a Addon
	if err := decodeStrict(r, MaxAddonSize, &a); err != nil {
		return nil, err
	}
	a.Normalize()
	return &a, nil
}

// UnmarshalAddonYAML parses one YAML definition (the catalog file format) with the same
// strictness as DecodeAddon; the key checks run on the converted JSON, so "Image" next to
// "image" is caught here too. Duplicate keys are rejected. YAML values for string fields must be
// strings, so quote numbers in ports and environment.
func UnmarshalAddonYAML(data []byte) (*Addon, error) {
	if len(data) > MaxAddonSize {
		return nil, ErrTooLarge
	}
	js, err := yaml.YAMLToJSONStrict(data)
	if err != nil {
		return nil, err
	}
	return DecodeAddon(bytes.NewReader(js))
}

func decodeStrict(r io.Reader, limit int64, v any) error {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return ErrTooLarge
	}
	if err := checkKeys(data, reflect.TypeOf(v)); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("unexpected data after the JSON document")
	}
	return nil
}
