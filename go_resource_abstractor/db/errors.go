package db

import (
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// ErrNotFound is returned by lookups that find no matching document. It is
// an alias for mongo.ErrNoDocuments so callers can use errors.Is against
// either name.
var ErrNotFound = mongo.ErrNoDocuments

// ErrInvalidID is returned when an id string isn't a valid hex ObjectID.
// Distinct from ErrNotFound so callers can tell a malformed id apart from a
// missing document.
var ErrInvalidID = errors.New("invalid object id")

// ErrInvalidResourceType is returned by CreateCustomResource for a
// resource_type that validateResourceType rejects.
var ErrInvalidResourceType = errors.New("invalid resource type")

// parseObjectID decodes id as a hex ObjectID, wrapping bson's error in
// ErrInvalidID so callers can check errors.Is(err, ErrInvalidID) regardless
// of what the driver itself returns.
func parseObjectID(id string) (bson.ObjectID, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return bson.ObjectID{}, fmt.Errorf("%w: %q", ErrInvalidID, id)
	}
	return oid, nil
}
