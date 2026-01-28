package weheat

import (
	_ "embed"

	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/internal/oauth"
	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
	weheatv1 "github.com/joshp123/gohome/proto/gen/plugins/weheat/v1"
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

// NewPlugin constructs a Weheat plugin from config.
func NewPlugin(cfg *weheatv1.WeheatConfig, oauthCfg *configv1.OAuthConfig) (Plugin, bool) {
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
	return "weheat"
}

func (p Plugin) Manifest() core.Manifest {
	return core.Manifest{
		PluginID:    "weheat",
		DisplayName: "Weheat",
		Version:     "0.1.0",
		Services:    []string{"gohome.plugins.weheat.v1.WeheatService"},
	}
}

func (p Plugin) AgentsMD() string {
	return agentsMD
}

func (p Plugin) OAuthDeclaration() oauth.Declaration {
	return oauth.Declaration{
		Provider:       "weheat",
		Flow:           oauth.FlowDevice,
		TokenURL:       "https://auth.weheat.nl/realms/Weheat/protocol/openid-connect/token",
		DeviceAuthURL:  "https://auth.weheat.nl/realms/Weheat/protocol/openid-connect/auth/device",
		DeviceTokenURL: "https://auth.weheat.nl/realms/Weheat/protocol/openid-connect/token",
		Scope:          "openid offline_access",
		StatePath:      "/var/lib/gohome/weheat-credentials.json",
	}
}

func (p Plugin) Dashboards() []core.Dashboard {
	return []core.Dashboard{{Name: "weheat-overview", JSON: dashboardJSON}}
}

func (p Plugin) RegisterGRPC(server *grpc.Server) {
	RegisterWeheatService(server, p.client)
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
