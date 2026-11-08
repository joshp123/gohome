package sonos

import (
	_ "embed"

	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/internal/oauth"
	sonosv1 "github.com/joshp123/gohome/proto/gen/plugins/sonos/v1"
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

// NewPlugin constructs a read-only Sonos plugin from config.
func NewPlugin(cfg *sonosv1.SonosConfig) (Plugin, bool) {
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

func (p Plugin) ID() string { return "sonos" }

func (p Plugin) Manifest() core.Manifest {
	return core.Manifest{
		PluginID:    "sonos",
		DisplayName: "Sonos",
		Version:     "0.1.0",
		Services:    []string{"gohome.plugins.sonos.v1.SonosService"},
	}
}

func (p Plugin) AgentsMD() string { return agentsMD }

func (p Plugin) OAuthDeclaration() oauth.Declaration { return oauth.Declaration{} }

func (p Plugin) Dashboards() []core.Dashboard {
	return []core.Dashboard{{Name: "overview", Folder: "Devices", JSON: dashboardJSON}}
}

func (p Plugin) RegisterGRPC(server *grpc.Server) {
	sonosv1.RegisterSonosServiceServer(server, &service{client: p.client})
}

func (p Plugin) Collectors() []prometheus.Collector {
	if p.client == nil {
		return nil
	}
	return []prometheus.Collector{NewMetricsCollector(p.client)}
}

func (p Plugin) Health() core.HealthStatus { return p.health }

func (p Plugin) HealthMessage() string { return p.healthMessage }
