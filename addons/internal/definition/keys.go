package definition

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// checkKeys walks a JSON document against the Go type it will be decoded into and rejects
// keys that encoding/json would accept but a human reviewer would read differently.
// encoding/json matches struct fields case-insensitively and lets the last duplicate win, so
// {"image": "good@...", "Image": "evil@..."} decodes to the evil image while a reviewer skimming
// the file sees the good one first. To rule that out:
//
//   - struct objects may only use keys spelled exactly like the JSON tags, once each;
//   - every other object (maps, and opaque values like json.RawMessage) may not repeat a key
//     exactly. Keys differing only in case stay allowed there, since FOO and foo are distinct
//     environment variables and labels and decode to distinct map entries.
//
// Syntax, truncation and type errors are left to the real decode, which reports them better.
func checkKeys(data []byte, t reflect.Type) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		// Not JSON at all; the decode that follows produces the useful error.
		return nil
	}
	err = walkJSON(dec, tok, t, "")
	var ke *keyError
	if errors.As(err, &ke) {
		return ke
	}
	return nil
}

// keyError separates the problems checkKeys is responsible for from tokenizer errors.
type keyError struct{ msg string }

func (e *keyError) Error() string { return e.msg }

func keyErrorf(format string, args ...any) error {
	return &keyError{msg: fmt.Sprintf(format, args...)}
}

var unmarshalerType = reflect.TypeFor[json.Unmarshaler]()

func walkJSON(dec *json.Decoder, tok json.Token, t reflect.Type, path string) error {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	// Types with their own UnmarshalJSON (time.Time, json.RawMessage) don't map keys onto
	// fields, so only the generic duplicate check applies inside them.
	if t != nil && reflect.PointerTo(t).Implements(unmarshalerType) {
		t = nil
	}
	d, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch d {
	case '[':
		var elem reflect.Type
		if t != nil && (t.Kind() == reflect.Slice || t.Kind() == reflect.Array) {
			elem = t.Elem()
		}
		for i := 0; dec.More(); i++ {
			next, err := dec.Token()
			if err != nil {
				return err
			}
			if err := walkJSON(dec, next, elem, idx(path, i)); err != nil {
				return err
			}
		}
	case '{':
		var fields map[string]reflect.Type
		if t != nil && t.Kind() == reflect.Struct {
			fields = jsonFields(t)
		}
		seen := map[string]bool{}
		for dec.More() {
			ktok, err := dec.Token()
			if err != nil {
				return err
			}
			k, _ := ktok.(string)
			if seen[k] {
				return keyErrorf("%s: key %q appears more than once", orRoot(path), k)
			}
			seen[k] = true

			var vt reflect.Type
			child := key(path, k)
			switch {
			case fields != nil:
				ft, ok := fields[k]
				if !ok {
					for tag := range fields {
						if strings.EqualFold(tag, k) {
							return keyErrorf("%s: key %q must be spelled %q", orRoot(path), k, tag)
						}
					}
					return keyErrorf("%s: unknown field %q", orRoot(path), k)
				}
				vt = ft
				child = k
				if path != "" {
					child = path + "." + k
				}
			case t != nil && t.Kind() == reflect.Map:
				vt = t.Elem()
			}

			vtok, err := dec.Token()
			if err != nil {
				return err
			}
			if err := walkJSON(dec, vtok, vt, child); err != nil {
				return err
			}
		}
	default:
		return nil
	}
	// The closing delimiter.
	_, err := dec.Token()
	return err
}

func orRoot(path string) string {
	if path == "" {
		return "document"
	}
	return path
}

var fieldCache sync.Map // reflect.Type -> map[string]reflect.Type

// jsonFields maps the JSON names of a struct's exported fields to their types. The definition
// types don't embed structs, so promoted fields aren't handled.
func jsonFields(t reflect.Type) map[string]reflect.Type {
	if m, ok := fieldCache.Load(t); ok {
		return m.(map[string]reflect.Type)
	}
	m := map[string]reflect.Type{}
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		m[name] = f.Type
	}
	fieldCache.Store(t, m)
	return m
}
