package airgradient

import (
	_ "embed"

	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/internal/oauth"
	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
	airgradientv1 "github.com/joshp123/gohome/proto/gen/plugins/airgradient/v1"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

//go:embed AGENTS.md
var agentsMD string

//go:embed dashboard.json
var dashboardJSON []byte

// Plugin implements the GoHome plugin contract.
type Plugin struct {
	client        *Client
	health        core.HealthStatus
	healthMessage string
}

// NewPlugin constructs an AirGradient plugin from config.
func NewPlugin(cfg *airgradientv1.AirgradientConfig, _ *configv1.OAuthConfig) (Plugin, bool) {
	if cfg == nil {
		return Plugin{}, false
	}

	runtimeCfg, err := ConfigFromProto(cfg)
	if err != nil {
		return Plugin{health: core.HealthError, healthMessage: err.Error()}, true
	}

	client, err := NewClient(runtimeCfg)
	if err != nil {
		return Plugin{health: core.HealthError, healthMessage: err.Error()}, true
	}

	return Plugin{client: client, health: core.HealthHealthy}, true
}

func (p Plugin) ID() string {
	return "airgradient"
}

func (p Plugin) Manifest() core.Manifest {
	return core.Manifest{
		PluginID:    "airgradient",
		DisplayName: "AirGradient",
		Version:     "0.1.0",
		Services:    []string{"gohome.plugins.airgradient.v1.AirGradientService"},
	}
}

func (p Plugin) AgentsMD() string {
	return agentsMD
}

func (p Plugin) OAuthDeclaration() oauth.Declaration {
	return oauth.Declaration{}
}

func (p Plugin) Dashboards() []core.Dashboard {
	return []core.Dashboard{{Name: "airgradient-overview", JSON: dashboardJSON}}
}

func (p Plugin) RegisterGRPC(server *grpc.Server) {
	RegisterAirGradientService(server, p.client)
}

func (p Plugin) Collectors() []prometheus.Collector {
	if p.client == nil {
		return nil
	}
	return []prometheus.Collector{NewMetricsCollector(p.client)}
}

func (p Plugin) Health() core.HealthStatus {
	return p.health
}

func (p Plugin) HealthMessage() string {
	return p.healthMessage
}
