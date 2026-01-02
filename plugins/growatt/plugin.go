package growatt

import (
	"context"
	_ "embed"
	"log"
	"time"

	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/internal/oauth"
	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
	growattv1 "github.com/joshp123/gohome/proto/gen/plugins/growatt/v1"
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

// NewPlugin constructs a Growatt plugin from config.
func NewPlugin(cfg *growattv1.GrowattConfig, _ *configv1.OAuthConfig) (Plugin, bool) {
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

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		plant, err := client.ResolvePlant(ctx, 0)
		if err != nil {
			log.Printf("growatt history bootstrap: resolve plant failed: %v", err)
			return
		}
		if err := client.ImportEnergyHistory(ctx, plant); err != nil {
			log.Printf("growatt history bootstrap failed: %v", err)
		}
	}()

	return Plugin{client: client, health: core.HealthHealthy}, true
}

func (p Plugin) ID() string {
	return "growatt"
}

func (p Plugin) Manifest() core.Manifest {
	return core.Manifest{
		PluginID:    "growatt",
		DisplayName: "Growatt",
		Version:     "0.1.0",
		Services:    []string{"gohome.plugins.growatt.v1.GrowattService"},
	}
}

func (p Plugin) AgentsMD() string {
	return agentsMD
}

func (p Plugin) OAuthDeclaration() oauth.Declaration {
	return oauth.Declaration{Provider: "growatt"}
}

func (p Plugin) Dashboards() []core.Dashboard {
	return []core.Dashboard{{Name: "growatt-overview", JSON: dashboardJSON}}
}

func (p Plugin) RegisterGRPC(server *grpc.Server) {
	RegisterGrowattService(server, p.client)
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
