package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/errs"
)

// parseObjectID decodes id as a hex ObjectID, wrapping bson's error in
// errs.ErrInvalidID so callers can check errors.Is(err, errs.ErrInvalidID)
// regardless of what the driver itself returns.
func parseObjectID(id string) (bson.ObjectID, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return bson.ObjectID{}, fmt.Errorf("%w: %q", errs.ErrInvalidID, id)
	}
	return oid, nil
}

// toSetDoc marshals v to BSON and back into a bson.M with "_id" dropped,
// giving a document safe to use as a $set operand or the base of a fresh
// insert. Centralizing the drop here means no write path can forget it.
//
// Field mapping is entirely v's own bson struct tags, including the
// `,inline` Extra field every model type carries; this only adds the _id
// drop on top.
func toSetDoc(v any) (bson.M, error) {
	raw, err := bson.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal update document: %w", err)
	}

	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("decode update document: %w", err)
	}
	delete(doc, "_id")
	return doc, nil
}

// findAll decodes every document matching filter (nil matches all) into T.
func findAll[T any](ctx context.Context, coll *mongo.Collection, filter bson.M) ([]T, error) {
	if filter == nil {
		filter = bson.M{}
	}

	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	var results []T
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	// cursor.All leaves results nil when nothing matched, which encodes as
	// JSON null. List endpoints must answer [] instead.
	if results == nil {
		results = []T{}
	}
	return results, nil
}

// findOne decodes the single document matching filter into T, mapping a
// miss to errs.ErrNotFound.
func findOne[T any](ctx context.Context, coll *mongo.Collection, filter bson.M) (T, error) {
	var v T
	if err := coll.FindOne(ctx, filter).Decode(&v); err != nil {
		var zero T
		return zero, wrapNotFound(err)
	}
	return v, nil
}

// findByID decodes id as a hex ObjectID and looks it up via findOne,
// additionally constrained by extra (e.g. a caller-supplied query filter).
// extra may be nil.
func findByID[T any](ctx context.Context, coll *mongo.Collection, id string, extra bson.M) (T, error) {
	var zero T

	oid, err := parseObjectID(id)
	if err != nil {
		return zero, err
	}

	filter := bson.M{}
	maps.Copy(filter, extra)
	filter["_id"] = oid

	return findOne[T](ctx, coll, filter)
}

// IsValidID reports whether id parses as a hex ObjectID, for callers that
// need to treat a malformed id differently from errs.ErrInvalidID's usual
// handling without depending on the driver themselves.
func IsValidID(id string) bool {
	_, err := parseObjectID(id)
	return err == nil
}

// insertReturning inserts v (dropping any caller-supplied _id via toSetDoc)
// and returns it as stored, with the server-assigned _id filled in.
func insertReturning[T any](ctx context.Context, coll *mongo.Collection, v T) (T, error) {
	doc, err := toSetDoc(v)
	if err != nil {
		var zero T
		return zero, err
	}
	return insertDoc[T](ctx, coll, doc)
}

// insertDoc inserts doc, generating its _id client-side if it has none, and
// decodes doc itself back into T instead of re-reading it. A generic T has
// no way to set its own ID field, but doc already is exactly what was
// stored (toSetDoc has normalized it through BSON), so decoding it with
// decodeHexID gives the same result as a FindOne without the round trip.
func insertDoc[T any](ctx context.Context, coll *mongo.Collection, doc bson.M) (T, error) {
	var zero T

	if _, ok := doc["_id"]; !ok {
		doc["_id"] = bson.NewObjectID()
	}
	if _, err := coll.InsertOne(ctx, doc); err != nil {
		return zero, err
	}

	raw, err := bson.Marshal(doc)
	if err != nil {
		return zero, err
	}
	return decodeHexID[T](raw)
}

// updateByID applies v as a $set update to the document with the given id
// and returns it as stored. Returns errs.ErrNotFound if no document
// matches.
func updateByID[T any](ctx context.Context, coll *mongo.Collection, id string, v T) (T, error) {
	var zero T

	oid, err := parseObjectID(id)
	if err != nil {
		return zero, err
	}

	doc, err := toSetDoc(v)
	if err != nil {
		return zero, err
	}
	return findOneAndUpdate[T](ctx, coll, bson.M{"_id": oid}, bson.M{"$set": doc})
}

// findOneAndUpdate applies update to the document matching filter and
// returns it as it is after the update, mapping a miss to errs.ErrNotFound.
func findOneAndUpdate[T any](ctx context.Context, coll *mongo.Collection, filter, update bson.M) (T, error) {
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var v T
	if err := coll.FindOneAndUpdate(ctx, filter, update, opts).Decode(&v); err != nil {
		var zero T
		return zero, wrapNotFound(err)
	}
	return v, nil
}

// findOneAndDelete removes the document matching filter and returns it as
// it was before deletion, mapping a miss to errs.ErrNotFound.
func findOneAndDelete[T any](ctx context.Context, coll *mongo.Collection, filter bson.M) (T, error) {
	var v T
	if err := coll.FindOneAndDelete(ctx, filter).Decode(&v); err != nil {
		var zero T
		return zero, wrapNotFound(err)
	}
	return v, nil
}

// deleteByIDReturning decodes id as a hex ObjectID and removes the matching
// document, returning what it removed.
func deleteByIDReturning[T any](ctx context.Context, coll *mongo.Collection, id string) (T, error) {
	var zero T

	oid, err := parseObjectID(id)
	if err != nil {
		return zero, err
	}

	return findOneAndDelete[T](ctx, coll, bson.M{"_id": oid})
}

// deleteByID decodes id as a hex ObjectID and removes the matching
// document, discarding its content.
func deleteByID(ctx context.Context, coll *mongo.Collection, id string) error {
	oid, err := parseObjectID(id)
	if err != nil {
		return err
	}
	_, err = coll.DeleteOne(ctx, bson.M{"_id": oid})
	return err
}

// aggregateAll runs pipeline and decodes every result into T through
// decodeHexID.
//
// Aggregate needs its own decode path: the v2 driver builds an aggregation
// cursor from the client's BSONOptions instead of the collection's, so
// hexIDOptions (which every collection handle here carries) never reaches
// it, even though Find, FindOne, FindOneAndUpdate and FindOneAndDelete all
// pick it up fine.
func aggregateAll[T any](ctx context.Context, coll *mongo.Collection, pipeline mongo.Pipeline) ([]T, error) {
	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	results := []T{}
	for cursor.Next(ctx) {
		v, err := decodeHexID[T](cursor.Current)
		if err != nil {
			return nil, err
		}
		results = append(results, v)
	}
	return results, cursor.Err()
}

// decodeHexID decodes raw into T with ObjectIDAsHexString enabled, so an
// ObjectID `_id` lands in T's `ID *string` field as its hex string. Used
// wherever a document didn't come through a collection handle carrying
// hexIDOptions: Aggregate results and insertDoc's locally built document.
func decodeHexID[T any](raw bson.Raw) (T, error) {
	var v T
	dec := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(raw)))
	dec.ObjectIDAsHexString()
	err := dec.Decode(&v)
	return v, err
}

// wrapNotFound maps mongo.ErrNoDocuments to errs.ErrNotFound so callers
// don't need the driver to check for a miss.
func wrapNotFound(err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return errs.ErrNotFound
	}
	return err
}
