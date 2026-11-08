package casambi

import (
	_ "embed"

	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/internal/oauth"
	casambiv1 "github.com/joshp123/gohome/proto/gen/plugins/casambi/v1"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

//go:embed AGENTS.md
var agentsMD string

//go:embed dashboard.json
var dashboardJSON []byte

// Plugin reads Casambi lights through the light bridge's read-only API.
type Plugin struct {
	bridge        casambiv1.CasambiServiceClient
	health        core.HealthStatus
	healthMessage string
}

func NewPlugin(cfg *casambiv1.CasambiConfig) (Plugin, bool) {
	if cfg == nil {
		return Plugin{}, false
	}
	bridge, err := NewBridgeClient(cfg.GetBridgeAddress())
	if err != nil {
		return Plugin{health: core.HealthError, healthMessage: err.Error()}, true
	}
	return Plugin{bridge: bridge, health: core.HealthHealthy}, true
}

func (p Plugin) ID() string {
	return "casambi"
}

func (p Plugin) Manifest() core.Manifest {
	return core.Manifest{
		PluginID:    "casambi",
		DisplayName: "Casambi lights",
		Version:     "0.1.0",
		Services:    []string{"gohome.plugins.casambi.v1.CasambiService"},
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
	if p.bridge != nil {
		casambiv1.RegisterCasambiServiceServer(server, forwardingService{bridge: p.bridge})
	}
}

func (p Plugin) Collectors() []prometheus.Collector {
	if p.bridge == nil {
		return nil
	}
	return []prometheus.Collector{NewMetricsCollector(p.bridge)}
}

func (p Plugin) Health() core.HealthStatus {
	return p.health
}

func (p Plugin) HealthMessage() string {
	return p.healthMessage
}
