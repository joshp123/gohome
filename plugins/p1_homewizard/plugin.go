package p1_homewizard

import (
	_ "embed"

	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/internal/oauth"
	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
	p1v1 "github.com/joshp123/gohome/proto/gen/plugins/p1_homewizard/v1"
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
	tariffs       Tariffs
	health        core.HealthStatus
	healthMessage string
}

// NewPlugin constructs a P1 Homewizard plugin from config.
func NewPlugin(cfg *p1v1.P1HomewizardConfig, _ *configv1.OAuthConfig) (Plugin, bool) {
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

	return Plugin{client: client, tariffs: runtimeCfg.Tariffs, health: core.HealthHealthy}, true
}

func (p Plugin) ID() string {
	return "p1_homewizard"
}

func (p Plugin) Manifest() core.Manifest {
	return core.Manifest{
		PluginID:    "p1_homewizard",
		DisplayName: "P1 Homewizard",
		Version:     "0.1.0",
		Services:    []string{"gohome.plugins.p1_homewizard.v1.P1HomewizardService"},
	}
}

func (p Plugin) AgentsMD() string {
	return agentsMD
}

func (p Plugin) OAuthDeclaration() oauth.Declaration {
	return oauth.Declaration{}
}

func (p Plugin) Dashboards() []core.Dashboard {
	return []core.Dashboard{{Name: "p1-homewizard-overview", JSON: dashboardJSON}}
}

func (p Plugin) RegisterGRPC(server *grpc.Server) {
	RegisterP1HomewizardService(server, p.client)
}

func (p Plugin) Collectors() []prometheus.Collector {
	if p.client == nil {
		return nil
	}
	return []prometheus.Collector{NewMetricsCollector(p.client, p.tariffs)}
}

func (p Plugin) Health() core.HealthStatus {
	return p.health
}

func (p Plugin) HealthMessage() string {
	return p.healthMessage
}
