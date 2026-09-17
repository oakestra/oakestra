// Package store implements the MongoDB access layer for the resource
// abstractor: typed CRUD over candidates, jobs, applications, hooks and
// custom resources, spread across four logical databases in one MongoDB
// deployment. Store doesn't dial or own the *mongo.Client it's built from -
// New takes an existing client, and the caller is responsible for
// Disconnect.
package store

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// metaDataCollectionName is the custom_resources collection that stores
// type definitions. It doubles as the reserved name validateResourceType
// rejects for an instance type, so the two can't drift apart.
const metaDataCollectionName = "meta_data"

// Store bundles the collection handles the service operates on.
type Store struct {
	candidates *mongo.Collection // candidates.candidates
	apps       *mongo.Collection // jobs.apps
	jobs       *mongo.Collection // jobs.jobs
	hooks      *mongo.Collection // hooks.hooks
	metaData   *mongo.Collection // custom_resources.meta_data

	customResourcesDB *mongo.Database // custom_resources, for dynamic per-type collections
}

// hexIDOptions makes a collection decode an ObjectID `_id` as its hex
// string, matching model types' `ID *string` field. Without it, the
// driver's default string codec refuses to decode an ObjectID into a
// string at all.
func hexIDOptions() options.Lister[options.CollectionOptions] {
	return options.Collection().SetBSONOptions(&options.BSONOptions{ObjectIDAsHexString: true})
}

// New wires up collection handles for client but doesn't dial, ping, or
// create indexes. The caller owns client's lifecycle, including Disconnect;
// call EnsureIndexes separately, typically right after New.
func New(client *mongo.Client) *Store {
	customResourcesDB := client.Database("custom_resources")

	return &Store{
		candidates:        client.Database("candidates").Collection("candidates", hexIDOptions()),
		apps:              client.Database("jobs").Collection("apps", hexIDOptions()),
		jobs:              client.Database("jobs").Collection("jobs", hexIDOptions()),
		hooks:             client.Database("hooks").Collection("hooks", hexIDOptions()),
		metaData:          customResourcesDB.Collection(metaDataCollectionName, hexIDOptions()),
		customResourcesDB: customResourcesDB,
	}
}

// EnsureIndexes creates the unique indexes on hooks (entity, webhook_url)
// and hook_name, and on meta_data's resource_type, plus non-unique indexes
// on candidate_name, job_name and last_modified_timestamp, which are hit on
// every worker heartbeat and scheduler query. The candidate/job name
// indexes are non-unique because that uniqueness only holds in practice,
// not by contract - a unique index could fail to build against an existing
// deployment with legacy duplicates.
//
// Separate from New so a caller managing its own index lifecycle, or
// running several replicas that would otherwise race to create the same
// indexes on startup, can skip or sequence it.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	_, err := s.hooks.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "entity", Value: 1}, {Key: "webhook_url", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "hook_name", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	})
	if err != nil {
		return fmt.Errorf("hooks indexes: %w", err)
	}

	_, err = s.metaData.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "resource_type", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("meta_data index: %w", err)
	}

	_, err = s.candidates.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "candidate_name", Value: 1}}},
		{Keys: bson.D{{Key: "last_modified_timestamp", Value: 1}}},
	})
	if err != nil {
		return fmt.Errorf("candidates indexes: %w", err)
	}

	_, err = s.jobs.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "job_name", Value: 1}},
	})
	if err != nil {
		return fmt.Errorf("jobs index: %w", err)
	}

	return nil
}

// customResourceCollection returns the dynamically named collection that
// stores instances of the given custom resource type, in the
// custom_resources database. It can't reach the candidates, jobs, hooks or
// apps collections, which live in separate databases.
//
// resourceType isn't validated here: callers only reach this with a type
// CreateCustomResource has already validated.
func (s *Store) customResourceCollection(resourceType string) *mongo.Collection {
	return s.customResourcesDB.Collection(resourceType, hexIDOptions())
}
