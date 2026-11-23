package unifi

import (
	_ "embed"

	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/internal/oauth"
	unifiv1 "github.com/joshp123/gohome/proto/gen/plugins/unifi/v1"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

//go:embed AGENTS.md
var agentsMD string

// Plugin reads network health, device metrics, and client counts from a local UniFi controller.
type Plugin struct {
	client        *Client
	health        core.HealthStatus
	healthMessage string
}

// NewPlugin constructs the UniFi plugin when configured.
func NewPlugin(cfg *unifiv1.UniFiConfig) (Plugin, bool) {
	if cfg == nil {
		return Plugin{}, false
	}
	client, err := NewClient(cfg.GetBaseUrl(), cfg.GetApiKeyFile(), cfg.GetSite(), cfg.GetInsecureSkipVerify())
	if err != nil {
		return Plugin{health: core.HealthError, healthMessage: err.Error()}, true
	}
	return Plugin{client: client, health: core.HealthHealthy}, true
}

func (p Plugin) ID() string {
	return "unifi"
}

func (p Plugin) Manifest() core.Manifest {
	return core.Manifest{
		PluginID:    "unifi",
		DisplayName: "UniFi network",
		Version:     "0.1.0",
		Services:    []string{"gohome.plugins.unifi.v1.UniFiService"},
	}
}

func (p Plugin) AgentsMD() string {
	return agentsMD
}

func (p Plugin) OAuthDeclaration() oauth.Declaration {
	return oauth.Declaration{}
}

func (p Plugin) RegisterGRPC(server *grpc.Server) {
	if p.client != nil {
		RegisterUniFiService(server, p.client)
	}
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
