// Package errs holds the sentinel errors and error types internal/store and
// internal/hooks return. They live in their own package, separate from
// internal/store, so abstractor can re-export them for library callers by
// depending on errs directly instead of on internal/store's import graph.
package errs

import (
	"errors"
	"fmt"
)

// ErrNotFound is returned by lookups that find no matching document.
var ErrNotFound = errors.New("not found")

// ErrInvalidID is returned when an id string isn't a valid hex ObjectID.
// Distinct from ErrNotFound so callers can tell a malformed id apart from a
// missing document (the REST layer maps the two to 400 and 404
// respectively).
var ErrInvalidID = errors.New("invalid object id")

// ErrInstanceExists is returned by AppendJobInstance when the instance
// already exists, the payload's instance_list is empty, or the parent job
// doesn't exist. The name only covers the first case.
var ErrInstanceExists = errors.New("job instance already exists or payload has no instance to append")

// ErrInvalidResourceType is returned when a custom resource_type would
// collide with the meta_data definitions collection or misbehave as a
// MongoDB collection name.
var ErrInvalidResourceType = errors.New("invalid resource type")

// ValidationError reports a single field-level validation failure. Callers
// that need the API's {"message": "Invalid input", "details": {...}} shape
// build it from Field/Message rather than parsing Error()'s text.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}
