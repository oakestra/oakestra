package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// --- request core -----------------------------------------------------

func TestDo_DecodesSuccessResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"candidate_name":"worker-1"}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	doc, err := c.Resources.GetByID(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("GetByID: unexpected error: %v", err)
	}
	if doc["candidate_name"] != "worker-1" {
		t.Errorf("GetByID: got %v, want candidate_name=worker-1", doc)
	}
}

func TestDo_NotFoundMapsToErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.Resources.GetByID(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID: got err %v, want ErrNotFound", err)
	}
}

func TestDo_NonNotFoundErrorMapsToAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"Internal Server Error"}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.Resources.GetByID(context.Background(), "id")

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("GetByID: got err %v (%T), want *APIError", err, err)
	}
	if apiErr.Status != http.StatusInternalServerError {
		t.Errorf("APIError.Status = %d, want %d", apiErr.Status, http.StatusInternalServerError)
	}
	if apiErr.Message != "Internal Server Error" {
		t.Errorf("APIError.Message = %q, want %q", apiErr.Message, "Internal Server Error")
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("a 500 must not satisfy errors.Is(err, ErrNotFound)")
	}
}

func TestDo_EmptyBodyOnSuccessDoesNotError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(srv.URL)
	if err := c.Apps.Delete(context.Background(), "app-1"); err != nil {
		t.Fatalf("Delete: unexpected error on empty 204 body: %v", err)
	}
}

func TestDo_QueryParamsOmitEmptyValues(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.Resources.List(context.Background(), map[string]string{
		"active":         "true",
		"candidate_name": "",
	})
	if err != nil {
		t.Fatalf("List: unexpected error: %v", err)
	}
	if gotQuery != "active=true" {
		t.Errorf("query = %q, want %q (empty-valued keys must be omitted)", gotQuery, "active=true")
	}
}

func TestDo_RequestBodyIsJSONWithContentType(t *testing.T) {
	var gotContentType string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"_id":"new-id"}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.Resources.Create(context.Background(), Document{"candidate_name": "worker-2"})
	if err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotBody["candidate_name"] != "worker-2" {
		t.Errorf("request body = %v, want candidate_name=worker-2", gotBody)
	}
}

func TestDo_TransportErrorIsNotAnAPIErrorOrNotFound(t *testing.T) {
	// A closed server guarantees a connection error rather than any HTTP response.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	c := New(srv.URL)
	_, err := c.Resources.GetByID(context.Background(), "id")
	if err == nil {
		t.Fatal("GetByID: expected a transport error, got nil")
	}
	if errors.Is(err, ErrNotFound) {
		t.Error("a transport error must not satisfy errors.Is(err, ErrNotFound)")
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		t.Error("a transport error must not be an *APIError")
	}
}

// --- construction -------------------------------------------------------

func TestNewFromEnv_MissingVarsErrors(t *testing.T) {
	t.Setenv("RESOURCE_ABSTRACTOR_URL", "")
	t.Setenv("RESOURCE_ABSTRACTOR_PORT", "")
	if _, err := NewFromEnv(); err == nil {
		t.Fatal("NewFromEnv: expected error when env vars are unset, got nil")
	}
}

func TestNewFromEnv_BuildsExpectedBaseURL(t *testing.T) {
	t.Setenv("RESOURCE_ABSTRACTOR_URL", "cluster_resource_abstractor")
	t.Setenv("RESOURCE_ABSTRACTOR_PORT", "11012")

	c, err := NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: unexpected error: %v", err)
	}
	want := "http://cluster_resource_abstractor:11012"
	if c.baseURL != want {
		t.Errorf("baseURL = %q, want %q", c.baseURL, want)
	}
}

// TestWithTimeout_CombinesWithHTTPClientRegardlessOfOptionOrder guards
// against Option application being order-dependent: WithTimeout must apply
// to whichever *http.Client the Client ends up using no matter which
// option was passed first.
func TestWithTimeout_CombinesWithHTTPClientRegardlessOfOptionOrder(t *testing.T) {
	custom := &http.Client{}

	timeoutThenClient := New("http://example.invalid", WithTimeout(7*time.Second), WithHTTPClient(custom))
	if timeoutThenClient.httpClient.Timeout != 7*time.Second {
		t.Errorf("WithTimeout then WithHTTPClient: Timeout = %v, want 7s", timeoutThenClient.httpClient.Timeout)
	}

	clientThenTimeout := New("http://example.invalid", WithHTTPClient(custom), WithTimeout(7*time.Second))
	if clientThenTimeout.httpClient.Timeout != 7*time.Second {
		t.Errorf("WithHTTPClient then WithTimeout: Timeout = %v, want 7s", clientThenTimeout.httpClient.Timeout)
	}
}

// --- method -> HTTP path/verb mapping ------------------------------------

func TestMethodRouting(t *testing.T) {
	cases := []struct {
		name       string
		call       func(c *Client) error
		wantMethod string
		wantPath   string
		// respBody is what the stub server returns; defaults to "{}" (a
		// single document) when empty, since list-returning calls below
		// override it to "[]".
		respBody string
	}{
		{
			name:       "Apps.List",
			call:       func(c *Client) error { _, err := c.Apps.List(context.Background(), nil); return err },
			wantMethod: http.MethodGet, wantPath: "/api/v1/applications", respBody: "[]",
		},
		{
			name: "Apps.GetByID",
			call: func(c *Client) error {
				_, err := c.Apps.GetByID(context.Background(), "app-1", "user-1")
				return err
			},
			wantMethod: http.MethodGet, wantPath: "/api/v1/applications/app-1",
		},
		{
			name: "Apps.Create",
			call: func(c *Client) error {
				_, err := c.Apps.Create(context.Background(), "user-1", Document{"application_name": "a"})
				return err
			},
			wantMethod: http.MethodPost, wantPath: "/api/v1/applications",
		},
		{
			name: "Apps.Update",
			call: func(c *Client) error {
				_, err := c.Apps.Update(context.Background(), "app-1", "user-1", Document{})
				return err
			},
			wantMethod: http.MethodPatch, wantPath: "/api/v1/applications/app-1",
		},
		{
			name:       "Apps.Delete",
			call:       func(c *Client) error { return c.Apps.Delete(context.Background(), "app-1") },
			wantMethod: http.MethodDelete, wantPath: "/api/v1/applications/app-1",
		},
		{
			name:       "Resources.List",
			call:       func(c *Client) error { _, err := c.Resources.List(context.Background(), nil); return err },
			wantMethod: http.MethodGet, wantPath: "/api/v1/resources", respBody: "[]",
		},
		{
			name:       "Resources.Create",
			call:       func(c *Client) error { _, err := c.Resources.Create(context.Background(), Document{}); return err },
			wantMethod: http.MethodPut, wantPath: "/api/v1/resources",
		},
		{
			name: "Resources.UpdateInformation",
			call: func(c *Client) error {
				_, err := c.Resources.UpdateInformation(context.Background(), "r-1", Document{})
				return err
			},
			wantMethod: http.MethodPatch, wantPath: "/api/v1/resources/r-1",
		},
		{
			name:       "Jobs.List",
			call:       func(c *Client) error { _, err := c.Jobs.List(context.Background(), nil); return err },
			wantMethod: http.MethodGet, wantPath: "/api/v1/jobs", respBody: "[]",
		},
		{
			name:       "Jobs.Create",
			call:       func(c *Client) error { _, err := c.Jobs.Create(context.Background(), Document{}); return err },
			wantMethod: http.MethodPut, wantPath: "/api/v1/jobs",
		},
		{
			name: "Jobs.Update",
			call: func(c *Client) error {
				_, err := c.Jobs.Update(context.Background(), "j-1", Document{})
				return err
			},
			wantMethod: http.MethodPatch, wantPath: "/api/v1/jobs/j-1",
		},
		{
			name:       "Jobs.Delete",
			call:       func(c *Client) error { return c.Jobs.Delete(context.Background(), "j-1") },
			wantMethod: http.MethodDelete, wantPath: "/api/v1/jobs/j-1",
		},
		{
			name: "Jobs.GetInstance",
			call: func(c *Client) error {
				_, err := c.Jobs.GetInstance(context.Background(), "j-1", 2)
				return err
			},
			wantMethod: http.MethodGet, wantPath: "/api/v1/jobs/j-1/2",
		},
		{
			name: "Jobs.AppendInstance",
			call: func(c *Client) error {
				_, err := c.Jobs.AppendInstance(context.Background(), "j-1", 2, Document{})
				return err
			},
			wantMethod: http.MethodPut, wantPath: "/api/v1/jobs/j-1/2",
		},
		{
			name: "Jobs.UpdateInstance",
			call: func(c *Client) error {
				_, err := c.Jobs.UpdateInstance(context.Background(), "j-1", 2, Document{})
				return err
			},
			wantMethod: http.MethodPatch, wantPath: "/api/v1/jobs/j-1/2",
		},
		{
			name: "Jobs.DeleteInstance",
			call: func(c *Client) error {
				_, err := c.Jobs.DeleteInstance(context.Background(), "j-1", 2)
				return err
			},
			wantMethod: http.MethodDelete, wantPath: "/api/v1/jobs/j-1/2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			respBody := tc.respBody
			if respBody == "" {
				respBody = "{}"
			}

			var gotMethod, gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(respBody))
			}))
			defer srv.Close()

			c := New(srv.URL)
			if err := tc.call(c); err != nil {
				t.Fatalf("%s: unexpected error: %v", tc.name, err)
			}
			if gotMethod != tc.wantMethod {
				t.Errorf("%s: method = %s, want %s", tc.name, gotMethod, tc.wantMethod)
			}
			if gotPath != tc.wantPath {
				t.Errorf("%s: path = %s, want %s", tc.name, gotPath, tc.wantPath)
			}
		})
	}
}

// --- "first or ErrNotFound" lookups --------------------------------------

func TestFirstOrLookups_EmptyResultIsErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := New(srv.URL)

	if _, err := c.Resources.GetByName(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Resources.GetByName: got %v, want ErrNotFound", err)
	}
	if _, err := c.Resources.GetByIP(context.Background(), "10.0.0.1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Resources.GetByIP: got %v, want ErrNotFound", err)
	}
	if _, err := c.Apps.GetByNameAndNamespace(context.Background(), "app", "ns", "user"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Apps.GetByNameAndNamespace: got %v, want ErrNotFound", err)
	}
}

func TestFirstOrLookups_ReturnsFirstMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"candidate_name":"first"},{"candidate_name":"second"}]`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	doc, err := c.Resources.GetByName(context.Background(), "first")
	if err != nil {
		t.Fatalf("GetByName: unexpected error: %v", err)
	}
	if doc["candidate_name"] != "first" {
		t.Errorf("GetByName: got %v, want the first element of the result list", doc)
	}
}

// --- request bodies with injected/derived fields -------------------------

func TestJobs_UpdateStatus_OmitsEmptyDetail(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	if _, err := c.Jobs.UpdateStatus(context.Background(), "j-1", "RUNNING", ""); err != nil {
		t.Fatalf("UpdateStatus: unexpected error: %v", err)
	}
	if _, ok := gotBody["status_detail"]; ok {
		t.Errorf("UpdateStatus: body has status_detail %v, want it omitted when empty", gotBody["status_detail"])
	}
	if gotBody["status"] != "RUNNING" {
		t.Errorf("UpdateStatus: body[status] = %v, want RUNNING", gotBody["status"])
	}
}

func TestJobs_UpdateStatus_IncludesDetailWhenSet(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	if _, err := c.Jobs.UpdateStatus(context.Background(), "j-1", "ERROR", "crash loop"); err != nil {
		t.Fatalf("UpdateStatus: unexpected error: %v", err)
	}
	if gotBody["status_detail"] != "crash loop" {
		t.Errorf("UpdateStatus: body[status_detail] = %v, want %q", gotBody["status_detail"], "crash loop")
	}
}

func TestApps_Create_InjectsUserIDWithoutMutatingCaller(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	data := Document{"application_name": "my-app"}
	if _, err := c.Apps.Create(context.Background(), "user-42", data); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}
	if gotBody["userId"] != "user-42" {
		t.Errorf("Create: body[userId] = %v, want user-42", gotBody["userId"])
	}
	if _, mutated := data["userId"]; mutated {
		t.Errorf("Create: caller's data map was mutated: %v", data)
	}
}
