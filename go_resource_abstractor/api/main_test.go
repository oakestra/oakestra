package api

import (
	"context"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"

	"go_resource_abstractor/db"
	"go_resource_abstractor/services"
)

// testStore and testRouter are shared by every test in this package: the
// full HTTP handler wired to a throwaway MongoDB instance, exercised
// end-to-end via httptest the same way a real client (scheduler,
// resource_abstractor_client) would see the service.
var (
	testStore  *db.Store
	testRouter http.Handler
)

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests exists separately from TestMain so its defers (container
// teardown, client disconnect) actually run: os.Exit bypasses deferred
// calls in the function that calls it.
func runTests(m *testing.M) int {
	gin.SetMode(gin.TestMode)

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
	testRouter = NewRouter(store, services.NewHooks(store, 3*time.Second, 3*time.Second))

	return m.Run()
}

func uniqueName(prefix string) string {
	return prefix + "-" + bson.NewObjectID().Hex()
}

func newObjectIDHex() string {
	return bson.NewObjectID().Hex()
}
