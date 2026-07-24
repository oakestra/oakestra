package api

import (
	"net/http"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	rec := doRequest(t, http.MethodGet, "/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "ok")
	}
}
