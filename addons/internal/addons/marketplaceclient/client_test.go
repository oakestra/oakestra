package marketplaceclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oakestra/oakestra/addons/internal/definition"
)

const goodBody = `{
  "name": "tool", "source": {"kind": "tap", "name": "acme"}, "version": "1.2.3",
  "id": "acme--tool", "qualified_name": "acme/tool",
  "required_permissions": {"docker_socket": true}, "definition_hash": "sha256:lies",
  "permissions": {"host_paths": [], "host_ports": [], "docker_socket": false, "host_network": false, "core_network": false},
  "services": [{"service_name": "web", "image": "x@sha256:aa"}]
}`

func TestGet(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(goodBody))
	}))
	defer srv.Close()

	a, err := New(srv.URL+"/").Get(context.Background(), definition.TapSource("acme"), "tool")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/marketplace/addons/acme/tool" {
		t.Errorf("path = %s", gotPath)
	}
	if a.Version != "1.2.3" || a.ID() != "acme--tool" || len(a.Volumes) != 0 || a.Volumes == nil {
		t.Errorf("addon = %+v", a)
	}
	// The precomputed permission fields can't reach callers: the type has no place for them.
	if a.Permissions.DockerSocket {
		t.Error("marketplace-provided permissions leaked in")
	}
}

func TestGetOfficialUsesOfficialSegment(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		http.NotFound(w, r)
	}))
	defer srv.Close()
	_, err := New(srv.URL).Get(context.Background(), definition.OfficialSource, "monitoring")
	if !errors.Is(err, ErrNotFound) || gotPath != "/api/v1/marketplace/addons/official/monitoring" {
		t.Errorf("err = %v path = %s", err, gotPath)
	}
}

func TestGetErrors(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"500":         func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) },
		"broken json": func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("{")) },
		"other addon": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(strings.Replace(goodBody, `"name": "tool"`, `"name": "other"`, 1)))
		},
		"other source": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(strings.Replace(goodBody, `"acme"`, `"evil"`, 1)))
		},
		"oversize": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"name":"tool","description":"` + strings.Repeat("a", maxBody) + `"}`))
		},
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			_, err := New(srv.URL).Get(context.Background(), definition.TapSource("acme"), "tool")
			if !errors.Is(err, ErrUnavailable) {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestGetUnreachableAndTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	c := New(srv.URL)
	c.http.Timeout = 50 * time.Millisecond
	if _, err := c.Get(context.Background(), definition.OfficialSource, "x"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("timeout: %v", err)
	}
	srv.CloseClientConnections()
	srv.Close()
	if _, err := New(srv.URL).Get(context.Background(), definition.OfficialSource, "x"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("closed: %v", err)
	}
}
