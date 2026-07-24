package api

import (
	"net/http"
	"testing"
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
