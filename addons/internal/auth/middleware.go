package auth

import (
	"net/http"
	"strings"

	"github.com/oakestra/oakestra/addons/internal/httpx"
)

func bearerToken(r *http.Request) (string, bool) {
	scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

func unauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("WWW-Authenticate", `Bearer`)
	httpx.WriteError(w, http.StatusUnauthorized, httpx.CodeUnauthorized, msg)
}
