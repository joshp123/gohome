package tado

import (
	_ "embed"

	"github.com/elliot-alderson/gohome/internal/core"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

//go:embed AGENTS.md
var agentsMD string

//go:embed dashboard.json
var dashboardJSON []byte

// Plugin implements the GoHome plugin contract.
type Plugin struct{}

func (p Plugin) ID() string {
	return "tado"
}

func (p Plugin) Manifest() core.Manifest {
	return core.Manifest{
		PluginID:    "tado",
		DisplayName: "Tado",
		Version:     "0.1.0",
		Services:    []string{"gohome.plugins.tado.v1.TadoService"},
	}
}

func (p Plugin) AgentsMD() string {
	return agentsMD
}

func (p Plugin) Dashboards() []core.Dashboard {
	return []core.Dashboard{{Name: "tado-overview", JSON: dashboardJSON}}
}

func (p Plugin) RegisterGRPC(server *grpc.Server) {
	RegisterTadoService(server)
}

func (p Plugin) Collectors() []prometheus.Collector {
	return nil
}

func (p Plugin) Health() core.HealthStatus {
	return core.HealthHealthy
}

func (p Plugin) HealthMessage() string {
	return ""
}
