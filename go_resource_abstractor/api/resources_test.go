package api

import (
	"net/http"
	"testing"
)

// TestResourceIDIsPlainStringNotExtendedJSON guards the single most
// important wire-compatibility detail: the Go driver's bson.ObjectID would
// otherwise JSON-marshal as {"$oid": "..."} (MongoDB Extended JSON), but
// both the scheduler and resource_abstractor_client expect _id as a plain
// string, matching Python's json.dumps(doc, default=str).
func TestResourceIDIsPlainStringNotExtendedJSON(t *testing.T) {
	createRec := doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{
		"candidate_name": uniqueName("wire-format"),
	})
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", createRec.Code, createRec.Body.String())
	}

	created := decodeJSON[map[string]any](t, createRec)
	id, ok := created["_id"].(string)
	if !ok {
		t.Fatalf("_id is not a plain string: %#v", created["_id"])
	}
	if len(id) != 24 {
		t.Errorf("_id %q doesn't look like a 24-char hex ObjectID", id)
	}

	getRec := doRequest(t, http.MethodGet, "/api/v1/resources/"+id, nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", getRec.Code)
	}
	got := decodeJSON[map[string]any](t, getRec)
	if got["_id"] != id {
		t.Errorf("_id = %v, want %v", got["_id"], id)
	}
}

// TestListResourcesActiveFilter exercises the freshness aggregation
// end-to-end through the HTTP layer: the scheduler's exact query shape is
// GET /api/v1/resources/?active=true&<params>&<resources csv>.
func TestListResourcesActiveFilter(t *testing.T) {
	name := uniqueName("active-e2e")

	putRec := doRequest(t, http.MethodPut, "/api/v1/resources/", map[string]any{
		"candidate_name": name,
	})
	if putRec.Code != http.StatusOK {
		t.Fatalf("put status = %d, want 200: %s", putRec.Code, putRec.Body.String())
	}
	created := decodeJSON[map[string]any](t, putRec)
	id := created["_id"].(string)

	patchRec := doRequest(t, http.MethodPatch, "/api/v1/resources/"+id, map[string]any{
		"cpu_percent": 42.0,
	})
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200: %s", patchRec.Code, patchRec.Body.String())
	}

	listRec := doRequest(t, http.MethodGet,
		"/api/v1/resources/?active=true&candidate_name="+name+"&resources=active", nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", listRec.Code, listRec.Body.String())
	}
	results := decodeJSON[[]map[string]any](t, listRec)
	if len(results) != 1 {
		t.Fatalf("expected 1 active candidate, got %d: %v", len(results), results)
	}
	if active, _ := results[0]["active"].(bool); !active {
		t.Errorf("expected active=true, got %v", results[0])
	}
}

func TestGetResourceInvalidIDIs400(t *testing.T) {
	rec := doRequest(t, http.MethodGet, "/api/v1/resources/not-an-object-id", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestGetResourceMissingIs404(t *testing.T) {
	rec := doRequest(t, http.MethodGet, "/api/v1/resources/"+newObjectIDHex(), nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// TestPatchResourceInvalidIDIs404NotBadRequest preserves a deliberate
// asymmetry from the Python service: ResourceController.patch raises
// NotFound (not BadRequest) for an invalid id, unlike the GET handler.
func TestPatchResourceInvalidIDIs404NotBadRequest(t *testing.T) {
	rec := doRequest(t, http.MethodPatch, "/api/v1/resources/not-an-object-id", map[string]any{})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDeleteResourceReturns204(t *testing.T) {
	created := decodeJSON[map[string]any](t, doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{
		"candidate_name": uniqueName("to-delete"),
	}))
	id := created["_id"].(string)

	rec := doRequest(t, http.MethodDelete, "/api/v1/resources/"+id, nil)
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
}

func TestUpsertResourceCreatesThenUpdatesByName(t *testing.T) {
	name := uniqueName("upsert")

	first := decodeJSON[map[string]any](t, doRequest(t, http.MethodPut, "/api/v1/resources/", map[string]any{
		"candidate_name": name,
		"vcpus":          float64(2),
	}))

	second := decodeJSON[map[string]any](t, doRequest(t, http.MethodPut, "/api/v1/resources/", map[string]any{
		"candidate_name": name,
		"vcpus":          float64(4),
	}))

	if second["_id"] != first["_id"] {
		t.Errorf("expected the same candidate to be updated, got a different _id: %v vs %v", second["_id"], first["_id"])
	}
	if second["vcpus"] != float64(4) {
		t.Errorf("vcpus = %v, want 4 after upsert-update", second["vcpus"])
	}
}
