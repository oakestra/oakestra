package rest

import (
	"context"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/oakestra/oakestra/go_resource_abstractor/abstractor"
)

// testClient, testSvc and testRouter are shared by every test in this
// package: the full HTTP handler wired to a throwaway MongoDB instance,
// exercised end-to-end via httptest. testClient stays available directly
// for the tests that need to check what actually landed in MongoDB, e.g.
// the BSON type a number was stored as.
var (
	testClient *mongo.Client
	testSvc    *abstractor.Service
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

	// Overridable so CI can pin a different Mongo version.
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

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	svc, err := abstractor.New(abstractor.Options{
		Client:             client,
		HookConnectTimeout: 3 * time.Second,
		HookRequestTimeout: 3 * time.Second,
		Logger:             logger,
	})
	if err != nil {
		log.Printf("failed to build abstractor service: %v", err)
		return 1
	}
	if err := svc.EnsureIndexes(ctx); err != nil {
		log.Printf("failed to ensure indexes: %v", err)
		return 1
	}

	testClient = client
	testSvc = svc
	testRouter = NewHandler(svc, logger)

	return m.Run()
}

func uniqueName(prefix string) string {
	return prefix + "-" + bson.NewObjectID().Hex()
}

func newObjectIDHex() string {
	return bson.NewObjectID().Hex()
}
