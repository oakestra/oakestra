// Package store persists installed addons.
package store

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/oakestra/oakestra/addons/internal/addons"
)

var (
	// ErrNotFound is returned for unknown ids, including malformed ones.
	ErrNotFound = errors.New("addon not found")
	// ErrDuplicate is returned by Insert when a current installation of the addon exists.
	ErrDuplicate = errors.New("addon is already installed")
	// ErrAbort can be returned from an Update callback to leave the record untouched without
	// reporting an error.
	ErrAbort = errors.New("update aborted")
)

// Store is the persistence of installed addons.
type Store interface {
	// List returns the addons in creation order, optionally only those with the given status.
	List(ctx context.Context, status addons.Status) ([]addons.Installed, error)
	Get(ctx context.Context, id string) (addons.Installed, error)
	// Insert stores a new addon with Current=true. It uses a.ID when set (it must come from
	// NewID), otherwise it assigns one. It returns ErrDuplicate if another current
	// installation of the same AddonID exists.
	Insert(ctx context.Context, a addons.Installed) (addons.Installed, error)
	// Update applies fn to the current record and stores the result. The API and the reconcile
	// loop both write, so fn may run more than once and has to be a pure function of its
	// argument. Returning ErrAbort skips the write; Update then returns the unchanged record
	// and a nil error. The callback sets UpdatedAt when the change should be visible.
	Update(ctx context.Context, id string, fn func(*addons.Installed) error) (addons.Installed, error)
	Delete(ctx context.Context, id string) error
}

// NewID returns a fresh record id. The API picks the id before inserting because the grant
// signature covers it: a record that appeared first and got signed in a second write could be
// picked up unsigned by the reconcile loop in between.
func NewID() string {
	return bson.NewObjectID().Hex()
}
