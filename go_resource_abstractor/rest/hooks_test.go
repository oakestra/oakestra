package rest

import (
	"context"
	"net/http"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
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

// Python's find_hook_by_id queries _id as a raw string, so GET /hooks/<id>
// is always 404 there. This checks the Go port actually finds it.
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

// The Python service passes validate=False on the arguments schema, but
// marshmallow's field-level OneOf validator still runs on load, so an
// invalid event is rejected there too.
func TestPatchHookValidatesEventNames(t *testing.T) {
	created := decodeJSON[map[string]any](t, doRequest(t, http.MethodPost, "/api/v1/hooks/", map[string]any{
		"hook_name":   uniqueName("hook"),
		"webhook_url": "http://example.invalid/" + uniqueName("webhook"),
		"entity":      "resources",
		"events":      []any{"post_create"},
	}))
	id := created["_id"].(string)

	rec := doRequest(t, http.MethodPatch, "/api/v1/hooks/"+id, map[string]any{
		"events": []any{"not_a_real_event"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an invalid event name on PATCH: %s", rec.Code, rec.Body.String())
	}
}

// APIObjectPostHookSchema is an @arguments schema in Python, so an empty
// hook POST loads as {} and is accepted, not rejected.
func TestCreateHookEmptyBodyAccepted(t *testing.T) {
	rec := doRequest(t, http.MethodPost, "/api/v1/hooks/", nil)
	if rec.Code != http.StatusCreated {
		t.Errorf("empty hook POST status = %d, want 201: %s", rec.Code, rec.Body.String())
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

// A hook document hand-edited straight into Mongo with a wrongly typed
// field (webhook_url as a number, as if someone poked it in a shell) must
// not turn GET /hooks or GET /hooks/{id} into a 500 for the whole
// collection. HEAD returned the stored document verbatim; decoding it into
// model.Hook would fail instead.
func TestListAndGetHookToleratesMalformedDocument(t *testing.T) {
	valid := decodeJSON[map[string]any](t, doRequest(t, http.MethodPost, "/api/v1/hooks/", map[string]any{
		"hook_name":   uniqueName("hook-valid"),
		"webhook_url": "http://example.invalid/" + uniqueName("webhook"),
		"entity":      "applications",
		"events":      []any{"post_create"},
	}))
	if _, ok := valid["_id"]; !ok {
		t.Fatalf("create valid hook: missing _id in %v", valid)
	}

	malformedID := bson.NewObjectID()
	malformed := bson.M{
		"_id":         malformedID,
		"hook_name":   uniqueName("hook-broken"),
		"webhook_url": int32(42),
		"entity":      "applications",
		"events":      bson.A{"post_create"},
	}
	if _, err := testClient.Database("hooks").Collection("hooks").
		InsertOne(context.Background(), malformed); err != nil {
		t.Fatalf("insert malformed hook: %v", err)
	}

	listRec := doRequest(t, http.MethodGet, "/api/v1/hooks/", nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", listRec.Code, listRec.Body.String())
	}
	list := decodeJSON[[]map[string]any](t, listRec)
	var sawValid, sawMalformed bool
	for _, h := range list {
		switch h["_id"] {
		case valid["_id"]:
			sawValid = true
		case malformedID.Hex():
			sawMalformed = true
			if webhookURL, ok := h["webhook_url"].(float64); !ok || webhookURL != 42 {
				t.Errorf("malformed hook webhook_url in list = %#v, want 42 verbatim", h["webhook_url"])
			}
		}
	}
	if !sawValid || !sawMalformed {
		t.Fatalf("expected both the valid and malformed hooks in the list, got %v", list)
	}

	getRec := doRequest(t, http.MethodGet, "/api/v1/hooks/"+malformedID.Hex(), nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200: %s", getRec.Code, getRec.Body.String())
	}
	got := decodeJSON[map[string]any](t, getRec)
	if webhookURL, ok := got["webhook_url"].(float64); !ok || webhookURL != 42 {
		t.Errorf("malformed hook webhook_url on GET = %#v, want 42 verbatim", got["webhook_url"])
	}
}
