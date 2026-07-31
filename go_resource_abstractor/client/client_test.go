package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oakestra/oakestra/go_resource_abstractor/client/openapi"
)

// stub serves body with status for every request, recording the last one it
// saw. Most tests below only care about one half or the other.
type stub struct {
	method  string
	path    string
	query   string
	headers http.Header
	body    map[string]any
}

func serve(t *testing.T, status int, body string) (*Client, *stub) {
	t.Helper()

	var got stub
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path, got.query = r.Method, r.URL.Path, r.URL.RawQuery
		got.headers = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&got.body)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != "" {
			_, _ = w.Write([]byte(body))
		}
	}))
	t.Cleanup(srv.Close)

	return New(srv.URL), &got
}

// generated returns the generated client underneath a Client, which is where
// the base URL and the *http.Client end up once New has resolved its Options.
func generated(t *testing.T, c *Client) *openapi.Client {
	t.Helper()

	inner, ok := c.api.ClientInterface.(*openapi.Client)
	if !ok {
		t.Fatalf("generated client is %T, want *openapi.Client", c.api.ClientInterface)
	}
	return inner
}

// --- response mapping ----------------------------------------------------

func TestResponse_DecodesSuccessPayload(t *testing.T) {
	c, _ := serve(t, http.StatusOK, `{"candidate_name":"worker-1"}`)

	res, err := c.Resources.GetByID(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("GetByID: unexpected error: %v", err)
	}
	if res.CandidateName == nil || *res.CandidateName != "worker-1" {
		t.Errorf("GetByID: got %+v, want candidate_name=worker-1", res)
	}
}

// TestResponse_UnknownFieldsSurviveTheRoundTrip is the guarantee that makes
// the generated models usable against a service whose documents are
// schemaless: every document schema in openapi.yaml sets additionalProperties,
// so a field the spec never mentions has to come back rather than be dropped
// on decode.
func TestResponse_UnknownFieldsSurviveTheRoundTrip(t *testing.T) {
	c, _ := serve(t, http.StatusOK, `{"candidate_name":"worker-1","gpu_temp":61}`)

	res, err := c.Resources.GetByID(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("GetByID: unexpected error: %v", err)
	}
	got, found := res.Get("gpu_temp")
	if !found {
		t.Fatalf("GetByID: gpu_temp missing from %+v, want it in AdditionalProperties", res)
	}
	if got != float64(61) {
		t.Errorf("GetByID: gpu_temp = %v (%T), want 61", got, got)
	}
}

func TestResponse_NotFoundMapsToErrNotFound(t *testing.T) {
	c, _ := serve(t, http.StatusNotFound, "")

	if _, err := c.Resources.GetByID(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID: got err %v, want ErrNotFound", err)
	}
}

func TestResponse_NonNotFoundErrorMapsToAPIError(t *testing.T) {
	c, _ := serve(t, http.StatusInternalServerError, `{"message":"Internal Server Error"}`)

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
	if apiErr.Method != http.MethodGet || apiErr.Path != "/api/v1/resources/id" {
		t.Errorf("APIError = %s %s, want GET /api/v1/resources/id", apiErr.Method, apiErr.Path)
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("a 500 must not satisfy errors.Is(err, ErrNotFound)")
	}
}

func TestResponse_EmptyBodyOnSuccessDoesNotError(t *testing.T) {
	c, _ := serve(t, http.StatusNoContent, "")

	if err := c.Apps.Delete(context.Background(), "app-1"); err != nil {
		t.Fatalf("Delete: unexpected error on empty 204 body: %v", err)
	}
}

// TestResponse_EmptyListIsNotNil keeps list methods returning something a
// caller can range over unconditionally, whether the service answered with
// [] or with nothing at all.
func TestResponse_EmptyListIsNotNil(t *testing.T) {
	c, _ := serve(t, http.StatusOK, `[]`)

	jobs, err := c.Jobs.List(context.Background())
	if err != nil {
		t.Fatalf("List: unexpected error: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("List: got %d jobs, want 0", len(jobs))
	}
}

func TestResponse_TransportErrorIsNotAnAPIErrorOrNotFound(t *testing.T) {
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

// --- request encoding ----------------------------------------------------

func TestRequest_BodyIsJSONWithContentType(t *testing.T) {
	c, got := serve(t, http.StatusOK, `{"_id":"new-id"}`)

	_, err := c.Resources.Create(context.Background(), Resource{CandidateName: Ptr("worker-2")})
	if err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}
	if ct := got.headers.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if got.body["candidate_name"] != "worker-2" {
		t.Errorf("request body = %v, want candidate_name=worker-2", got.body)
	}
}

func TestRequest_NoFiltersSendNoQuery(t *testing.T) {
	c, got := serve(t, http.StatusOK, `[]`)

	if _, err := c.Resources.List(context.Background()); err != nil {
		t.Fatalf("List: unexpected error: %v", err)
	}
	if got.query != "" {
		t.Errorf("query = %q, want empty when no filters are given", got.query)
	}
}

// TestFilters_BuildTheDocumentedQuery pins each filter option to the query
// parameter openapi.yaml declares for it. A filter wired to the wrong field
// is silently ignored by the server, so nothing else would catch it.
func TestFilters_BuildTheDocumentedQuery(t *testing.T) {
	cases := []struct {
		name string
		call func(c *Client) error
		want string
	}{
		{
			name: "Active",
			call: func(c *Client) error { _, err := c.Resources.List(context.Background(), Active()); return err },
			want: "active=true",
		},
		{
			name: "NamedCandidate",
			call: func(c *Client) error {
				_, err := c.Resources.List(context.Background(), NamedCandidate("worker-1"))
				return err
			},
			want: "candidate_name=worker-1",
		},
		{
			name: "CandidateIP",
			call: func(c *Client) error {
				_, err := c.Resources.List(context.Background(), CandidateIP("10.0.0.1"))
				return err
			},
			want: "ip=10.0.0.1",
		},
		{
			name: "RunningJob",
			call: func(c *Client) error {
				_, err := c.Resources.List(context.Background(), RunningJob("j-1"))
				return err
			},
			want: "job_id=j-1",
		},
		{
			name: "OfApplication",
			call: func(c *Client) error {
				_, err := c.Jobs.List(context.Background(), OfApplication("app-1"))
				return err
			},
			want: "applicationID=app-1",
		},
		{
			name: "NamedJob",
			call: func(c *Client) error { _, err := c.Jobs.List(context.Background(), NamedJob("j")); return err },
			want: "job_name=j",
		},
		{
			name: "OfUser",
			call: func(c *Client) error { _, err := c.Apps.List(context.Background(), OfUser("u-1")); return err },
			want: "userId=u-1",
		},
		{
			name: "NamedApp",
			call: func(c *Client) error { _, err := c.Apps.List(context.Background(), NamedApp("a")); return err },
			want: "application_name=a",
		},
		{
			name: "InNamespace",
			call: func(c *Client) error {
				_, err := c.Apps.List(context.Background(), InNamespace("default"))
				return err
			},
			want: "application_namespace=default",
		},
		{
			name: "combined",
			call: func(c *Client) error {
				_, err := c.Apps.List(context.Background(), NamedApp("a"), InNamespace("default"))
				return err
			},
			want: "application_name=a&application_namespace=default",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, got := serve(t, http.StatusOK, `[]`)
			if err := tc.call(c); err != nil {
				t.Fatalf("%s: unexpected error: %v", tc.name, err)
			}
			if got.query != tc.want {
				t.Errorf("%s: query = %q, want %q", tc.name, got.query, tc.want)
			}
		})
	}
}

// TestFilters_ProjectionIsCommaSeparated pins the one parameter whose
// encoding is not the obvious one: openapi.yaml declares ?resources= as
// `style: form, explode: false`, so the generated client has to send a single
// comma-joined value rather than one key per field.
func TestFilters_ProjectionIsCommaSeparated(t *testing.T) {
	c, got := serve(t, http.StatusOK, `[]`)

	_, err := c.Resources.List(context.Background(), Fields("cpu_percent", "memory_percent"))
	if err != nil {
		t.Fatalf("List: unexpected error: %v", err)
	}
	if got.query != "resources=cpu_percent,memory_percent" {
		t.Errorf("query = %q, want a single comma-separated resources value", got.query)
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
	// The generated constructor appends the trailing slash it resolves the
	// spec's paths against.
	want := "http://cluster_resource_abstractor:11012/"
	if got := generated(t, c).Server; got != want {
		t.Errorf("Server = %q, want %q", got, want)
	}
}

// TestWithTimeout_CombinesWithHTTPClientRegardlessOfOptionOrder guards
// against Option application being order-dependent: WithTimeout must apply
// to whichever *http.Client the Client ends up using no matter which
// option was passed first.
func TestWithTimeout_CombinesWithHTTPClientRegardlessOfOptionOrder(t *testing.T) {
	custom := &http.Client{}

	for _, tc := range []struct {
		name string
		opts []Option
	}{
		{"WithTimeout then WithHTTPClient", []Option{WithTimeout(7 * time.Second), WithHTTPClient(custom)}},
		{"WithHTTPClient then WithTimeout", []Option{WithHTTPClient(custom), WithTimeout(7 * time.Second)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doer := generated(t, New("http://example.invalid", tc.opts...)).Client
			httpClient, ok := doer.(*http.Client)
			if !ok {
				t.Fatalf("request doer is %T, want *http.Client", doer)
			}
			if httpClient.Timeout != 7*time.Second {
				t.Errorf("Timeout = %v, want 7s", httpClient.Timeout)
			}
		})
	}

	if custom.Timeout != 0 {
		t.Errorf("caller's own *http.Client was mutated: Timeout = %v, want 0", custom.Timeout)
	}
}

// --- method -> HTTP path/verb mapping ------------------------------------

// TestMethodRouting is what catches a facade method wired to the wrong
// generated operation - the paths and verbs themselves come from
// openapi.yaml, so nothing else in this package asserts them.
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
			call:       func(c *Client) error { _, err := c.Apps.List(context.Background()); return err },
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
				_, err := c.Apps.Create(context.Background(), "user-1", Application{})
				return err
			},
			wantMethod: http.MethodPost, wantPath: "/api/v1/applications",
		},
		{
			name: "Apps.Update",
			call: func(c *Client) error {
				_, err := c.Apps.Update(context.Background(), "app-1", "user-1", Application{})
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
			call:       func(c *Client) error { _, err := c.Resources.List(context.Background()); return err },
			wantMethod: http.MethodGet, wantPath: "/api/v1/resources", respBody: "[]",
		},
		{
			name: "Resources.GetByID",
			call: func(c *Client) error {
				_, err := c.Resources.GetByID(context.Background(), "r-1")
				return err
			},
			wantMethod: http.MethodGet, wantPath: "/api/v1/resources/r-1",
		},
		{
			name: "Resources.Create",
			call: func(c *Client) error {
				_, err := c.Resources.Create(context.Background(), Resource{})
				return err
			},
			wantMethod: http.MethodPut, wantPath: "/api/v1/resources",
		},
		{
			name: "Resources.UpdateInformation",
			call: func(c *Client) error {
				_, err := c.Resources.UpdateInformation(context.Background(), "r-1", Resource{})
				return err
			},
			wantMethod: http.MethodPatch, wantPath: "/api/v1/resources/r-1",
		},
		{
			name:       "Jobs.List",
			call:       func(c *Client) error { _, err := c.Jobs.List(context.Background()); return err },
			wantMethod: http.MethodGet, wantPath: "/api/v1/jobs", respBody: "[]",
		},
		{
			name: "Jobs.GetByID",
			call: func(c *Client) error {
				_, err := c.Jobs.GetByID(context.Background(), "j-1")
				return err
			},
			wantMethod: http.MethodGet, wantPath: "/api/v1/jobs/j-1",
		},
		{
			name: "Jobs.Create",
			call: func(c *Client) error {
				_, err := c.Jobs.Create(context.Background(), Job{})
				return err
			},
			wantMethod: http.MethodPut, wantPath: "/api/v1/jobs",
		},
		{
			name: "Jobs.Update",
			call: func(c *Client) error {
				_, err := c.Jobs.Update(context.Background(), "j-1", Job{})
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
				_, err := c.Jobs.AppendInstance(context.Background(), "j-1", 2, JobInstanceAppend{})
				return err
			},
			wantMethod: http.MethodPut, wantPath: "/api/v1/jobs/j-1/2",
		},
		{
			name: "Jobs.UpdateInstance",
			call: func(c *Client) error {
				_, err := c.Jobs.UpdateInstance(context.Background(), "j-1", 2, JobInstance{})
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
		{
			name:       "Health",
			call:       func(c *Client) error { return c.Health(context.Background()) },
			wantMethod: http.MethodGet, wantPath: "/",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			respBody := tc.respBody
			if respBody == "" {
				respBody = "{}"
			}

			c, got := serve(t, http.StatusOK, respBody)
			if err := tc.call(c); err != nil {
				t.Fatalf("%s: unexpected error: %v", tc.name, err)
			}
			if got.method != tc.wantMethod {
				t.Errorf("%s: method = %s, want %s", tc.name, got.method, tc.wantMethod)
			}
			if got.path != tc.wantPath {
				t.Errorf("%s: path = %s, want %s", tc.name, got.path, tc.wantPath)
			}
		})
	}
}

// --- "first or ErrNotFound" lookups --------------------------------------

func TestFirstOrLookups_EmptyResultIsErrNotFound(t *testing.T) {
	c, _ := serve(t, http.StatusOK, `[]`)

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
	c, got := serve(t, http.StatusOK, `[{"candidate_name":"first"},{"candidate_name":"second"}]`)

	res, err := c.Resources.GetByName(context.Background(), "first")
	if err != nil {
		t.Fatalf("GetByName: unexpected error: %v", err)
	}
	if res.CandidateName == nil || *res.CandidateName != "first" {
		t.Errorf("GetByName: got %+v, want the first element of the result list", res)
	}
	if got.query != "candidate_name=first" {
		t.Errorf("GetByName: query = %q, want candidate_name=first", got.query)
	}
}

// --- request bodies with injected/derived fields -------------------------

func TestJobs_UpdateStatus_OmitsEmptyDetail(t *testing.T) {
	c, got := serve(t, http.StatusOK, `{}`)

	if _, err := c.Jobs.UpdateStatus(context.Background(), "j-1", "RUNNING", ""); err != nil {
		t.Fatalf("UpdateStatus: unexpected error: %v", err)
	}
	if _, ok := got.body["status_detail"]; ok {
		t.Errorf("UpdateStatus: body has status_detail %v, want it omitted when empty", got.body["status_detail"])
	}
	if got.body["status"] != "RUNNING" {
		t.Errorf("UpdateStatus: body[status] = %v, want RUNNING", got.body["status"])
	}
}

func TestJobs_UpdateStatus_IncludesDetailWhenSet(t *testing.T) {
	c, got := serve(t, http.StatusOK, `{}`)

	if _, err := c.Jobs.UpdateStatus(context.Background(), "j-1", "ERROR", "crash loop"); err != nil {
		t.Fatalf("UpdateStatus: unexpected error: %v", err)
	}
	if got.body["status_detail"] != "crash loop" {
		t.Errorf("UpdateStatus: body[status_detail] = %v, want %q", got.body["status_detail"], "crash loop")
	}
}

func TestApps_Create_InjectsUserIDWithoutMutatingCaller(t *testing.T) {
	c, got := serve(t, http.StatusOK, `{}`)

	app := Application{ApplicationName: Ptr("my-app")}
	if _, err := c.Apps.Create(context.Background(), "user-42", app); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}
	if got.body["userId"] != "user-42" {
		t.Errorf("Create: body[userId] = %v, want user-42", got.body)
	}
	if app.UserId != nil {
		t.Errorf("Create: caller's application was mutated: UserId = %q", *app.UserId)
	}
}

// TestApps_ListByUser_LeavesCallerFiltersUntouched guards the reason
// ListByUser sets UserId on the built query instead of appending OfUser to
// the caller's filters: a slice with spare capacity would be written into.
func TestApps_ListByUser_LeavesCallerFiltersUntouched(t *testing.T) {
	c, got := serve(t, http.StatusOK, `[]`)

	filters := make([]AppFilter, 1, 4) // room to grow, so a stray append lands in it
	filters[0] = InNamespace("default")

	if _, err := c.Apps.ListByUser(context.Background(), "user-42", filters...); err != nil {
		t.Fatalf("ListByUser: unexpected error: %v", err)
	}
	if got.query != "application_namespace=default&userId=user-42" {
		t.Errorf("ListByUser: query = %q, want both the caller's filter and userId", got.query)
	}
	if len(filters) != 1 {
		t.Errorf("ListByUser: caller's filters grew to %d entries", len(filters))
	}
	if filters[0] == nil {
		t.Error("ListByUser: caller's filters were overwritten")
	}
}

// --- optional-field helpers ----------------------------------------------

func TestValue_UnsetFieldIsTheZeroValue(t *testing.T) {
	var job Job

	if got := Value(job.JobName); got != "" {
		t.Errorf("Value(nil *string) = %q, want the zero value", got)
	}
	if got := Value(Ptr("my-job")); got != "my-job" {
		t.Errorf("Value(Ptr(%q)) = %q, want my-job", "my-job", got)
	}
}
