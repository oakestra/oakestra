// Package db implements the MongoDB access layer for the resource
// abstractor. It ports resource-abstractor/db/mongodb_client.py and its
// sibling *_db.py modules: four logical databases (candidates, jobs, hooks,
// custom_resources) reached from a single MongoDB deployment.
package db

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Store bundles the collection handles the service operates on, the same
// four collections db/mongodb_client.py's mongo_init sets up as module-level
// globals.
type Store struct {
	client *mongo.Client

	Candidates *mongo.Collection // candidates.candidates
	Apps       *mongo.Collection // jobs.apps
	Jobs       *mongo.Collection // jobs.jobs
	Hooks      *mongo.Collection // hooks.hooks
	MetaData   *mongo.Collection // custom_resources.meta_data

	customResourcesDB *mongo.Database // custom_resources, for dynamic per-type collections
}

// Connect dials MongoDB at uri, wires up the collection handles for all four
// logical databases, and ensures the same indexes the Python service creates
// at startup exist.
func Connect(ctx context.Context, uri string) (*Store, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("connect to mongo: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("ping mongo: %w", err)
	}

	customResourcesDB := client.Database("custom_resources")

	store := &Store{
		client:            client,
		Candidates:        client.Database("candidates").Collection("candidates"),
		Apps:              client.Database("jobs").Collection("apps"),
		Jobs:              client.Database("jobs").Collection("jobs"),
		Hooks:             client.Database("hooks").Collection("hooks"),
		MetaData:          customResourcesDB.Collection("meta_data"),
		customResourcesDB: customResourcesDB,
	}

	if err := store.ensureIndexes(ctx); err != nil {
		return nil, fmt.Errorf("ensure indexes: %w", err)
	}

	return store, nil
}

// ensureIndexes recreates the unique indexes the Python service defines in
// mongo_init (hooks on (entity, webhook_url) and hook_name, meta_data on
// resource_type), plus the non-unique indexes below that back this service's
// hot-path lookups.
//
// Deviation from the Python service, which relies on collection scans for
// these: candidates and jobs are queried by candidate_name/job_name on every
// worker PUT-upsert heartbeat, and candidates are filtered by
// last_modified_timestamp on every scheduler active-resources query. Those
// scans grow with cluster size (O(nodes) per heartbeat), so we index the
// fields. The indexes only affect performance, not results, so adding them is
// strictly beneficial. They are intentionally non-unique: unlike hook_name,
// candidate_name/job_name are only unique in practice, and a unique index
// would fail to build (aborting startup) against any existing deployment that
// already holds duplicate or legacy documents.
func (s *Store) ensureIndexes(ctx context.Context) error {
	unique := true

	_, err := s.Hooks.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "entity", Value: 1}, {Key: "webhook_url", Value: 1}},
			Options: options.Index().SetUnique(unique),
		},
		{
			Keys:    bson.D{{Key: "hook_name", Value: 1}},
			Options: options.Index().SetUnique(unique),
		},
	})
	if err != nil {
		return fmt.Errorf("hooks indexes: %w", err)
	}

	_, err = s.MetaData.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "resource_type", Value: 1}},
		Options: options.Index().SetUnique(unique),
	})
	if err != nil {
		return fmt.Errorf("meta_data index: %w", err)
	}

	_, err = s.Candidates.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "candidate_name", Value: 1}}},
		{Keys: bson.D{{Key: "last_modified_timestamp", Value: 1}}},
	})
	if err != nil {
		return fmt.Errorf("candidates indexes: %w", err)
	}

	_, err = s.Jobs.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "job_name", Value: 1}},
	})
	if err != nil {
		return fmt.Errorf("jobs index: %w", err)
	}

	return nil
}

// CustomResourceCollection returns the dynamically named collection that
// stores instances of the given custom resource type - the Go equivalent of
// Python's db.db_custom_resources.db[resource_type].
func (s *Store) CustomResourceCollection(resourceType string) *mongo.Collection {
	return s.customResourcesDB.Collection(resourceType)
}

// Disconnect closes the underlying MongoDB client.
func (s *Store) Disconnect(ctx context.Context) error {
	return s.client.Disconnect(ctx)
}
