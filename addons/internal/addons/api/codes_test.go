package api

import (
	"strings"
	"testing"

	"github.com/oakestra/oakestra/addons/internal/addons"
	"github.com/oakestra/oakestra/addons/internal/addons/marketplaceclient"
	"github.com/oakestra/oakestra/addons/internal/httpx"
)

func errorCode(t *testing.T, body []byte) string {
	t.Helper()
	return decode[struct {
		Code string `json:"code"`
	}](t, body).Code
}

func TestErrorCodes(t *testing.T) {
	f := newFixture(t, "root")
	f.mkt.set(def("insights", "1.0.0"))
	f.mkt.set(def("other", "1.0.0"))

	// Install needs the confirmation first, then succeeds, then is a different 409.
	code, body := f.do("POST", "/api/v1/addons", f.admin, map[string]string{"name": "insights"})
	if code != 409 || errorCode(t, body) != httpx.CodeConfirmationRequired {
		t.Fatalf("unconfirmed install: %d %s", code, body)
	}
	a := f.install("insights")
	code, body = f.do("POST", "/api/v1/addons", f.admin, map[string]string{"name": "insights"})
	if code != 409 || errorCode(t, body) != httpx.CodeAlreadyInstalled {
		t.Errorf("second install: %d %s", code, body)
	}

	f.mkt.set(withHostPath("hostpath", "1.0.0", "/srv/shared"))
	f.mkt.set(withHostPath("clash", "1.0.0", "/srv/shared/app"))
	f.install("hostpath")
	code, body = f.do("POST", "/api/v1/addons", f.admin, map[string]string{"name": "clash"})
	if code != 409 || errorCode(t, body) != httpx.CodeHostPathConflict {
		t.Errorf("host path conflict: %d %s", code, body)
	}

	f.setStatus(a.ID, addons.Active)
	if code, body := f.do("POST", "/api/v1/addons/"+a.ID+"/update", f.admin, f.accept("insights")); code != 409 || errorCode(t, body) != httpx.CodeNoUpdateAvailable {
		t.Errorf("nothing newer: %d %s", code, body)
	}
	f.mkt.set(def("insights", "1.1.0"))
	if code, body := f.do("POST", "/api/v1/addons/"+a.ID+"/update", f.admin, "{}"); code != 409 || errorCode(t, body) != httpx.CodeConfirmationRequired {
		t.Errorf("update without hash: %d %s", code, body)
	}
	if code, body := f.do("POST", "/api/v1/addons/"+a.ID+"/approve", f.admin, "{}"); code != 409 || errorCode(t, body) != httpx.CodeConflict {
		t.Errorf("approve of a superseded version: %d %s", code, body)
	}
	f.setStatus(a.ID, addons.Updating)
	if code, body := f.do("POST", "/api/v1/addons/"+a.ID+"/update", f.admin, "{}"); code != 409 || errorCode(t, body) != httpx.CodeBusy {
		t.Errorf("busy: %d %s", code, body)
	}
}

func TestErrorCodesForAuthAndInput(t *testing.T) {
	f := newFixture(t, "root")
	for _, tc := range []struct {
		name, method, path, tok string
		body                    any
		status                  int
		code                    string
	}{
		{"no token", "POST", "/api/v1/addons", "", "{}", 401, httpx.CodeUnauthorized},
		{"wrong role", "POST", "/api/v1/addons", f.user, "{}", 403, httpx.CodeForbidden},
		{"unknown addon", "POST", "/api/v1/addons/nope/update", f.admin, "{}", 404, httpx.CodeNotFound},
		{"unknown route", "GET", "/nope", "", nil, 404, httpx.CodeNotFound},
		{"wrong method", "PATCH", "/api/v1/addons", f.admin, "{}", 405, httpx.CodeMethodNotAllowed},
		{"invalid name", "POST", "/api/v1/addons", f.admin, map[string]string{"name": "Bad Name"}, 400, httpx.CodeValidationFailed},
		{"not an object", "POST", "/api/v1/addons", f.admin, "[]", 400, httpx.CodeValidationFailed},
		{"unlisted addon", "POST", "/api/v1/addons", f.admin, map[string]string{"name": "nothing"}, 404, httpx.CodeNotFound},
		{"oversize body", "POST", "/api/v1/addons", f.admin, `{"name":"` + strings.Repeat("a", MaxBodyBytes) + `"}`, 413, httpx.CodePayloadTooLarge},
	} {
		code, body := f.do(tc.method, tc.path, tc.tok, tc.body)
		if code != tc.status || errorCode(t, body) != tc.code {
			t.Errorf("%s: %d %s, want %d %s", tc.name, code, body, tc.status, tc.code)
		}
	}

	unpinned := def("loose", "1.0.0")
	unpinned.Services[0].Image = "ghcr.io/x/web:latest"
	f.mkt.set(unpinned)
	code, body := f.do("POST", "/api/v1/addons", f.admin, map[string]string{"name": "loose"})
	if code != 422 || errorCode(t, body) != httpx.CodeValidationFailed {
		t.Errorf("invalid definition: %d %s", code, body)
	}

	f.mkt.err = marketplaceclient.ErrUnavailable
	code, body = f.do("POST", "/api/v1/addons", f.admin, map[string]string{"name": "loose"})
	if code != 502 || errorCode(t, body) != httpx.CodeMarketplaceDown {
		t.Errorf("marketplace down: %d %s", code, body)
	}
}
