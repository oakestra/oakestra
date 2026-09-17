package db

import (
	"context"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// FindCustomResources lists registered custom-resource type definitions
// from the meta_data collection.
func (s *Store) FindCustomResources(ctx context.Context) ([]bson.M, error) {
	return findAll(ctx, s.metaData, nil)
}

// FindCustomResourceByType looks up a type definition by its resource_type.
func (s *Store) FindCustomResourceByType(ctx context.Context, resourceType string) (bson.M, error) {
	var def bson.M
	if err := s.metaData.FindOne(ctx, bson.M{"resource_type": resourceType}).Decode(&def); err != nil {
		return nil, err
	}
	return def, nil
}

// DeleteCustomResourceByType removes a type definition, returning it as
// deleted.
func (s *Store) DeleteCustomResourceByType(ctx context.Context, resourceType string) (bson.M, error) {
	var deleted bson.M
	err := s.metaData.FindOneAndDelete(ctx, bson.M{"resource_type": resourceType}).Decode(&deleted)
	if err != nil {
		return nil, err
	}
	return deleted, nil
}

// CreateCustomResource registers a new custom-resource type definition,
// dropping any client-supplied _id. resource_type is validated here and
// nowhere else: the instance routes only accept types that are already
// registered, so this is the one place a bad name could get in.
func (s *Store) CreateCustomResource(ctx context.Context, data bson.M) (bson.M, error) {
	resourceType, _ := data["resource_type"].(string)
	if err := validateResourceType(resourceType); err != nil {
		return nil, err
	}
	return insertReturning(ctx, s.metaData, data)
}

// validateResourceType rejects resource_type values that would collide
// with the meta_data definitions collection or misbehave as a MongoDB
// collection name. Without this, POSTing {"resource_type":"meta_data"} lets
// a later DELETE run DeleteMany({}) directly against meta_data, wiping
// every other registered type definition; names with "$", a NUL byte, or a
// "system." prefix are collection names MongoDB itself refuses, which would
// otherwise surface as a raw driver error instead of a clean 400.
func validateResourceType(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("%w: resource_type is required", ErrInvalidResourceType)
	case name == metaDataCollectionName:
		return fmt.Errorf("%w: %q is reserved for custom resource type definitions", ErrInvalidResourceType, name)
	case strings.HasPrefix(name, "system."):
		return fmt.Errorf("%w: %q uses the reserved \"system.\" prefix", ErrInvalidResourceType, name)
	case strings.ContainsAny(name, "$\x00"):
		return fmt.Errorf("%w: %q contains a character not allowed in a collection name", ErrInvalidResourceType, name)
	default:
		return nil
	}
}

// FindResources lists instances of the given custom resource type matching
// filter, from that type's dynamically named collection.
func (s *Store) FindResources(ctx context.Context, resourceType string, filter bson.M) ([]bson.M, error) {
	return findAll(ctx, s.customResourceCollection(resourceType), filter)
}

// FindResourceByID looks up a single instance of resourceType by id.
func (s *Store) FindResourceByID(ctx context.Context, resourceType, id string) (bson.M, error) {
	oid, err := parseObjectID(id)
	if err != nil {
		return nil, err
	}

	var result bson.M
	if err := s.customResourceCollection(resourceType).FindOne(ctx, bson.M{"_id": oid}).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// CreateResource inserts a new instance of resourceType.
func (s *Store) CreateResource(ctx context.Context, resourceType string, data bson.M) (bson.M, error) {
	return insertReturning(ctx, s.customResourceCollection(resourceType), data)
}

// UpdateResource applies a plain $set update to an instance of
// resourceType, dropping any client-supplied _id.
func (s *Store) UpdateResource(ctx context.Context, resourceType, id string, data bson.M) (bson.M, error) {
	return updateByID(ctx, s.customResourceCollection(resourceType), id, data)
}

// DeleteResource removes an instance of resourceType by id, returning it as
// deleted.
func (s *Store) DeleteResource(ctx context.Context, resourceType, id string) (bson.M, error) {
	oid, err := parseObjectID(id)
	if err != nil {
		return nil, err
	}

	var deleted bson.M
	err = s.customResourceCollection(resourceType).FindOneAndDelete(ctx, bson.M{"_id": oid}).Decode(&deleted)
	if err != nil {
		return nil, err
	}
	return deleted, nil
}

// DeleteAllResources drops every instance of resourceType, used by the
// cascading DELETE /custom-resources/<type> definition delete. Returns the
// number of deleted instances.
func (s *Store) DeleteAllResources(ctx context.Context, resourceType string) (int64, error) {
	result, err := s.customResourceCollection(resourceType).DeleteMany(ctx, bson.M{})
	if err != nil {
		return 0, err
	}
	return result.DeletedCount, nil
}
