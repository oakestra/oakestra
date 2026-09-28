package rest

import (
	"net/http"
	"testing"
)

// bson.ObjectID would otherwise JSON-marshal as {"$oid": "..."}, but the
// scheduler and resource_abstractor_client expect _id as a plain string.
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

// Exercises the freshness aggregation through the exact query shape the
// scheduler uses: GET /api/v1/resources/?active=true&<params>&<resources csv>.
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

// "?active=" is present but empty, and marshmallow's Boolean field rejects
// that like any other non-boolean value, so this must 422, not silently
// return everything.
func TestListResourcesEmptyActiveIs422(t *testing.T) {
	rec := doRequest(t, http.MethodGet, "/api/v1/resources/?active=", nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422: %s", rec.Code, rec.Body.String())
	}
}

// marshmallow's fields.Boolean also treats "yes"/"on"/"y" as true, wider
// than strconv.ParseBool's vocabulary.
func TestListResourcesActiveAcceptsMarshmallowTruthyStrings(t *testing.T) {
	for _, v := range []string{"yes", "on", "y", "Yes", "ON"} {
		rec := doRequest(t, http.MethodGet, "/api/v1/resources/?active="+v, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("?active=%s status = %d, want 200: %s", v, rec.Code, rec.Body.String())
		}
	}
}

// marshmallow's default allow_none=False means an explicit null for
// candidate_name must be rejected, unlike the three fields declared
// allow_none=True.
func TestCreateResourceRejectsNullForNonNullableField(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{
		"candidate_name": uniqueName("null-check"),
		"memory":         nil,
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateResourceAcceptsNullForNullableField(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{
		"candidate_name": uniqueName("null-ok"),
		"virtualization": nil,
	})
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
}

// marshmallow's Integer field truncates a fractional float rather than
// rejecting it, so vcpus: 4.7 must be stored as the integer 4.
func TestCreateResourceCoercesFractionalIntegerField(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{
		"candidate_name": uniqueName("int-coerce"),
		"vcpus":          4.7,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	created := decodeJSON[map[string]any](t, rec)
	if created["vcpus"] != float64(4) {
		t.Errorf("vcpus = %v, want 4 (truncated)", created["vcpus"])
	}
}

// virtualization is declared List(fields.String()) in ResourceSchema, so a
// wrong-typed element (e.g. a nested object) must be rejected too, not just
// a non-array value.
func TestCreateResourceRejectsNonStringListElement(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{
		"candidate_name": uniqueName("bad-list-element"),
		"virtualization": []any{map[string]any{"bad": true}},
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateResourceAcceptsStringListElements(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{
		"candidate_name": uniqueName("good-list-element"),
		"virtualization": []any{"docker", "kvm"},
	})
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
}

// csi_drivers is declared List(fields.Raw()), not List(fields.String()),
// so its elements aren't held to the string-only rule that applies to
// virtualization and supported_addons.
func TestCreateResourceAllowsMixedCsiDriversElements(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{
		"candidate_name": uniqueName("csi-raw"),
		"csi_drivers":    []any{map[string]any{"name": "csi.example.com"}, "plain-string"},
	})
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
}

// webargs loads a missing body as an empty mapping, so an empty resource
// POST creates a bare candidate instead of being rejected, unlike the raw
// request.json handlers (jobs, apps).
func TestCreateResourceEmptyBodyAccepted(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/resources/", nil)
	if rec.Code != http.StatusCreated {
		t.Errorf("empty resource POST status = %d, want 201: %s", rec.Code, rec.Body.String())
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

// ResourceController.patch in the Python service raises NotFound, not
// BadRequest, for an invalid id, unlike the GET handler.
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

// Flask's strict_slashes=False means "/resources/<id>" and
// "/resources/<id>/" must both resolve.
func TestGetResourceTrailingSlashResolves(t *testing.T) {
	created := decodeJSON[map[string]any](t, doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{
		"candidate_name": uniqueName("trailing-slash"),
	}))
	id := created["_id"].(string)

	rec := doRequest(t, http.MethodGet, "/api/v1/resources/"+id+"/", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("GET .../%s/ status = %d, want 200: %s", id, rec.Code, rec.Body.String())
	}
}

// An unparsable ?active= value must reject the request, not get dropped
// from the filter silently, matching ResourceFilterSchema's validation.
func TestListResourcesInvalidActiveIs422(t *testing.T) {
	rec := doRequest(t, http.MethodGet, "/api/v1/resources/?active=notabool", nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422: %s", rec.Code, rec.Body.String())
	}
}

// ResourceSchema's typed fields (e.g. memory as Integer) must reject the
// wrong JSON type instead of silently storing it, matching the marshmallow
// validation resources_blueprint.py applies.
func TestCreateResourceRejectsWrongFieldType(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{
		"candidate_name": uniqueName("bad-type"),
		"memory":         "not-a-number",
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422: %s", rec.Code, rec.Body.String())
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
