package manager

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// withTestServer temporarily redirects Deploy calls to srv for the duration of fn.
func withTestServer(t *testing.T, srv *httptest.Server, fn func()) {
	t.Helper()
	u, _ := url.Parse(srv.URL)
	origURL, origPort := managerURL, managerPort
	managerURL = u.Hostname()
	managerPort = u.Port()
	defer func() { managerURL = origURL; managerPort = origPort }()
	fn()
}

func TestDeploy_Success(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	withTestServer(t, srv, func() {
		if err := Deploy("job-1", "candidate-1", true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var req deploymentRequest
	if err := json.Unmarshal(gotBody, &req); err != nil {
		t.Fatalf("could not parse body: %v", err)
	}
	if req.JobID != "job-1" || req.CandidateID != "candidate-1" {
		t.Errorf("unexpected payload: %+v", req)
	}
}

func TestDeploy_Failure(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	withTestServer(t, srv, func() {
		if err := Deploy("job-1", "NO_WORKER_CAPACITY", false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var req deploymentFailedRequest
	if err := json.Unmarshal(gotBody, &req); err != nil {
		t.Fatalf("could not parse body: %v", err)
	}
	if req.JobID != "job-1" || req.Status != "NO_WORKER_CAPACITY" {
		t.Errorf("unexpected payload: %+v", req)
	}
}

func TestDeploy_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	u, _ := url.Parse(srv.URL)
	srv.Close() // close immediately so connection is refused

	origURL, origPort := managerURL, managerPort
	managerURL = u.Hostname()
	managerPort = u.Port()
	defer func() { managerURL = origURL; managerPort = origPort }()

	if err := Deploy("job-1", "candidate-1", true); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDeploy_CorrectPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	withTestServer(t, srv, func() {
		_ = Deploy("job-1", "candidate-1", true)
	})

	if gotPath != deployPath {
		t.Errorf("path = %q, want %q", gotPath, deployPath)
	}
}
