package db

import (
	"context"
	"maps"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Application operations

// FindApps lists applications matching filter.
func (s *Store) FindApps(ctx context.Context, filter bson.M) ([]bson.M, error) {
	return findAll(ctx, s.apps, filter)
}

// FindAppByID looks up a single application by id, additionally constrained
// by extraFilter (e.g. userId from query params).
func (s *Store) FindAppByID(ctx context.Context, id string, extraFilter bson.M) (bson.M, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	filter := bson.M{}
	maps.Copy(filter, extraFilter)
	filter["_id"] = oid

	var result bson.M
	if err := s.apps.FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteApp removes an application by id and returns the deleted document.
func (s *Store) DeleteApp(ctx context.Context, id string) (bson.M, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	var deleted bson.M
	if err := s.apps.FindOneAndDelete(ctx, bson.M{"_id": oid}).Decode(&deleted); err != nil {
		return nil, err
	}
	return deleted, nil
}

// UpdateApp applies a plain $set update, dropping any client-supplied _id.
func (s *Store) UpdateApp(ctx context.Context, id string, data bson.M) (bson.M, error) {
	return updateByID(ctx, s.apps, id, data)
}

// CreateApp inserts a new application, populating applicationID with its own
// stringified _id. The id is generated client-side so both fields can be set
// in a single insert, rather than jobs_db.create_app's insert-then-update.
func (s *Store) CreateApp(ctx context.Context, data bson.M) (bson.M, error) {
	delete(data, "_id")
	id := bson.NewObjectID()
	data["_id"] = id
	data["applicationID"] = id.Hex()

	if _, err := s.apps.InsertOne(ctx, data); err != nil {
		return nil, err
	}
	return data, nil
}
