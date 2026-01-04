package core

import (
	"net/http"

	"github.com/joshp123/gohome/internal/oauth"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

// HealthStatus represents plugin health states for registry reporting.
type HealthStatus string

const (
	HealthHealthy  HealthStatus = "HEALTHY"
	HealthDegraded HealthStatus = "DEGRADED"
	HealthError    HealthStatus = "ERROR"
)

// Dashboard is a Grafana dashboard asset embedded by the plugin.
type Dashboard struct {
	Name string
	JSON []byte
}

// Manifest describes a plugin for discovery and registry metadata.
type Manifest struct {
	PluginID    string
	DisplayName string
	Version     string
	Services    []string
}

// Plugin is the compile-time contract for all GoHome plugins.
type Plugin interface {
	ID() string
	Manifest() Manifest
	AgentsMD() string
	OAuthDeclaration() oauth.Declaration
	Dashboards() []Dashboard
	RegisterGRPC(*grpc.Server)
	Collectors() []prometheus.Collector
	Health() HealthStatus
	HealthMessage() string
}

// HTTPRegistrant allows plugins to expose HTTP handlers.
type HTTPRegistrant interface {
	RegisterHTTP(*http.ServeMux)
}
