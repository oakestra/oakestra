package model

import "errors"

// ErrRuntimeMigrationNotSupported is returned when the runtime does not support migration
var ErrRuntimeMigrationNotSupported = errors.New("runtime migration not supported")
