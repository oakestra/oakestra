// Package errs holds the sentinel errors internal/store and internal/hooks
// return. They live here rather than in internal/store so that hooks, which
// has no business knowing about the database driver, can return them without
// importing the store package.
package errs

import (
	"errors"
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
