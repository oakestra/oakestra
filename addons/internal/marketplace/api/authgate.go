package api

import (
	"encoding/json"
	"net/http"

	"github.com/oakestra/oakestra/addons/internal/auth"
)

// DefaultKeyRefreshInterval limits how often a failed verification may re-fetch the key.
const DefaultKeyRefreshInterval = auth.DefaultKeyRefreshInterval

// AdminGate is the readiness check and key refresh around the admin endpoints. The logic lives
// in auth so the addons service shares it.
type AdminGate = auth.Gate

// NewAdminGate returns a gate without a key; see auth.NewGate.
var NewAdminGate = auth.NewGate

type errorBody struct {
	Message string `json:"message"`
	Errors  any    `json:"errors,omitempty"`
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Message: msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
