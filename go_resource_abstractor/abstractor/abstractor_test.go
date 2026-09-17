package abstractor_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/oakestra/oakestra/go_resource_abstractor/abstractor"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

// testSvc and testClient are shared by every test in this package: a
// Service wired to a throwaway MongoDB instance, exercised directly with no
// HTTP layer in between.
var (
	testSvc    *abstractor.Service
	testClient *mongo.Client
)

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests exists separately from TestMain so its defers (container
// teardown, client disconnect) actually run: os.Exit bypasses deferred
// calls in the function that calls it.
func runTests(m *testing.M) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Overridable because some hosts' kernels are incompatible with 8.0's
	// storage engine (SERVER-121912); CI and real deployments should leave
	// this at the default.
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

	svc, err := abstractor.New(abstractor.Options{
		Client:             client,
		HookConnectTimeout: 3 * time.Second,
		HookRequestTimeout: 3 * time.Second,
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		log.Printf("failed to build service: %v", err)
		return 1
	}
	if err := svc.EnsureIndexes(ctx); err != nil {
		log.Printf("failed to ensure indexes: %v", err)
		return 1
	}

	testSvc = svc
	testClient = client

	return m.Run()
}

func uniqueName(prefix string) string {
	return prefix + "-" + bson.NewObjectID().Hex()
}

// registerHook registers a hook for entity/event pointing at a throwaway
// webhook server running handler, cleaning up both at the end of the test.
func registerHook(t *testing.T, entity string, event model.HookEvent, handler http.HandlerFunc) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	created, err := testSvc.Hooks.Create(context.Background(), model.Hook{
		HookName:   model.Ptr(uniqueName("hook")),
		WebhookURL: model.Ptr(server.URL),
		Entity:     model.Ptr(entity),
		Events:     model.Ptr([]model.HookEvent{event}),
	})
	if err != nil {
		t.Fatalf("register hook: %v", err)
	}
	t.Cleanup(func() {
		_ = testSvc.Hooks.Delete(context.Background(), *created.ID)
	})
}

// TestCreateThroughLibraryFiresHooks exercises the library with no HTTP
// layer: a pre_create hook transforms the payload before it's persisted,
// and a post_create hook observes the write afterward.
func TestCreateThroughLibraryFiresHooks(t *testing.T) {
	name := uniqueName("lib-create")

	registerHook(t, "resources", model.EventPreCreate, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		body["transformed_by_hook"] = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})

	postReceived := make(chan map[string]any, 1)
	registerHook(t, "resources", model.EventPostCreate, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		postReceived <- body
		w.WriteHeader(http.StatusOK)
	})

	created, err := testSvc.Resources.Create(context.Background(), model.Resource{
		CandidateName: model.Ptr(name),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if transformed, _ := created.Extra["transformed_by_hook"].(bool); !transformed {
		t.Errorf("expected the pre_create hook's transformation to be persisted, got Extra = %v", created.Extra)
	}

	select {
	case body := <-postReceived:
		if body["entity"] != "resources" {
			t.Errorf("post_create entity = %v, want resources", body["entity"])
		}
		if body["entity_id"] != *created.ID {
			t.Errorf("post_create entity_id = %v, want %v", body["entity_id"], *created.ID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the post_create hook to fire")
	}
}

// TestUpsertThenPatchThroughLibrary exercises Resources.Upsert (both the
// create and update branches) and Resources.Report.
func TestUpsertThenPatchThroughLibrary(t *testing.T) {
	ctx := context.Background()
	name := uniqueName("lib-upsert")

	created, err := testSvc.Resources.Upsert(ctx, model.Resource{CandidateName: model.Ptr(name)})
	if err != nil {
		t.Fatalf("Upsert (create branch): %v", err)
	}

	updated, err := testSvc.Resources.Upsert(ctx, model.Resource{
		CandidateName: model.Ptr(name),
		VCPUs:         model.Ptr(int64(4)),
	})
	if err != nil {
		t.Fatalf("Upsert (update branch): %v", err)
	}
	if *updated.ID != *created.ID {
		t.Errorf("Upsert (update branch) created a new document: %v, want %v", *updated.ID, *created.ID)
	}
	if updated.VCPUs == nil || *updated.VCPUs != 4 {
		t.Errorf("vcpus = %v, want 4", updated.VCPUs)
	}

	reported, err := testSvc.Resources.Report(ctx, *created.ID, model.Resource{CPUPercent: model.Ptr(55.5)})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if reported.CPUPercent == nil || *reported.CPUPercent != 55.5 {
		t.Errorf("cpu_percent = %v, want 55.5", reported.CPUPercent)
	}
	if reported.CPUHistory == nil || len(*reported.CPUHistory) != 1 {
		t.Errorf("cpu_history = %v, want one sample appended by Report", reported.CPUHistory)
	}
}

// A pre_update hook payload must keep an empty instance_list as a present,
// empty array, not collapse it to "absent" the way a plain
// json.Unmarshal-into-interface{} would. It also checks the opposite case:
// a field the patch leaves nil (Candidate) must not appear in the hook
// payload at all, and must leave the stored value untouched.
func TestPreUpdateHookRoundTripPreservesEmptySlice(t *testing.T) {
	ctx := context.Background()

	seen := make(chan map[string]any, 1)
	registerHook(t, "jobs", model.EventPreUpdate, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		seen <- body
		// Echo the payload back unchanged: this hook doesn't transform
		// anything, it only observes what toMap handed it.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})

	created, err := testSvc.Jobs.Create(ctx, model.Job{
		JobName:      model.Ptr(uniqueName("hook-roundtrip")),
		InstanceList: model.Ptr([]model.JobInstance{{InstanceNumber: model.Ptr(int64(1))}}),
		Candidate:    model.Ptr("original-candidate"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := testSvc.Jobs.Update(ctx, *created.ID, model.Job{
		InstanceList: model.Ptr([]model.JobInstance{}),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	select {
	case body := <-seen:
		listRaw, ok := body["instance_list"]
		if !ok {
			t.Errorf("pre_update hook payload missing instance_list key entirely")
		} else if list, ok := listRaw.([]any); !ok || len(list) != 0 {
			t.Errorf("pre_update hook payload instance_list = %#v, want a present, empty array", listRaw)
		}
		if candRaw, present := body["candidate"]; present {
			t.Errorf("pre_update hook payload candidate = %#v, want absent since the patch left it nil", candRaw)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the pre_update hook to fire")
	}

	if updated.Candidate == nil || *updated.Candidate != "original-candidate" {
		t.Errorf("candidate = %v, want left untouched at original-candidate since the patch left it nil", updated.Candidate)
	}
	if updated.InstanceList == nil || len(*updated.InstanceList) != 0 {
		t.Errorf("instance_list = %v, want present and empty after the patch", updated.InstanceList)
	}
}

// TestCreateDefinitionRejectsInvalidResourceTypeNames checks that every
// reserved or malformed resource_type comes back as ErrInvalidResourceType.
func TestCreateDefinitionRejectsInvalidResourceTypeNames(t *testing.T) {
	overlong := make([]byte, 121)
	for i := range overlong {
		overlong[i] = 'a'
	}

	tests := []struct {
		name         string
		resourceType string
	}{
		{"empty", ""},
		{"reserved meta_data", "meta_data"},
		{"system prefix", "system.widgets"},
		{"dollar sign", "wid$get"},
		{"NUL byte", "wid\x00get"},
		{"over 120 bytes", string(overlong)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := testSvc.CustomResources.CreateDefinition(context.Background(), model.CustomResourceDefinition{
				ResourceType: tt.resourceType,
			})
			if !errors.Is(err, abstractor.ErrInvalidResourceType) {
				t.Errorf("CreateDefinition(%q) error = %v, want ErrInvalidResourceType", tt.resourceType, err)
			}
		})
	}
}

// TestListInstancesRejectsDollarFilterKey checks that a filter key starting
// with "$" is rejected before it can reach MongoDB as an operator.
func TestListInstancesRejectsDollarFilterKey(t *testing.T) {
	resourceType := uniqueName("filter-injection")
	if _, err := testSvc.CustomResources.CreateDefinition(context.Background(), model.CustomResourceDefinition{
		ResourceType: resourceType,
	}); err != nil {
		t.Fatalf("CreateDefinition: %v", err)
	}

	_, err := testSvc.CustomResources.ListInstances(context.Background(), resourceType, map[string]string{
		"$where": "sleep(5000)",
	})
	if !errors.Is(err, abstractor.ErrInvalidFilterKey) {
		t.Errorf("ListInstances error = %v, want ErrInvalidFilterKey", err)
	}
}

// TestCloseDrainsPostHooks checks that Close blocks until an in-flight
// post_* hook actually finishes delivering, not just until the write call
// that triggered it returns.
func TestCloseDrainsPostHooks(t *testing.T) {
	ctx := context.Background()

	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	delivered := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-releaseRequest
		delivered <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// A dedicated Service, not the shared testSvc, so Close doesn't tear
	// down the dispatcher every other test in this file depends on. It
	// reuses testClient rather than dialing its own - Close never touches
	// the Mongo client, so sharing is safe.
	svc, err := abstractor.New(abstractor.Options{
		Client:             testClient,
		HookConnectTimeout: 3 * time.Second,
		HookRequestTimeout: 3 * time.Second,
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	created, err := svc.Hooks.Create(ctx, model.Hook{
		HookName:   model.Ptr(uniqueName("hook")),
		WebhookURL: model.Ptr(server.URL),
		Entity:     model.Ptr("resources"),
		Events:     model.Ptr([]model.HookEvent{model.EventPostCreate}),
	})
	if err != nil {
		t.Fatalf("register hook: %v", err)
	}
	defer func() { _ = svc.Hooks.Delete(ctx, *created.ID) }()

	if _, err := svc.Resources.Create(ctx, model.Resource{CandidateName: model.Ptr(uniqueName("close-drain"))}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	select {
	case <-requestStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the async post_create webhook to start")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- svc.Close(context.Background()) }()

	select {
	case <-closeDone:
		t.Fatal("Close returned before the in-flight webhook call finished")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseRequest)

	select {
	case err := <-closeDone:
		if err != nil {
			t.Errorf("Close returned an error after a normal drain: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not return after the in-flight webhook call finished")
	}

	select {
	case <-delivered:
	default:
		t.Error("webhook handler ran but did not report delivery before Close returned")
	}
}
