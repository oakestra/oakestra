package resource

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"scheduler/calculate/schedulers/placement"
	"testing"
)

// testNode is a minimal placement.Candidate used by AvailableResources tests.
type testNode struct {
	NodeID string  `json:"_id"`
	Memory float64 `json:"memory"`
}

func (n testNode) ID() string { return n.NodeID }

var _ placement.Candidate = testNode{}

// --- formatRequestParameters ---

func TestFormatRequestParameters_EmptyValuesOnly(t *testing.T) {
	got := formatRequestParameters(map[string]string{"candidate_name": ""})
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestFormatRequestParameters_SingleEntry(t *testing.T) {
	got := formatRequestParameters(map[string]string{"candidate_name": "cluster-a"})
	if got != "candidate_name=cluster-a" {
		t.Fatalf("expected %q, got %q", "candidate_name=cluster-a", got)
	}
}

func TestFormatRequestParameters_EmptyMap(t *testing.T) {
	if got := formatRequestParameters(nil); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

// --- formatInterestedResources ---

func TestFormatInterestedResources_Empty(t *testing.T) {
	if got := formatInterestedResources(nil); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestFormatInterestedResources_Single(t *testing.T) {
	got := formatInterestedResources([]string{"_id"})
	if got != "_id" {
		t.Fatalf("expected _id, got %q", got)
	}
}

func TestFormatInterestedResources_Multiple(t *testing.T) {
	got := formatInterestedResources([]string{"_id", "memory", "vcpus"})
	if got != "_id,memory,vcpus" {
		t.Fatalf("expected _id,memory,vcpus, got %q", got)
	}
}

// --- formatQuery ---

func TestFormatQuery_OnlyActiveWhenNoFilters(t *testing.T) {
	got := formatQuery(map[string]string{"candidate_name": ""}, nil)
	if got != "?active=true" {
		t.Fatalf("expected %q, got %q", "?active=true", got)
	}
}

func TestFormatQuery_WithRequestAndResources(t *testing.T) {
	got := formatQuery(
		map[string]string{"candidate_name": "cluster-a"},
		[]string{"_id", "memory"},
	)
	if got != "?active=true&candidate_name=cluster-a&_id,memory" {
		t.Fatalf("expected %q, got %q", "?active=true&candidate_name=cluster-a&_id,memory", got)
	}
}

// --- AvailableResources ---

func TestAvailableResources_Success(t *testing.T) {
	want := []testNode{{NodeID: "n1", Memory: 1000}, {NodeID: "n2", Memory: 2000}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(want); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()
	withTestServer(t, srv, func() {
		var got []testNode
		if err := AvailableResources(&got, nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(want) {
			t.Fatalf("got %d nodes, want %d", len(got), len(want))
		}
		if got[0].NodeID != "n1" || got[1].NodeID != "n2" {
			t.Errorf("unexpected nodes: %v", got)
		}
	})
}

func TestAvailableResources_HTTPError(t *testing.T) {
	// Create a server then close it immediately so the port refuses connections.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	u, _ := url.Parse(srv.URL)
	srv.Close()
	origURL, origPort, origBase := resourceAbstractorURL, resourceAbstractorPort, resourceBaseURL
	resourceAbstractorURL = u.Hostname()
	resourceAbstractorPort = u.Port()
	resourceBaseURL = fmt.Sprintf("%s://%s:%s%s/", protocol, resourceAbstractorURL, resourceAbstractorPort, resourcesPath)
	defer func() { resourceAbstractorURL = origURL; resourceAbstractorPort = origPort; resourceBaseURL = origBase }()

	var got []testNode
	if err := AvailableResources(&got, nil, nil); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAvailableResources_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()
	withTestServer(t, srv, func() {
		var got []testNode
		if err := AvailableResources(&got, nil, nil); err == nil {
			t.Fatal("expected unmarshal error, got nil")
		}
	})
}

// withTestServer temporarily points the resource-abstractor client at srv
// for the duration of fn.
func withTestServer(t *testing.T, srv *httptest.Server, fn func()) {
	t.Helper()
	u, _ := url.Parse(srv.URL)
	origURL, origPort, origBase := resourceAbstractorURL, resourceAbstractorPort, resourceBaseURL
	resourceAbstractorURL = u.Hostname()
	resourceAbstractorPort = u.Port()
	resourceBaseURL = fmt.Sprintf("%s://%s:%s%s/", protocol, resourceAbstractorURL, resourceAbstractorPort, resourcesPath)
	defer func() { resourceAbstractorURL = origURL; resourceAbstractorPort = origPort; resourceBaseURL = origBase }()
	fn()
}
