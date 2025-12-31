package server

import (
	"net/http"
)

// HTTPServer serves health, metrics, and dashboards.
type HTTPServer struct {
	Server *http.Server
}

func NewHTTPServer(addr string, handler http.Handler) *HTTPServer {
	return &HTTPServer{Server: &http.Server{Addr: addr, Handler: handler}}
}

func (s *HTTPServer) ListenAndServe() error {
	return s.Server.ListenAndServe()
}
