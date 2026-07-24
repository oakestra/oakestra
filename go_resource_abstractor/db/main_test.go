package db

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/mongodb"
)

// testStore is a Store connected to a throwaway MongoDB instance shared by
// every test in this package (started once in TestMain), matching the
// real deployment's single-server, multi-database topology.
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

	// Matches the MongoDB version the rest of Oakestra deploys (see
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

	store, err := Connect(ctx, uri)
	if err != nil {
		log.Printf("failed to connect store: %v", err)
		return 1
	}
	defer func() { _ = store.Disconnect(context.Background()) }()

	testStore = store
	return m.Run()
}
