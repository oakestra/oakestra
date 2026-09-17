package rest

import (
	"net/http"
	"testing"
)

func TestCustomResourceInstanceSchemaValidation(t *testing.T) {
	resourceType := uniqueName("widget")

	defRec := doRequest(t, http.MethodPost, "/api/v1/custom-resources/", map[string]any{
		"resource_type": resourceType,
		"schema": map[string]any{
			"type":     "object",
			"required": []any{"name"},
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
		},
	})
	if defRec.Code != http.StatusCreated {
		t.Fatalf("definition create status = %d, want 201: %s", defRec.Code, defRec.Body.String())
	}

	invalidRec := doRequest(t, http.MethodPost, "/api/v1/custom-resources/"+resourceType, map[string]any{
		"not_name": "oops",
	})
	if invalidRec.Code != http.StatusBadRequest {
		t.Errorf("status for a schema-invalid instance = %d, want 400: %s", invalidRec.Code, invalidRec.Body.String())
	}

	validRec := doRequest(t, http.MethodPost, "/api/v1/custom-resources/"+resourceType, map[string]any{
		"name": "widget-1",
	})
	if validRec.Code != http.StatusOK {
		t.Errorf("status for a valid instance = %d, want 200: %s", validRec.Code, validRec.Body.String())
	}
}

// A schema that doesn't compile must fail closed with a 500, not silently
// accept any payload.
func TestCustomResourceMalformedSchemaFailsClosed(t *testing.T) {
	resourceType := uniqueName("broken-schema")

	defRec := doRequest(t, http.MethodPost, "/api/v1/custom-resources/", map[string]any{
		"resource_type": resourceType,
		"schema": map[string]any{
			"$ref": "#/definitions/does-not-exist",
		},
	})
	if defRec.Code != http.StatusCreated {
		t.Fatalf("definition create status = %d, want 201: %s", defRec.Code, defRec.Body.String())
	}

	rec := doRequest(t, http.MethodPost, "/api/v1/custom-resources/"+resourceType, map[string]any{
		"anything": "goes",
	})
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status with a malformed stored schema = %d, want 500 (fail closed): %s", rec.Code, rec.Body.String())
	}
}

func TestCustomResourceCascadingDeleteViaAPI(t *testing.T) {
	resourceType := uniqueName("gadget")

	doRequest(t, http.MethodPost, "/api/v1/custom-resources/", map[string]any{"resource_type": resourceType})
	doRequest(t, http.MethodPost, "/api/v1/custom-resources/"+resourceType, map[string]any{"name": "instance-1"})

	deleteRec := doRequest(t, http.MethodDelete, "/api/v1/custom-resources/"+resourceType, nil)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200: %s", deleteRec.Code, deleteRec.Body.String())
	}

	listRec := doRequest(t, http.MethodGet, "/api/v1/custom-resources/"+resourceType, nil)
	if listRec.Code != http.StatusNotFound {
		t.Errorf("listing instances of a deleted type: status = %d, want 404", listRec.Code)
	}
}

// "meta_data" is the collection where definitions themselves live, so
// registering it as a resource type would let a later delete wipe out
// every other definition. It must be rejected outright, leaving other
// definitions unaffected.
func TestCustomResourceDefinitionRejectsReservedMetaDataName(t *testing.T) {
	resourceType := uniqueName("survivor")

	createRec := doRequest(t, http.MethodPost, "/api/v1/custom-resources/", map[string]any{
		"resource_type": resourceType,
	})
	if createRec.Code != http.StatusCreated {
		t.Fatalf("definition create status = %d, want 201: %s", createRec.Code, createRec.Body.String())
	}

	metaRec := doRequest(t, http.MethodPost, "/api/v1/custom-resources/", map[string]any{
		"resource_type": "meta_data",
	})
	if metaRec.Code != http.StatusBadRequest {
		t.Fatalf("registering \"meta_data\" status = %d, want 400: %s", metaRec.Code, metaRec.Body.String())
	}
	msg := decodeJSON[map[string]any](t, metaRec)["message"]
	if msg == "" || msg == nil {
		t.Errorf("registering \"meta_data\": message = %v, want a non-empty reason", msg)
	}

	// Deleting "meta_data" gets the same 400 as registering it, since a
	// bare 404 could be misread as "not registered yet, but a fine name".
	deleteRec := doRequest(t, http.MethodDelete, "/api/v1/custom-resources/meta_data", nil)
	if deleteRec.Code != http.StatusBadRequest {
		t.Errorf("deleting \"meta_data\" status = %d, want 400 (reserved name): %s", deleteRec.Code, deleteRec.Body.String())
	}

	survivedRec := doRequest(t, http.MethodGet, "/api/v1/custom-resources/"+resourceType, nil)
	if survivedRec.Code != http.StatusOK {
		t.Errorf("listing instances of %q after the rejected registration: status = %d, want 200: %s", resourceType, survivedRec.Code, survivedRec.Body.String())
	}
}

func TestCustomResourceDefinitionRequiresResourceType(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/custom-resources/", map[string]any{
		"schema": map[string]any{"type": "object"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 when resource_type is missing", rec.Code)
	}
}

// A name over MongoDB's 120-byte collection-name limit can never become a
// valid collection, so it must be rejected up front instead of failing as
// a raw driver error on the first write.
func TestCustomResourceDefinitionRejectsOverlongName(t *testing.T) {
	overlong := make([]byte, 121)
	for i := range overlong {
		overlong[i] = 'a'
	}

	rec := doRequest(t, http.MethodPost, "/api/v1/custom-resources/", map[string]any{
		"resource_type": string(overlong),
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a resource_type over 120 bytes: %s", rec.Code, rec.Body.String())
	}
}

// A malformed resourceType in the URL, like one containing "$", must be
// rejected with 400 before the request reaches Mongo, on every instance
// route, not just definition create.
func TestCustomResourceTypeValidationAppliesToInstanceOperations(t *testing.T) {
	badType := "bad$type"

	tests := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"list instances", http.MethodGet, "/api/v1/custom-resources/" + badType, nil},
		{"create instance", http.MethodPost, "/api/v1/custom-resources/" + badType, map[string]any{"name": "x"}},
		{"get instance", http.MethodGet, "/api/v1/custom-resources/" + badType + "/" + newObjectIDHex(), nil},
		{"patch instance", http.MethodPatch, "/api/v1/custom-resources/" + badType + "/" + newObjectIDHex(), map[string]any{"name": "x"}},
		{"delete instance", http.MethodDelete, "/api/v1/custom-resources/" + badType + "/" + newObjectIDHex(), nil},
		{"delete definition", http.MethodDelete, "/api/v1/custom-resources/" + badType, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequest(t, tt.method, tt.path, tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 for a \"$\"-containing resource type: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// An unvalidated query parameter key would reach MongoDB verbatim as part
// of the filter document, letting a caller pass an operator like $where
// straight through.
func TestCustomResourceInstanceFilterRejectsOperatorKey(t *testing.T) {
	resourceType := uniqueName("filter-injection")
	registerCustomResourceType(t, resourceType)

	rec := doRequest(t, http.MethodGet, "/api/v1/custom-resources/"+resourceType+"?$where=sleep(5000)", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a \"$\"-prefixed filter key: %s", rec.Code, rec.Body.String())
	}
}

func TestCustomResourceInstanceFilterAcceptsPlainKeys(t *testing.T) {
	resourceType := uniqueName("filter-ok")
	registerCustomResourceType(t, resourceType)
	doRequest(t, http.MethodPost, "/api/v1/custom-resources/"+resourceType, map[string]any{"name": "instance-1"})

	rec := doRequest(t, http.MethodGet, "/api/v1/custom-resources/"+resourceType+"?name=instance-1", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	results := decodeJSON[[]map[string]any](t, rec)
	if len(results) != 1 {
		t.Errorf("got %d instances, want 1 matching name=instance-1", len(results))
	}
}
