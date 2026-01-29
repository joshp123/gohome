package home

import (
	_ "embed"

	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/internal/oauth"
	homev1 "github.com/joshp123/gohome/proto/gen/plugins/home/v1"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

//go:embed AGENTS.md
var agentsMD string

//go:embed dashboard.json
var dashboardJSON []byte

// Plugin implements the GoHome plugin contract.
type Plugin struct {
	health        core.HealthStatus
	healthMessage string
}

// NewPlugin constructs the My Home dashboard plugin.
func NewPlugin(cfg *homev1.HomeConfig) (Plugin, bool) {
	if cfg == nil {
		return Plugin{}, false
	}
	return Plugin{health: core.HealthHealthy}, true
}

func (p Plugin) ID() string {
	return "home"
}

func (p Plugin) Manifest() core.Manifest {
	return core.Manifest{
		PluginID:    "home",
		DisplayName: "My Home",
		Version:     "0.1.0",
		Services:    nil,
	}
}

func (p Plugin) AgentsMD() string {
	return agentsMD
}

func (p Plugin) OAuthDeclaration() oauth.Declaration {
	return oauth.Declaration{}
}

func (p Plugin) Dashboards() []core.Dashboard {
	return []core.Dashboard{{Name: "my-home-overview", JSON: dashboardJSON}}
}

func (p Plugin) RegisterGRPC(*grpc.Server) {}

func (p Plugin) Collectors() []prometheus.Collector {
	return nil
}

func (p Plugin) Health() core.HealthStatus {
	return p.health
}

func (p Plugin) HealthMessage() string {
	return p.healthMessage
}
