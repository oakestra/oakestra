package abstractor

import (
	"errors"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/errs"
	"github.com/oakestra/oakestra/go_resource_abstractor/internal/store"
)

// Sentinel errors every sub-service can return, re-exported from
// internal/errs so a library caller never needs to import an internal
// package to check for them with errors.Is.
var (
	ErrNotFound            = errs.ErrNotFound
	ErrInvalidID           = errs.ErrInvalidID
	ErrInstanceExists      = errs.ErrInstanceExists
	ErrInvalidResourceType = errs.ErrInvalidResourceType
)

// IsValidID reports whether id is well-formed, for callers that answer a
// malformed id differently from how they treat ErrInvalidID.
func IsValidID(id string) bool {
	return store.IsValidID(id)
}

// ErrInvalidHookEvent means data.Events named something outside the
// model.HookEvent enum.
var ErrInvalidHookEvent = errors.New("invalid hook event")

// ErrInvalidFilterKey means a filter key would be interpreted as a MongoDB
// query operator, like $where.
var ErrInvalidFilterKey = errors.New("invalid filter key")

// ErrSchemaInvalid means the payload fails the type's stored JSON Schema. A
// schema that itself fails to compile is a plain error instead, not this
// sentinel.
var ErrSchemaInvalid = errors.New("instance does not satisfy the custom resource's schema")
