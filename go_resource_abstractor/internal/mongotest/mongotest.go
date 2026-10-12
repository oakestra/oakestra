// Package mongotest starts a throwaway MongoDB for package tests, with the
// same single-server, multi-database topology as a real deployment.
//
// Shared by the store, abstractor and rest suites so a Mongo version bump
// or a testcontainers API change is one edit rather than three.
package mongotest

import (
	"context"
	"fmt"
	"os"

	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Start boots a MongoDB container and returns a client connected to it,
// plus a cleanup that disconnects the client and terminates the container.
// Skipping cleanup leaks the container.
//
// clientOpts is a parameter rather than a direct store.ClientOptions call so
// internal/store's own in-package tests can use this without an import
// cycle. Everyone else passes abstractor.MongoClientOptions.
func Start(
	ctx context.Context,
	clientOpts func(uri string) *options.ClientOptions,
) (*mongo.Client, func(), error) {
	// Same MongoDB version the rest of Oakestra deploys (see
	// docker-compose.yml). Overridable because some hosts' kernels are
	// incompatible with 8.0's storage engine (SERVER-121912); CI and real
	// deployments should leave this at the default.
	image := "mongo:8.0"
	if v := os.Getenv("MONGO_TEST_IMAGE"); v != "" {
		image = v
	}

	container, err := mongodb.Run(ctx, image)
	if err != nil {
		return nil, nil, fmt.Errorf("start mongodb container: %w", err)
	}
	stopContainer := func() { _ = container.Terminate(context.Background()) }

	uri, err := container.ConnectionString(ctx)
	if err != nil {
		stopContainer()
		return nil, nil, fmt.Errorf("get mongodb connection string: %w", err)
	}

	client, err := mongo.Connect(clientOpts(uri))
	if err != nil {
		stopContainer()
		return nil, nil, fmt.Errorf("connect to mongodb: %w", err)
	}

	cleanup := func() {
		_ = client.Disconnect(context.Background())
		stopContainer()
	}

	if err := client.Ping(ctx, nil); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("ping mongodb: %w", err)
	}
	return client, cleanup, nil
}

// UniqueName returns a collision-free name for test fixtures, since tests
// share one long-lived MongoDB instance and several fields (hook_name,
// resource_type) carry unique indexes.
func UniqueName(prefix string) string {
	return prefix + "-" + bson.NewObjectID().Hex()
}
