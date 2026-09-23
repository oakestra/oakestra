package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/oakestra/oakestra/go_resource_abstractor/model"
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

// A JSON integer in an untyped field must persist as a BSON integer, not a
// BSON double, so this reads the document straight out of Mongo instead of
// through the service's typed model, which would mask the bug.
func TestJobIntegerFieldStoredAsBSONInteger(t *testing.T) {
	created := decodeJSON[map[string]any](t, doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{
		"job_name": uniqueName("job"),
		"replicas": 3,
	}))

	oid, err := bson.ObjectIDFromHex(created["_id"].(string))
	if err != nil {
		t.Fatalf("parse created _id: %v", err)
	}

	var raw bson.M
	if err := testClient.Database("jobs").Collection("jobs").
		FindOne(context.Background(), bson.M{"_id": oid}).Decode(&raw); err != nil {
		t.Fatalf("read raw job: %v", err)
	}
	switch raw["replicas"].(type) {
	case int32, int64:
		// stored as a BSON integer, as Python/pymongo would
	default:
		t.Errorf("replicas stored as %T, want a BSON integer (not double)", raw["replicas"])
	}
}

// Python's request.json rejects an absent body, so a bodyless POST must
// 400 rather than silently create a document from {}.
func TestCreateJobEmptyBodyRejected(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/jobs/", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an empty body: %s", rec.Code, rec.Body.String())
	}
}

func TestGetJobFilteredByInstanceNumber(t *testing.T) {
	job := decodeJSON[map[string]any](t, doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{
		"job_name":      uniqueName("job"),
		"instance_list": []any{},
	}))
	jobID := job["_id"].(string)

	appendRec := doRequest(t, http.MethodPut, "/api/v1/jobs/"+jobID+"/1", map[string]any{
		"instance_list": []any{map[string]any{"instance_number": 1}},
	})
	if appendRec.Code != http.StatusOK {
		t.Fatalf("append status = %d, want 200: %s", appendRec.Code, appendRec.Body.String())
	}

	hit := doRequest(t, http.MethodGet, "/api/v1/jobs/"+jobID+"?instance_number=1", nil)
	if hit.Code != http.StatusOK {
		t.Errorf("instance_number=1 status = %d, want 200: %s", hit.Code, hit.Body.String())
	}

	miss := doRequest(t, http.MethodGet, "/api/v1/jobs/"+jobID+"?instance_number=999", nil)
	if miss.Code != http.StatusNotFound {
		t.Errorf("instance_number=999 status = %d, want 404", miss.Code)
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

// An unparsable ?instance_number= value must reject the request, not get
// dropped from the filter silently, matching JobFilterSchema's validation.
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

// "?instance_number=" is present with an empty value, and must not be
// treated the same as the key being absent entirely.
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

// Flask's strict_slashes=False means "/jobs/<id>" and "/jobs/<id>/" must
// both resolve.
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

// jobs_blueprint.py's PUT /jobs/ fired hooks under "job" (singular) on the
// update path while every other route uses "jobs". A "jobs" hook must see
// these updates too.
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

	if _, err := testSvc.Hooks.Create(context.Background(), model.Hook{
		HookName:   model.Ptr(uniqueName("hook")),
		WebhookURL: model.Ptr(server.URL),
		Entity:     model.Ptr("jobs"),
		Events:     model.Ptr([]model.HookEvent{model.EventPostUpdate}),
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

// rawJob reads a job straight out of Mongo, bypassing the service's typed
// model, so a test can tell "key absent" apart from "key present holding an
// empty/null value".
func rawJob(t *testing.T, id string) bson.M {
	t.Helper()

	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		t.Fatalf("parse job id %q: %v", id, err)
	}

	var raw bson.M
	if err := testClient.Database("jobs").Collection("jobs").
		FindOne(context.Background(), bson.M{"_id": oid}).Decode(&raw); err != nil {
		t.Fatalf("read raw job: %v", err)
	}
	return raw
}

// An explicit empty instance_list must stay present (as []) in both the
// response and the stored document, not vanish the way a plain []T with
// "omitempty" on its bson tag would.
func TestUpsertJobEmptyInstanceListPreserved(t *testing.T) {
	name := uniqueName("empty-instance-list")

	rec := doRequest(t, http.MethodPut, "/api/v1/jobs/", map[string]any{
		"job_name":      name,
		"instance_list": []any{},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	created := decodeJSON[map[string]any](t, rec)

	list, ok := created["instance_list"]
	if !ok {
		t.Fatalf("response missing instance_list key entirely: %v", created)
	}
	if arr, ok := list.([]any); !ok || len(arr) != 0 {
		t.Errorf("response instance_list = %#v, want a present, empty array", list)
	}

	raw := rawJob(t, created["_id"].(string))
	rawList, present := raw["instance_list"]
	if !present {
		t.Fatalf("stored document missing instance_list key entirely: %v", raw)
	}
	if arr, ok := rawList.(bson.A); !ok || len(arr) != 0 {
		t.Errorf("stored instance_list = %#v, want a present, empty array", rawList)
	}
}

// An explicit JSON null on a typed field decodes with the field left nil,
// same as an absent key, but a PATCH sending it must clear the stored
// value to null rather than leaving it untouched: HEAD's jobs_db.update_job
// pops only "_id" off the request body and $sets the rest verbatim, nulls
// included, so a caller that explicitly sends null gets exactly that
// written and read back.
func TestPatchJobCandidateNullClearsField(t *testing.T) {
	created := decodeJSON[map[string]any](t, doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{
		"job_name":  uniqueName("job"),
		"candidate": "some-candidate-id",
	}))
	id := created["_id"].(string)

	patchRec := doRequest(t, http.MethodPatch, "/api/v1/jobs/"+id, map[string]any{"candidate": nil})
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200: %s", patchRec.Code, patchRec.Body.String())
	}
	updated := decodeJSON[map[string]any](t, patchRec)

	if raw, ok := updated["candidate"]; !ok || raw != nil {
		t.Errorf("response candidate = %#v, want present holding null", updated["candidate"])
	}

	raw := rawJob(t, id)
	if v, ok := raw["candidate"]; !ok || v != nil {
		t.Errorf("stored candidate = %#v, want present holding null", raw["candidate"])
	}
}

// A PATCH that omits a field entirely (as opposed to sending it as an
// explicit null, see TestPatchJobCandidateNullClearsField) must still leave
// the stored value alone; see TestPatchJobAbsentFieldsUntouched below.

// A whole-job PATCH replacing instance_list must preserve an explicit null
// on one of its elements' fields, the same as a top-level field: the
// element is replaced wholesale, so there's no "leave untouched" semantics
// to apply within it, and HEAD stored (and returned) exactly what the
// request body held for each instance.
func TestPatchJobInstanceListElementNullPreserved(t *testing.T) {
	created := decodeJSON[map[string]any](t, doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{
		"job_name": uniqueName("job"),
		"instance_list": []any{
			map[string]any{"instance_number": 1, "status": "RUNNING", "host_ip": "10.0.0.1"},
		},
	}))
	id := created["_id"].(string)

	patchRec := doRequest(t, http.MethodPatch, "/api/v1/jobs/"+id, map[string]any{
		"instance_list": []any{
			map[string]any{"instance_number": 1, "status": nil, "host_ip": "10.0.0.1"},
		},
	})
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200: %s", patchRec.Code, patchRec.Body.String())
	}
	updated := decodeJSON[map[string]any](t, patchRec)

	list, ok := updated["instance_list"].([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("instance_list = %#v, want 1 entry", updated["instance_list"])
	}
	instance, ok := list[0].(map[string]any)
	if !ok {
		t.Fatalf("instance_list[0] = %#v, want an object", list[0])
	}
	if v, present := instance["status"]; !present || v != nil {
		t.Errorf("response status = %#v, want present holding null", instance["status"])
	}
	if instance["host_ip"] != "10.0.0.1" {
		t.Errorf("response host_ip = %v, want 10.0.0.1", instance["host_ip"])
	}

	raw := rawJob(t, id)
	rawList, ok := raw["instance_list"].(bson.A)
	if !ok || len(rawList) != 1 {
		t.Fatalf("stored instance_list = %#v, want 1 entry", raw["instance_list"])
	}
	// A nested document inside a bson.M-decoded field comes back as bson.D,
	// not bson.M - bson.M only applies at the level the caller asked to
	// decode into.
	rawInstance, ok := rawList[0].(bson.D)
	if !ok {
		t.Fatalf("stored instance_list[0] = %#v, want a document", rawList[0])
	}
	var status any
	var statusPresent bool
	for _, elem := range rawInstance {
		if elem.Key == "status" {
			status, statusPresent = elem.Value, true
		}
	}
	if !statusPresent || status != nil {
		t.Errorf("stored status = %#v, want present holding null", status)
	}
}

// A PATCH sending an explicit empty instance_list must actually clear any
// existing instances, not get silently dropped the way an empty slice with
// "omitempty" would be.
func TestPatchJobEmptyInstanceListClears(t *testing.T) {
	created := decodeJSON[map[string]any](t, doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{
		"job_name":      uniqueName("job"),
		"instance_list": []any{map[string]any{"instance_number": 1}},
	}))
	id := created["_id"].(string)

	patchRec := doRequest(t, http.MethodPatch, "/api/v1/jobs/"+id, map[string]any{"instance_list": []any{}})
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200: %s", patchRec.Code, patchRec.Body.String())
	}
	updated := decodeJSON[map[string]any](t, patchRec)

	list, ok := updated["instance_list"]
	if !ok {
		t.Fatalf("response missing instance_list key entirely: %v", updated)
	}
	if arr, ok := list.([]any); !ok || len(arr) != 0 {
		t.Errorf("response instance_list = %#v, want a present, empty array (cleared by the patch)", list)
	}

	raw := rawJob(t, id)
	rawList, present := raw["instance_list"]
	if !present {
		t.Fatalf("stored document missing instance_list key entirely: %v", raw)
	}
	if arr, ok := rawList.(bson.A); !ok || len(arr) != 0 {
		t.Errorf("stored instance_list = %#v, want a present, empty array (cleared by the patch)", rawList)
	}
}

// A PATCH that omits a field must leave the stored value alone.
func TestPatchJobAbsentFieldsUntouched(t *testing.T) {
	created := decodeJSON[map[string]any](t, doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{
		"job_name":      uniqueName("job"),
		"candidate":     "keep-me",
		"instance_list": []any{map[string]any{"instance_number": 1}},
	}))
	id := created["_id"].(string)

	// A patch that only touches an unrelated (Extra) field.
	patchRec := doRequest(t, http.MethodPatch, "/api/v1/jobs/"+id, map[string]any{"status": "RUNNING"})
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200: %s", patchRec.Code, patchRec.Body.String())
	}
	updated := decodeJSON[map[string]any](t, patchRec)

	if updated["candidate"] != "keep-me" {
		t.Errorf("candidate = %v, want unchanged %q", updated["candidate"], "keep-me")
	}
	list, ok := updated["instance_list"].([]any)
	if !ok || len(list) != 1 {
		t.Errorf("instance_list = %#v, want unchanged (1 element)", updated["instance_list"])
	}
	if updated["status"] != "RUNNING" {
		t.Errorf("status = %v, want RUNNING", updated["status"])
	}
}
