package rest

import (
	"net/http"
	"testing"
)

// A path id that isn't a valid hex ObjectID must be rejected with 400
// rather than reaching the store and 500ing. PatchResource and GetHook are
// exceptions that keep answering 404 for an invalid id.
func TestMalformedIDIs400(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       any
		wantStatus int
	}{
		{"patch job", http.MethodPatch, "/api/v1/jobs/not-an-id", map[string]any{"status": "RUNNING"}, http.StatusBadRequest},
		{"delete job", http.MethodDelete, "/api/v1/jobs/not-an-id", nil, http.StatusBadRequest},
		{"get application", http.MethodGet, "/api/v1/applications/not-an-id", nil, http.StatusBadRequest},
		{"patch application", http.MethodPatch, "/api/v1/applications/not-an-id", map[string]any{"application_name": "x"}, http.StatusBadRequest},
		{"delete application", http.MethodDelete, "/api/v1/applications/not-an-id", nil, http.StatusBadRequest},
		{"patch hook", http.MethodPatch, "/api/v1/hooks/not-an-id", nil, http.StatusBadRequest},
		{"patch resource stays 404", http.MethodPatch, "/api/v1/resources/not-an-id", map[string]any{"cpu_percent": 1}, http.StatusNotFound},
		{"get hook stays 404", http.MethodGet, "/api/v1/hooks/not-an-id", nil, http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequest(t, tt.method, tt.path, tt.body)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
