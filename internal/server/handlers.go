package server

import (
	"net/http"
)

// HealthHandler returns a simple OK for liveness checks.
func HealthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
