package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestAppendJobInstanceConflict(t *testing.T) {
	job := decodeJSON[map[string]any](t, doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{
		"job_name":      uniqueName("job"),
		"instance_list": []any{},
	}))
	jobID := job["_id"].(string)

	appendBody := map[string]any{
		"instance_list": []any{map[string]any{"instance_number": float64(1)}},
	}

	first := doRequest(t, http.MethodPut, "/api/v1/jobs/"+jobID+"/1", appendBody)
	if first.Code != http.StatusOK {
		t.Fatalf("first append status = %d, want 200: %s", first.Code, first.Body.String())
	}

	second := doRequest(t, http.MethodPut, "/api/v1/jobs/"+jobID+"/1", appendBody)
	if second.Code != http.StatusBadRequest {
		t.Fatalf("second append status = %d, want 400: %s", second.Code, second.Body.String())
	}
	body := decodeJSON[map[string]any](t, second)
	if body["message"] != "Instance already exists" {
		t.Errorf("message = %v, want %q", body["message"], "Instance already exists")
	}
}

func TestGetJobInstanceMissingIs404(t *testing.T) {
	job := decodeJSON[map[string]any](t, doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{
		"job_name": uniqueName("job"),
	}))
	jobID := job["_id"].(string)

	rec := doRequest(t, http.MethodGet, "/api/v1/jobs/"+jobID+"/999", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestGetJobInvalidIDIs400(t *testing.T) {
	rec := doRequest(t, http.MethodGet, "/api/v1/jobs/not-an-object-id", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// TestGetJobInvalidInstanceNumberIs422 guards against an unparsable
// ?instance_number= value being silently dropped from the filter instead
// of rejecting the request, matching JobFilterSchema's marshmallow
// validation on that field.
func TestGetJobInvalidInstanceNumberIs422(t *testing.T) {
	job := decodeJSON[map[string]any](t, doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{
		"job_name": uniqueName("job"),
	}))
	jobID := job["_id"].(string)

	rec := doRequest(t, http.MethodGet, "/api/v1/jobs/"+jobID+"?instance_number=not-a-number", nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422: %s", rec.Code, rec.Body.String())
	}
}

// TestGetJobEmptyInstanceNumberIs422 guards against "?instance_number="
// (the key present with an empty value) being treated the same as the key
// being absent entirely.
func TestGetJobEmptyInstanceNumberIs422(t *testing.T) {
	job := decodeJSON[map[string]any](t, doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{
		"job_name": uniqueName("job"),
	}))
	jobID := job["_id"].(string)

	rec := doRequest(t, http.MethodGet, "/api/v1/jobs/"+jobID+"?instance_number=", nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422: %s", rec.Code, rec.Body.String())
	}
}

// TestGetJobTrailingSlashResolves guards against item routes only being
// registered under their bare form: Flask's strict_slashes=False means
// "/jobs/<id>" and "/jobs/<id>/" must both resolve.
func TestGetJobTrailingSlashResolves(t *testing.T) {
	job := decodeJSON[map[string]any](t, doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{
		"job_name": uniqueName("job"),
	}))
	jobID := job["_id"].(string)

	rec := doRequest(t, http.MethodGet, "/api/v1/jobs/"+jobID+"/", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("GET .../%s/ status = %d, want 200: %s", jobID, rec.Code, rec.Body.String())
	}
}

func TestUpsertJobCreatesThenUpdatesByName(t *testing.T) {
	name := uniqueName("upsert-job")

	created := decodeJSON[map[string]any](t, doRequest(t, http.MethodPut, "/api/v1/jobs/", map[string]any{
		"job_name": name,
		"status":   "PENDING",
	}))

	updated := decodeJSON[map[string]any](t, doRequest(t, http.MethodPut, "/api/v1/jobs/", map[string]any{
		"job_name": name,
		"status":   "RUNNING",
	}))

	if updated["_id"] != created["_id"] {
		t.Errorf("expected the same job to be updated (upsert), got a different _id: %v vs %v", updated["_id"], created["_id"])
	}
	if updated["status"] != "RUNNING" {
		t.Errorf("status = %v, want RUNNING", updated["status"])
	}
}

// TestUpsertJobUpdatePathFiresJobsHooks is a regression test for the fixed
// entity-name inconsistency in jobs_blueprint.py's PUT /jobs/: the original
// fires hooks under "job" (singular) on the update path while every other
// job route uses "jobs", so a hook registered for "jobs" would never see
// PUT /jobs/ updates. This asserts it now does.
func TestUpsertJobUpdatePathFiresJobsHooks(t *testing.T) {
	name := uniqueName("upsert-job-hook")

	// Seed the job via POST so the PUT below takes the update path.
	doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{"job_name": name, "status": "PENDING"})

	received := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if _, err := testStore.CreateHook(context.Background(), bson.M{
		"hook_name":   uniqueName("hook"),
		"webhook_url": server.URL,
		"entity":      "jobs",
		"events":      bson.A{"post_update"},
	}); err != nil {
		t.Fatalf("register hook: %v", err)
	}

	doRequest(t, http.MethodPut, "/api/v1/jobs/", map[string]any{"job_name": name, "status": "RUNNING"})

	select {
	case body := <-received:
		if body["event"] != "post_update" {
			t.Errorf("event = %v, want post_update", body["event"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("expected the 'jobs' post_update hook to fire for the PUT /jobs/ upsert-update path")
	}
}
