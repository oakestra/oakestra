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

// TestListResourcesEmptyActiveIs422 guards against "?active=" (the key
// present with an empty value) being treated the same as the key being
// absent entirely: marshmallow's Boolean field rejects an empty string
// just like any other non-boolean value, so this must 422 too, not
// silently return an unfiltered list.
func TestListResourcesEmptyActiveIs422(t *testing.T) {
	rec := doRequest(t, http.MethodGet, "/api/v1/resources/?active=", nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422: %s", rec.Code, rec.Body.String())
	}
}

// TestListResourcesActiveAcceptsMarshmallowTruthyStrings guards
// strconv.ParseBool's narrower true/false vocabulary from silently
// replacing marshmallow's fields.Boolean, which also treats "yes"/"on"/"y"
// (and their falsy counterparts) as valid.
func TestListResourcesActiveAcceptsMarshmallowTruthyStrings(t *testing.T) {
	for _, v := range []string{"yes", "on", "y", "Yes", "ON"} {
		rec := doRequest(t, http.MethodGet, "/api/v1/resources/?active="+v, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("?active=%s status = %d, want 200: %s", v, rec.Code, rec.Body.String())
		}
	}
}

// TestCreateResourceRejectsNullForNonNullableField guards against treating
// every field as implicitly nullable: marshmallow's default
// allow_none=False means an explicit JSON null for e.g. candidate_name
// must be rejected, unlike the three fields (virtualization,
// supported_addons, csi_drivers) explicitly declared allow_none=True.
func TestCreateResourceRejectsNullForNonNullableField(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{
		"candidate_name": uniqueName("null-check"),
		"memory":         nil,
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422: %s", rec.Code, rec.Body.String())
	}
}

// TestCreateResourceAcceptsNullForNullableField is
// TestCreateResourceRejectsNullForNonNullableField's counterpart: the three
// list fields declared allow_none=True must still accept an explicit null.
func TestCreateResourceAcceptsNullForNullableField(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{
		"candidate_name": uniqueName("null-ok"),
		"virtualization": nil,
	})
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
}

// TestCreateResourceCoercesFractionalIntegerField is a regression test:
// marshmallow's Integer field (verified against the pinned
// marshmallow~=3.15.0) truncates a fractional float rather than rejecting
// it - Integer().deserialize(4.7) returns 4 - so a JSON body value like
// vcpus: 4.7 must be stored as the integer 4, not rejected and not left as
// the raw float64 encoding/json produces.
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

// TestCreateResourceRejectsNonStringListElement is a regression test:
// virtualization and supported_addons are declared List(fields.String())
// in ResourceSchema, so an element of the wrong type (e.g. a nested
// object) must be rejected, not just the outer value being an array.
func TestCreateResourceRejectsNonStringListElement(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{
		"candidate_name": uniqueName("bad-list-element"),
		"virtualization": []any{map[string]any{"bad": true}},
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422: %s", rec.Code, rec.Body.String())
	}
}

// TestCreateResourceAcceptsStringListElements is
// TestCreateResourceRejectsNonStringListElement's counterpart: a
// virtualization list whose elements are all strings must still be
// accepted.
func TestCreateResourceAcceptsStringListElements(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{
		"candidate_name": uniqueName("good-list-element"),
		"virtualization": []any{"docker", "kvm"},
	})
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
}

// TestCreateResourceAllowsMixedCsiDriversElements guards against
// over-applying the string-element check: csi_drivers is declared
// List(fields.Raw()), not List(fields.String()), so non-string elements
// (e.g. nested objects) must still be accepted there.
func TestCreateResourceAllowsMixedCsiDriversElements(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{
		"candidate_name": uniqueName("csi-raw"),
		"csi_drivers":    []any{map[string]any{"name": "csi.example.com"}, "plain-string"},
	})
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
}

// TestCreateResourceEmptyBodyAccepted guards parity with the Python service,
// where ResourceSchema is an @arguments schema and webargs loads a missing
// body as an empty mapping: an empty resource POST is accepted (creating a
// bare candidate), not rejected. This is the counterpart to the raw
// request.json handlers (jobs, apps) that do reject an empty body.
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

// TestGetResourceTrailingSlashResolves guards against item routes only
// being registered under their bare form: Flask's strict_slashes=False
// means "/resources/<id>" and "/resources/<id>/" must both resolve.
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

// TestListResourcesInvalidActiveIs422 guards against an unparsable
// ?active= value being silently dropped from the filter (which would
// return an unfiltered list) instead of rejecting the request, matching
// ResourceFilterSchema's marshmallow validation on that field.
func TestListResourcesInvalidActiveIs422(t *testing.T) {
	rec := doRequest(t, http.MethodGet, "/api/v1/resources/?active=notabool", nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422: %s", rec.Code, rec.Body.String())
	}
}

// TestCreateResourceRejectsWrongFieldType guards against ResourceSchema's
// typed fields (e.g. memory as Integer) being silently stored with the
// wrong JSON type instead of rejected, matching the marshmallow validation
// resources_blueprint.py applies to POST/PUT/PATCH bodies.
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
