package httpapi

import (
	"encoding/json"
	"net/http"
)

type Health struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// NewHealthHandler reports contract-conformant readiness for the local host.
func NewHealthHandler(version string) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(Health{Status: "ready", Version: version})
	})
}
