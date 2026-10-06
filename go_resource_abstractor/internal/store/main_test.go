package store

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/mongotest"
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

	client, cleanup, err := mongotest.Start(ctx, ClientOptions)
	if err != nil {
		log.Printf("failed to start mongodb: %v", err)
		return 1
	}
	defer cleanup()

	store := New(client)
	if err := store.EnsureIndexes(ctx); err != nil {
		log.Printf("failed to ensure indexes: %v", err)
		return 1
	}

	testStore = store
	return m.Run()
}

func uniqueName(prefix string) string {
	return mongotest.UniqueName(prefix)
}
