package store

import (
	"context"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/errs"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

// FindCustomResources lists registered custom-resource type definitions
// from the meta_data collection.
func (s *Store) FindCustomResources(ctx context.Context) ([]model.CustomResourceDefinition, error) {
	return findAll[model.CustomResourceDefinition](ctx, s.metaData, nil)
}

// FindCustomResourceByType looks up a type definition by its resource_type.
func (s *Store) FindCustomResourceByType(ctx context.Context, resourceType string) (model.CustomResourceDefinition, error) {
	return findOne[model.CustomResourceDefinition](ctx, s.metaData, bson.M{"resource_type": resourceType})
}

// DeleteCustomResourceByType removes a type definition, returning it as
// deleted.
func (s *Store) DeleteCustomResourceByType(ctx context.Context, resourceType string) (model.CustomResourceDefinition, error) {
	return findOneAndDelete[model.CustomResourceDefinition](ctx, s.metaData, bson.M{"resource_type": resourceType})
}

// CreateCustomResource registers a new custom-resource type definition,
// dropping any client-supplied _id. resource_type is validated here: the
// instance routes only accept types that are already registered, so this is
// the one place a bad name could get in.
func (s *Store) CreateCustomResource(ctx context.Context, data model.CustomResourceDefinition) (model.CustomResourceDefinition, error) {
	if err := ValidateResourceType(data.ResourceType); err != nil {
		return model.CustomResourceDefinition{}, err
	}
	return insertReturning(ctx, s.metaData, data)
}

// maxResourceTypeLength is MongoDB's own collection-name limit (120 bytes,
// counting the database prefix): a resource_type longer than this can never
// become a valid collection name, so ValidateResourceType rejects it before
// it reaches the driver.
const maxResourceTypeLength = 120

// ValidateResourceType rejects resource_type values that would collide
// with the meta_data definitions collection or misbehave as a MongoDB
// collection name. Without this, registering "meta_data" as a type lets a
// later delete of that type run DeleteMany({}) directly against meta_data,
// wiping every other registered definition. Names with "$", a NUL byte, or
// a "system." prefix are collection names MongoDB itself refuses, which
// would otherwise surface as a raw driver error instead of a clean
// validation failure.
func ValidateResourceType(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("%w: resource_type is required", errs.ErrInvalidResourceType)
	case name == metaDataCollectionName:
		return fmt.Errorf("%w: %q is reserved for custom resource type definitions", errs.ErrInvalidResourceType, name)
	case strings.HasPrefix(name, "system."):
		return fmt.Errorf("%w: %q uses the reserved \"system.\" prefix", errs.ErrInvalidResourceType, name)
	case strings.ContainsAny(name, "$\x00"):
		return fmt.Errorf("%w: %q contains a character not allowed in a collection name", errs.ErrInvalidResourceType, name)
	case len(name) > maxResourceTypeLength:
		return fmt.Errorf("%w: %q is longer than the %d-byte collection name limit", errs.ErrInvalidResourceType, name, maxResourceTypeLength)
	default:
		return nil
	}
}

// FindResources lists instances of the given custom resource type matching
// filter, from that type's dynamically named collection.
func (s *Store) FindResources(ctx context.Context, resourceType string, filter bson.M) ([]model.CustomResourceInstance, error) {
	return findAll[model.CustomResourceInstance](ctx, s.customResourceCollection(resourceType), filter)
}

// FindResourceByID looks up a single instance of resourceType by id.
func (s *Store) FindResourceByID(ctx context.Context, resourceType, id string) (model.CustomResourceInstance, error) {
	return findByID[model.CustomResourceInstance](ctx, s.customResourceCollection(resourceType), id, nil)
}

// CreateResource inserts a new instance of resourceType.
func (s *Store) CreateResource(ctx context.Context, resourceType string, data model.CustomResourceInstance) (model.CustomResourceInstance, error) {
	return insertReturning(ctx, s.customResourceCollection(resourceType), data)
}

// UpdateResource applies a plain $set update to an instance of
// resourceType, dropping any client-supplied _id.
func (s *Store) UpdateResource(ctx context.Context, resourceType, id string, data model.CustomResourceInstance) (model.CustomResourceInstance, error) {
	return updateByID(ctx, s.customResourceCollection(resourceType), id, data)
}

// DeleteResource removes an instance of resourceType by id, returning it as
// deleted.
func (s *Store) DeleteResource(ctx context.Context, resourceType, id string) (model.CustomResourceInstance, error) {
	return deleteByIDReturning[model.CustomResourceInstance](ctx, s.customResourceCollection(resourceType), id)
}

// DeleteAllResources drops every instance of resourceType, used by the
// cascading delete of a type definition. Returns the number of deleted
// instances.
func (s *Store) DeleteAllResources(ctx context.Context, resourceType string) (int64, error) {
	result, err := s.customResourceCollection(resourceType).DeleteMany(ctx, bson.M{})
	if err != nil {
		return 0, err
	}
	return result.DeletedCount, nil
}
