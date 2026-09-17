package api

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

// TestCustomResourceMalformedSchemaFailsClosed is a regression test:
// validateAgainstSchema used to fail open on a schema that doesn't compile,
// accepting any payload. It now surfaces as a 500 instead.
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

// TestCustomResourceDefinitionRejectsReservedMetaDataName guards the
// data-loss fix end to end: registering "meta_data" as a resource type used
// to let a later delete wipe out every other type definition, because
// instances and definitions shared the same collection. It must now be
// rejected outright, and other definitions must be unaffected.
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

	deleteRec := doRequest(t, http.MethodDelete, "/api/v1/custom-resources/meta_data", nil)
	if deleteRec.Code != http.StatusNotFound {
		t.Errorf("deleting \"meta_data\" status = %d, want 404 (never registered): %s", deleteRec.Code, deleteRec.Body.String())
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
