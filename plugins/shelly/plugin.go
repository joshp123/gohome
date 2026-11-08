package shelly

import (
	_ "embed"

	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/internal/oauth"
	shellyv1 "github.com/joshp123/gohome/proto/gen/plugins/shelly/v1"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

//go:embed AGENTS.md
var agentsMD string

//go:embed dashboard.json
var dashboardJSON []byte

// Plugin implements the GoHome plugin contract for Shelly relays driving roller shutters.
type Plugin struct {
	client        *Client
	health        core.HealthStatus
	healthMessage string
}

// NewPlugin constructs the plugin; the plugin is enabled when its config is present.
func NewPlugin(cfg *shellyv1.ShellyConfig) (Plugin, bool) {
	if cfg == nil {
		return Plugin{}, false
	}
	covers, err := coversFromProto(cfg)
	if err != nil {
		return Plugin{health: core.HealthError, healthMessage: err.Error()}, true
	}
	return Plugin{client: NewClient(covers), health: core.HealthHealthy}, true
}

func (p Plugin) ID() string {
	return "shelly"
}

func (p Plugin) Manifest() core.Manifest {
	return core.Manifest{
		PluginID:    "shelly",
		DisplayName: "Shelly shutters",
		Version:     "0.1.0",
		Services:    []string{"gohome.plugins.shelly.v1.ShellyService"},
	}
}

func (p Plugin) AgentsMD() string {
	return agentsMD
}

func (p Plugin) OAuthDeclaration() oauth.Declaration {
	return oauth.Declaration{}
}

func (p Plugin) Dashboards() []core.Dashboard {
	return []core.Dashboard{{Name: "overview", Folder: "Devices", JSON: dashboardJSON}}
}

func (p Plugin) RegisterGRPC(server *grpc.Server) {
	RegisterShellyService(server, p.client)
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
