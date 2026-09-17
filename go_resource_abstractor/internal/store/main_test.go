package store

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// testStore is a Store connected to a throwaway MongoDB instance shared by
// every test in this package, started once in TestMain with the same
// single-server, multi-database topology as a real deployment.
//
// Store doesn't dial or own the *mongo.Client itself - New only wires up
// collection handles - so TestMain owns dialing, EnsureIndexes and
// Disconnect directly.
var testStore *Store

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests exists separately from TestMain so its defers (container
// teardown, client disconnect) actually run: os.Exit bypasses deferred
// calls in the function that calls it.
func runTests(m *testing.M) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

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
		log.Printf("failed to start mongodb container: %v", err)
		return 1
	}
	defer func() { _ = container.Terminate(context.Background()) }()

	uri, err := container.ConnectionString(ctx)
	if err != nil {
		log.Printf("failed to get mongodb connection string: %v", err)
		return 1
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		log.Printf("failed to connect: %v", err)
		return 1
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	if err := client.Ping(ctx, nil); err != nil {
		log.Printf("failed to ping mongo: %v", err)
		return 1
	}

	store := New(client)
	if err := store.EnsureIndexes(ctx); err != nil {
		log.Printf("failed to ensure indexes: %v", err)
		return 1
	}

	testStore = store
	return m.Run()
}

// uniqueName returns a collision-free name for test fixtures, since tests
// share one long-lived MongoDB instance and several fields (hook_name,
// resource_type) carry unique indexes.
func uniqueName(prefix string) string {
	return prefix + "-" + bson.NewObjectID().Hex()
}
