package client

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// stub serves body with status for every request, recording the last one it
// saw. Most tests below only care about one half or the other.
type stub struct {
	method  string
	path    string
	query   string
	host    string
	headers http.Header
	body    map[string]any
}

// stubHandler is the handler shared by serve and rawServer: it records every
// request it sees into got and answers with status/body.
func stubHandler(got *stub, status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path, got.query = r.Method, r.URL.Path, r.URL.RawQuery
		got.host = r.Host
		got.headers = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&got.body)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != "" {
			_, _ = w.Write([]byte(body))
		}
	}
}

func serve(t *testing.T, status int, body string) (*Client, *stub) {
	t.Helper()

	var got stub
	srv := httptest.NewServer(stubHandler(&got, status, body))
	t.Cleanup(srv.Close)

	return New(srv.URL), &got
}

// rawServer is like serve, but hands back the *httptest.Server itself so a
// caller can read the listener's address - what the NewFromEnv tests need to
// build RESOURCE_ABSTRACTOR_URL/PORT from a real address.
func rawServer(t *testing.T, tlsServer bool) (*httptest.Server, *stub) {
	t.Helper()

	var got stub
	handler := stubHandler(&got, http.StatusOK, `[]`)

	var srv *httptest.Server
	if tlsServer {
		srv = httptest.NewTLSServer(handler)
	} else {
		srv = httptest.NewServer(handler)
	}
	t.Cleanup(srv.Close)

	return srv, &got
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

// TestResponse_UnknownFieldsSurviveTheRoundTrip is what makes the generated
// models usable against a schemaless service: every document schema in
// openapi.yaml sets additionalProperties, so a field the spec never
// mentions still comes back on decode instead of getting dropped.
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
	if jobs == nil {
		t.Error("List: got nil slice, want a non-nil empty slice")
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

	_, err := c.Resources.List(context.Background(), ResourceFields("cpu_percent", "memory_percent"))
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
	srv, got := rawServer(t, false)
	host, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", srv.Listener.Addr().String(), err)
	}
	t.Setenv("RESOURCE_ABSTRACTOR_URL", host)
	t.Setenv("RESOURCE_ABSTRACTOR_PORT", port)

	c, err := NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: unexpected error: %v", err)
	}
	if _, err := c.Resources.List(context.Background()); err != nil {
		t.Fatalf("List: unexpected error: %v", err)
	}
	if got.host != srv.Listener.Addr().String() {
		t.Errorf("request reached host %q, want %q", got.host, srv.Listener.Addr().String())
	}
}

// TestNewFromEnv_HostWithSchemeIsUsedAsIs guards against NewFromEnv
// prepending "http://" a second time when RESOURCE_ABSTRACTOR_URL already
// carries a scheme.
func TestNewFromEnv_HostWithSchemeIsUsedAsIs(t *testing.T) {
	cases := []struct {
		name   string
		useTLS bool
		host   func(host string) string
	}{
		{"http scheme", false, func(host string) string { return "http://" + host }},
		{"https scheme", true, func(host string) string { return "https://" + host }},
		{"trailing slash", false, func(host string) string { return "http://" + host + "/" }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := rawServer(t, tc.useTLS)
			host, port, err := net.SplitHostPort(srv.Listener.Addr().String())
			if err != nil {
				t.Fatalf("SplitHostPort(%q): %v", srv.Listener.Addr().String(), err)
			}
			t.Setenv("RESOURCE_ABSTRACTOR_URL", tc.host(host))
			t.Setenv("RESOURCE_ABSTRACTOR_PORT", port)

			var opts []Option
			if tc.useTLS {
				// The TLS server's own client trusts its self-signed
				// certificate; without it the handshake would fail before
				// the request ever reached the handler.
				opts = append(opts, WithHTTPClient(srv.Client()))
			}
			c, err := NewFromEnv(opts...)
			if err != nil {
				t.Fatalf("NewFromEnv: unexpected error: %v", err)
			}
			if _, err := c.Resources.List(context.Background()); err != nil {
				t.Fatalf("List: unexpected error: %v", err)
			}
			if got.host != srv.Listener.Addr().String() {
				t.Errorf("request reached host %q, want %q", got.host, srv.Listener.Addr().String())
			}
		})
	}
}

// TestWithTimeout_CombinesWithHTTPClientRegardlessOfOptionOrder guards
// against Option application being order-dependent: WithTimeout must apply
// to whichever *http.Client the Client ends up using, whichever option was
// passed first.
func TestWithTimeout_CombinesWithHTTPClientRegardlessOfOptionOrder(t *testing.T) {
	custom := &http.Client{}

	// Blocks on the request's own context so it returns as soon as the
	// client's timeout cancels it, instead of sleeping a fixed duration.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	for _, tc := range []struct {
		name string
		opts []Option
	}{
		{"WithTimeout then WithHTTPClient", []Option{WithTimeout(50 * time.Millisecond), WithHTTPClient(custom)}},
		{"WithHTTPClient then WithTimeout", []Option{WithHTTPClient(custom), WithTimeout(50 * time.Millisecond)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := New(srv.URL, tc.opts...)

			_, err := c.Resources.GetByID(context.Background(), "id")
			if err == nil {
				t.Fatal("GetByID: expected the timeout to fire, got nil error")
			}
			if errors.Is(err, ErrNotFound) {
				t.Error("a timeout must not satisfy errors.Is(err, ErrNotFound)")
			}
			var apiErr *APIError
			if errors.As(err, &apiErr) {
				t.Error("a timeout must not be an *APIError")
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
			name:       "Hooks.List",
			call:       func(c *Client) error { _, err := c.Hooks.List(context.Background()); return err },
			wantMethod: http.MethodGet, wantPath: "/api/v1/hooks", respBody: "[]",
		},
		{
			name: "Hooks.GetByID",
			call: func(c *Client) error {
				_, err := c.Hooks.GetByID(context.Background(), "h-1")
				return err
			},
			wantMethod: http.MethodGet, wantPath: "/api/v1/hooks/h-1",
		},
		{
			name: "Hooks.Create",
			call: func(c *Client) error {
				_, err := c.Hooks.Create(context.Background(), Hook{})
				return err
			},
			wantMethod: http.MethodPost, wantPath: "/api/v1/hooks",
		},
		{
			name: "Hooks.Update",
			call: func(c *Client) error {
				_, err := c.Hooks.Update(context.Background(), "h-1", Hook{})
				return err
			},
			wantMethod: http.MethodPatch, wantPath: "/api/v1/hooks/h-1",
		},
		{
			name:       "Hooks.Delete",
			call:       func(c *Client) error { return c.Hooks.Delete(context.Background(), "h-1") },
			wantMethod: http.MethodDelete, wantPath: "/api/v1/hooks/h-1",
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

// --- Hooks ----------------------------------------------------------------

// TestHooks_Delete_EmptyBodyOnSuccessDoesNotError checks the ordinary
// empty-204 case, and that an unknown id isn't reported as ErrNotFound -
// DeleteHook answers 204 either way.
func TestHooks_Delete_EmptyBodyOnSuccessDoesNotError(t *testing.T) {
	c, _ := serve(t, http.StatusNoContent, "")

	if err := c.Hooks.Delete(context.Background(), "missing"); err != nil {
		t.Fatalf("Delete: unexpected error on empty 204 body: %v", err)
	}
}

// TestHooks_Create_DecodesOnCreated checks the one Create method that
// answers 201 rather than 200 (Jobs.Create and Resources.Create upsert via
// PUT and answer 200).
func TestHooks_Create_DecodesOnCreated(t *testing.T) {
	c, _ := serve(t, http.StatusCreated, `{"_id":"h-1"}`)

	hook, err := c.Hooks.Create(context.Background(), Hook{})
	if err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}
	if hook.ID == nil || *hook.ID != "h-1" {
		t.Errorf("Create: got %+v, want _id=h-1", hook)
	}
}

// --- the escape hatch carries the error contract --------------------------

// These tests pin that Decode/Done give a call made through the OpenAPI
// escape hatch the same three-way error split (ErrNotFound, *APIError,
// transport error) as any facade method, using custom resources - the one
// document type still without a facade - as the illustration.

func TestOpenAPI_Decode_DecodesSuccessPayload(t *testing.T) {
	c, _ := serve(t, http.StatusOK, `[{"resource_type":"gpu"}]`)

	defs, err := Decode[[]CustomResourceDefinition](c.OpenAPI().ListCustomResourceDefinitions(context.Background()))
	if err != nil {
		t.Fatalf("ListCustomResourceDefinitions: unexpected error: %v", err)
	}
	if len(defs) != 1 || defs[0].ResourceType != "gpu" {
		t.Errorf("ListCustomResourceDefinitions: got %+v, want one definition named gpu", defs)
	}
}

func TestOpenAPI_Decode_NotFoundMapsToErrNotFound(t *testing.T) {
	c, _ := serve(t, http.StatusNotFound, "")

	_, err := Decode[[]CustomResourceDefinition](c.OpenAPI().ListCustomResourceDefinitions(context.Background()))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("ListCustomResourceDefinitions: got err %v, want ErrNotFound", err)
	}
}

func TestOpenAPI_Decode_NonNotFoundErrorMapsToAPIError(t *testing.T) {
	c, _ := serve(t, http.StatusInternalServerError, `{"message":"boom"}`)

	_, err := Decode[[]CustomResourceDefinition](c.OpenAPI().ListCustomResourceDefinitions(context.Background()))

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("ListCustomResourceDefinitions: got err %v (%T), want *APIError", err, err)
	}
	if apiErr.Status != http.StatusInternalServerError {
		t.Errorf("APIError.Status = %d, want %d", apiErr.Status, http.StatusInternalServerError)
	}
	if apiErr.Message != "boom" {
		t.Errorf("APIError.Message = %q, want %q", apiErr.Message, "boom")
	}
	if errors.Is(err, ErrNotFound) {
		t.Error("a 500 must not satisfy errors.Is(err, ErrNotFound)")
	}
}

func TestOpenAPI_Done_MapsSuccessAndNotFoundTheSameWayAsDecode(t *testing.T) {
	c, _ := serve(t, http.StatusNoContent, "")
	if err := Done(c.OpenAPI().DeleteCustomResourceInstance(context.Background(), "gpu", "cri-1")); err != nil {
		t.Fatalf("DeleteCustomResourceInstance: unexpected error on empty 204 body: %v", err)
	}

	c, _ = serve(t, http.StatusNotFound, "")
	if err := Done(c.OpenAPI().DeleteCustomResourceInstance(context.Background(), "gpu", "cri-1")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteCustomResourceInstance: got err %v, want ErrNotFound", err)
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
