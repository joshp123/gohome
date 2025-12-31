package server

import (
	"net/http"
)

// DashboardsHandler serves dashboard JSON from an in-memory map.
func DashboardsHandler(dashboards map[string][]byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if data, ok := dashboards[path]; ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}

		http.NotFound(w, r)
	})
}
