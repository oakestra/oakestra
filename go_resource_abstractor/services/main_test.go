package services

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"

	"go_resource_abstractor/db"
)

// testStore is a Store connected to a throwaway MongoDB instance shared by
// every test in this package: Hooks looks up hook registrations through
// db.Store, so exercising it end-to-end needs a real hooks collection.
var testStore *db.Store

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests exists separately from TestMain so its defers (container
// teardown, client disconnect) actually run: os.Exit bypasses deferred
// calls in the function that calls it.
func runTests(m *testing.M) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// See db/main_test.go for why this is overridable.
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

	store, err := db.Connect(ctx, uri)
	if err != nil {
		log.Printf("failed to connect store: %v", err)
		return 1
	}
	defer func() { _ = store.Disconnect(context.Background()) }()

	testStore = store
	return m.Run()
}

func uniqueName(prefix string) string {
	return prefix + "-" + bson.NewObjectID().Hex()
}
