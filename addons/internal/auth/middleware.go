package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// RequireUser rejects requests without a valid access token with 401 and stores the claims in
// the request context for next.
func (v *Verifier) RequireUser(next http.Handler) http.Handler {
	return v.require(next, nil)
}

// RequireAdmin is RequireUser plus a 403 for tokens without AdminRole.
func (v *Verifier) RequireAdmin(next http.Handler) http.Handler {
	return v.require(next, func(c *Claims) bool { return c.IsAdmin() })
}

func (v *Verifier) require(next http.Handler, allowed func(*Claims) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := bearerToken(r)
		if !ok {
			unauthorized(w, "Missing authentication token")
			return
		}
		claims, err := v.Verify(raw)
		switch {
		case errors.Is(err, ErrNotAccess):
			unauthorized(w, "Only access token allowed")
			return
		case err != nil:
			// The parser's error text can echo token details, so keep the response generic.
			unauthorized(w, "Invalid authentication token")
			return
		}
		if allowed != nil && !allowed(claims) {
			// Same message as system_manager's not_authorized.
			writeError(w, http.StatusForbidden, "You have not enough permissions!")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), claims)))
	})
}

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
	writeError(w, http.StatusUnauthorized, msg)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": msg})
}
