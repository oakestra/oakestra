package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

var testDigest = "sha256:" + strings.Repeat("ab", 32)

func fakeRegistry(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/v2/" {
			return
		}
		w.Header().Set("Docker-Content-Digest", testDigest)
		w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		w.Header().Set("Content-Length", "2")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestResolveRefusesLoopbackRegistryByDefault(t *testing.T) {
	srv, hits := fakeRegistry(t)
	image := strings.TrimPrefix(srv.URL, "http://") + "/team/app:1.0"

	_, err := Remote{}.Resolve(context.Background(), image)
	if err == nil {
		t.Fatal("a registry on 127.0.0.1 must be refused by default")
	}
	if hits.Load() != 0 {
		t.Errorf("registry was contacted %d times", hits.Load())
	}
}

func TestResolveAllowPrivate(t *testing.T) {
	srv, _ := fakeRegistry(t)
	image := strings.TrimPrefix(srv.URL, "http://") + "/team/app:1.0"

	got, err := Remote{AllowPrivate: true}.Resolve(context.Background(), image)
	if err != nil {
		t.Fatal(err)
	}
	if got != testDigest {
		t.Errorf("digest = %q", got)
	}
}
