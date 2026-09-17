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
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/oakestra/oakestra/go_resource_abstractor/abstractor"
	"github.com/oakestra/oakestra/go_resource_abstractor/internal/mongotest"
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

	client, cleanup, err := mongotest.Start(ctx, abstractor.MongoClientOptions)
	if err != nil {
		log.Printf("failed to start mongodb: %v", err)
		return 1
	}
	defer cleanup()

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
	return mongotest.UniqueName(prefix)
}

func newObjectIDHex() string {
	return bson.NewObjectID().Hex()
}
