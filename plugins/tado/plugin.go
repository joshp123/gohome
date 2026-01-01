package tado

import (
	_ "embed"

	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/internal/oauth"
	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
	tadov1 "github.com/joshp123/gohome/proto/gen/plugins/tado/v1"
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

// NewPlugin constructs a Tado plugin from config.
func NewPlugin(cfg *tadov1.TadoConfig, oauthCfg *configv1.OAuthConfig) (Plugin, bool) {
	if cfg == nil {
		return Plugin{}, false
	}

	runtimeCfg, err := ConfigFromProto(cfg)
	if err != nil {
		return Plugin{health: core.HealthError, healthMessage: err.Error()}, true
	}

	decl := Plugin{}.OAuthDeclaration()
	client, err := NewClient(runtimeCfg, decl, oauthCfg)
	if err != nil {
		return Plugin{health: core.HealthError, healthMessage: err.Error()}, true
	}

	return Plugin{client: client, health: core.HealthHealthy}, true
}

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

func (p Plugin) OAuthDeclaration() oauth.Declaration {
	return oauth.Declaration{
		Provider:       "tado",
		Flow:           oauth.FlowDevice,
		TokenURL:       "https://login.tado.com/oauth2/token",
		DeviceAuthURL:  "https://login.tado.com/oauth2/device_authorize",
		DeviceTokenURL: "https://login.tado.com/oauth2/token",
		Scope:          "offline_access",
		StatePath:      "/var/lib/gohome/tado-token.json",
	}
}

func (p Plugin) Dashboards() []core.Dashboard {
	return []core.Dashboard{{Name: "tado-overview", JSON: dashboardJSON}}
}

func (p Plugin) RegisterGRPC(server *grpc.Server) {
	RegisterTadoService(server, p.client)
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
