package definition

import (
	"fmt"
	"strings"
)

// FieldError is one problem with a definition. Path uses JSON field names and indexes, e.g.
// "services[0].volumes[1]".
type FieldError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e FieldError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return e.Path + ": " + e.Message
}

// Errors collects every problem found, so authors can fix them all in one go.
type Errors []FieldError

func (e Errors) Error() string {
	msgs := make([]string, len(e))
	for i, fe := range e {
		msgs[i] = fe.Error()
	}
	return strings.Join(msgs, "; ")
}

// Err returns nil for an empty list, so callers can write `if err := errs.Err(); err != nil`
// without tripping over a typed nil inside a non-nil error interface.
func (e Errors) Err() error {
	if len(e) == 0 {
		return nil
	}
	return e
}

// add returns e with one more error. It uses a value receiver like the rest of the methods,
// so call it as errs = errs.add(...).
func (e Errors) add(path, format string, args ...any) Errors {
	return append(e, FieldError{Path: path, Message: fmt.Sprintf(format, args...)})
}

// prefixed returns a copy with prefix prepended to every path.
func (e Errors) prefixed(prefix string) Errors {
	out := make(Errors, len(e))
	for i, fe := range e {
		if fe.Path == "" {
			fe.Path = prefix
		} else {
			fe.Path = prefix + "." + fe.Path
		}
		out[i] = fe
	}
	return out
}

func idx(path string, i int) string {
	return fmt.Sprintf("%s[%d]", path, i)
}

func key(path, k string) string {
	return fmt.Sprintf("%s[%q]", path, k)
}
