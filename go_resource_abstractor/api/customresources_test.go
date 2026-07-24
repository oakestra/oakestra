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

func TestCustomResourceDefinitionRequiresResourceType(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/custom-resources/", map[string]any{
		"schema": map[string]any{"type": "object"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 when resource_type is missing", rec.Code)
	}
}
