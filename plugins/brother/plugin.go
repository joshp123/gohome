package brother

import (
	_ "embed"

	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/internal/oauth"
	brotherv1 "github.com/joshp123/gohome/proto/gen/plugins/brother/v1"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

//go:embed AGENTS.md
var agentsMD string

// Plugin reads maintenance counters and consumable life from a Brother network printer.
type Plugin struct {
	client        *Client
	health        core.HealthStatus
	healthMessage string
}

// NewPlugin constructs the Brother printer plugin when configured.
func NewPlugin(cfg *brotherv1.BrotherConfig) (Plugin, bool) {
	if cfg == nil {
		return Plugin{}, false
	}
	client, err := NewClient(cfg.GetBaseUrl())
	if err != nil {
		return Plugin{health: core.HealthError, healthMessage: err.Error()}, true
	}
	return Plugin{client: client, health: core.HealthHealthy}, true
}

func (p Plugin) ID() string {
	return "brother"
}

func (p Plugin) Manifest() core.Manifest {
	return core.Manifest{
		PluginID:    "brother",
		DisplayName: "Brother printer",
		Version:     "0.1.0",
		Services:    []string{"gohome.plugins.brother.v1.BrotherService"},
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
		RegisterBrotherService(server, p.client)
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
