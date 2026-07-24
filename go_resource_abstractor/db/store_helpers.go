package db

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// findAll runs filter (nil meaning "match everything") against coll and
// decodes every matching document, the Find+cursor.All boilerplate shared by
// every *_db.py list function.
func findAll(ctx context.Context, coll *mongo.Collection, filter bson.M) ([]bson.M, error) {
	if filter == nil {
		filter = bson.M{}
	}

	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// insertReturning inserts data (with any client-supplied _id stripped) and
// returns it annotated with the server-assigned _id. It avoids the
// refetch-by-InsertedID round trip that create-then-read would otherwise
// need, since a plain insert doesn't transform the document server-side.
func insertReturning(ctx context.Context, coll *mongo.Collection, data bson.M) (bson.M, error) {
	delete(data, "_id")

	res, err := coll.InsertOne(ctx, data)
	if err != nil {
		return nil, err
	}
	data["_id"] = res.InsertedID
	return data, nil
}

// updateByID applies a plain $set update to the document with the given id,
// dropping any client-supplied _id, and returns it as stored. This is the
// find-by-hex-id-then-$set-and-return-after shape shared by every plain
// *_db.py update function.
func updateByID(ctx context.Context, coll *mongo.Collection, id string, data bson.M) (bson.M, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	delete(data, "_id")

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var updated bson.M
	if err := coll.FindOneAndUpdate(ctx, bson.M{"_id": oid}, bson.M{"$set": data}, opts).Decode(&updated); err != nil {
		return nil, err
	}
	return updated, nil
}
