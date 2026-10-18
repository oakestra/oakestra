package rest

import (
	"context"
	"net/http"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestCreateApplicationSetsApplicationID(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/applications/", map[string]any{
		"application_name": uniqueName("app"),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	created := decodeJSON[map[string]any](t, rec)
	if created["applicationID"] != created["_id"] {
		t.Errorf("applicationID = %v, want %v (own _id)", created["applicationID"], created["_id"])
	}
}

// An explicit empty microservices array must stay present (as []) in both
// the response and the stored document, not vanish the way a plain []T
// with "omitempty" on its bson tag would.
func TestCreateApplicationEmptyMicroservicesPreserved(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/applications/", map[string]any{
		"application_name": uniqueName("app"),
		"microservices":    []any{},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	created := decodeJSON[map[string]any](t, rec)

	ms, ok := created["microservices"]
	if !ok {
		t.Fatalf("response missing microservices key entirely: %v", created)
	}
	if arr, ok := ms.([]any); !ok || len(arr) != 0 {
		t.Errorf("response microservices = %#v, want a present, empty array", ms)
	}

	oid, err := bson.ObjectIDFromHex(created["_id"].(string))
	if err != nil {
		t.Fatalf("parse created _id: %v", err)
	}
	var raw bson.M
	if err := testClient.Database("jobs").Collection("apps").
		FindOne(context.Background(), bson.M{"_id": oid}).Decode(&raw); err != nil {
		t.Fatalf("read raw application: %v", err)
	}
	rawMS, present := raw["microservices"]
	if !present {
		t.Fatalf("stored document missing microservices key entirely: %v", raw)
	}
	if arr, ok := rawMS.(bson.A); !ok || len(arr) != 0 {
		t.Errorf("stored microservices = %#v, want a present, empty array", rawMS)
	}
}

func TestApplicationGetPatchDeleteRoundTrip(t *testing.T) {
	created := decodeJSON[map[string]any](t, doRequest(t, http.MethodPost, "/api/v1/applications/", map[string]any{
		"application_name": uniqueName("app"),
	}))
	id := created["_id"].(string)

	getRec := doRequest(t, http.MethodGet, "/api/v1/applications/"+id, nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", getRec.Code)
	}

	patchRec := doRequest(t, http.MethodPatch, "/api/v1/applications/"+id, map[string]any{
		"application_namespace": "custom-ns",
	})
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200: %s", patchRec.Code, patchRec.Body.String())
	}
	updated := decodeJSON[map[string]any](t, patchRec)
	if updated["application_namespace"] != "custom-ns" {
		t.Errorf("application_namespace = %v, want custom-ns", updated["application_namespace"])
	}

	deleteRec := doRequest(t, http.MethodDelete, "/api/v1/applications/"+id, nil)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200: %s", deleteRec.Code, deleteRec.Body.String())
	}

	afterDeleteRec := doRequest(t, http.MethodGet, "/api/v1/applications/"+id, nil)
	if afterDeleteRec.Code != http.StatusNotFound {
		t.Errorf("status after delete = %d, want 404: %s", afterDeleteRec.Code, afterDeleteRec.Body.String())
	}
}

func TestListApplicationsFilter(t *testing.T) {
	name := uniqueName("filtered-app")
	doRequest(t, http.MethodPost, "/api/v1/applications/", map[string]any{"application_name": name})
	doRequest(t, http.MethodPost, "/api/v1/applications/", map[string]any{"application_name": uniqueName("other-app")})

	rec := doRequest(t, http.MethodGet, "/api/v1/applications/?application_name="+name, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	results := decodeJSON[[]map[string]any](t, rec)
	if len(results) != 1 {
		t.Fatalf("expected 1 matching application, got %d: %v", len(results), results)
	}
	if results[0]["application_name"] != name {
		t.Errorf("application_name = %v, want %v", results[0]["application_name"], name)
	}
}
