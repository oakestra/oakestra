package api

import (
	"net/http"
	"testing"
)

func TestCreateHookValidatesEventNames(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/hooks/", map[string]any{
		"hook_name":   uniqueName("hook"),
		"webhook_url": "http://example.invalid/" + uniqueName("webhook"),
		"entity":      "resources",
		"events":      []any{"not_a_real_event"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an invalid event name", rec.Code)
	}
}

// TestCreateAndGetHookRoundTrip is a regression test for the fixed
// find_hook_by_id bug: the Python service queries _id as a raw string
// there, so it can never match and GET /hooks/<id> is always 404. This
// asserts the Go port's ObjectID-based lookup actually finds the hook.
func TestCreateAndGetHookRoundTrip(t *testing.T) {
	createRec := doRequest(t, http.MethodPost, "/api/v1/hooks/", map[string]any{
		"hook_name":   uniqueName("hook"),
		"webhook_url": "http://example.invalid/" + uniqueName("webhook"),
		"entity":      "resources",
		"events":      []any{"post_create"},
	})
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", createRec.Code, createRec.Body.String())
	}
	created := decodeJSON[map[string]any](t, createRec)
	id := created["_id"].(string)

	getRec := doRequest(t, http.MethodGet, "/api/v1/hooks/"+id, nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200: %s", getRec.Code, getRec.Body.String())
	}
}

func TestDeleteHookReturns204(t *testing.T) {
	createRec := doRequest(t, http.MethodPost, "/api/v1/hooks/", map[string]any{
		"hook_name":   uniqueName("hook"),
		"webhook_url": "http://example.invalid/" + uniqueName("webhook"),
		"entity":      "resources",
		"events":      []any{"post_delete"},
	})
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", createRec.Code, createRec.Body.String())
	}
	created := decodeJSON[map[string]any](t, createRec)
	id := created["_id"].(string)

	rec := doRequest(t, http.MethodDelete, "/api/v1/hooks/"+id, nil)
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
}
