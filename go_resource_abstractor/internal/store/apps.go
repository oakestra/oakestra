package store

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

// FindApps lists applications matching filter.
func (s *Store) FindApps(ctx context.Context, filter bson.M) ([]model.Application, error) {
	return findAll[model.Application](ctx, s.apps, filter)
}

// FindAppByID looks up a single application by id, additionally constrained
// by extraFilter (e.g. userId from query params).
func (s *Store) FindAppByID(ctx context.Context, id string, extraFilter bson.M) (model.Application, error) {
	return findByID[model.Application](ctx, s.apps, id, extraFilter)
}

// DeleteApp removes an application by id and returns the deleted document.
func (s *Store) DeleteApp(ctx context.Context, id string) (model.Application, error) {
	return deleteByIDReturning[model.Application](ctx, s.apps, id)
}

// UpdateApp applies a plain $set update, dropping any client-supplied _id.
func (s *Store) UpdateApp(ctx context.Context, id string, data model.Application) (model.Application, error) {
	return updateByID(ctx, s.apps, id, data)
}

// CreateApp inserts a new application, populating ApplicationID with its
// own stringified _id, generated client-side so both fields can be set in a
// single insert.
func (s *Store) CreateApp(ctx context.Context, data model.Application) (model.Application, error) {
	data.ID = nil

	id := bson.NewObjectID()
	data.ApplicationID = model.Ptr(id.Hex())

	doc, err := toSetDoc(data)
	if err != nil {
		return model.Application{}, err
	}
	doc["_id"] = id

	return insertDoc[model.Application](ctx, s.apps, doc)
}
