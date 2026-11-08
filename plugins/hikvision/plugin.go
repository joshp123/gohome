package hikvision

import (
	"context"
	_ "embed"

	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/internal/oauth"
	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
	hikvisionv1 "github.com/joshp123/gohome/proto/gen/plugins/hikvision/v1"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

//go:embed AGENTS.md
var agentsMD string

//go:embed dashboard.json
var dashboardJSON []byte

// Plugin implements the GoHome plugin contract for one Hikvision camera.
type Plugin struct {
	client        *Client
	stream        *alertStream
	metrics       *MetricsCollector
	health        core.HealthStatus
	healthMessage string
}

// NewPlugin builds the plugin and starts reading the camera's event stream.
func NewPlugin(cfg *hikvisionv1.HikvisionConfig, _ *configv1.OAuthConfig) (Plugin, bool) {
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
	plugin := newPlugin(client)
	go plugin.stream.Run(context.Background())
	return plugin, true
}

func newPlugin(client *Client) Plugin {
	metrics := NewMetricsCollector(client)
	stream := newAlertStream(client, metrics.observeEvent)
	metrics.stream = stream
	return Plugin{client: client, stream: stream, metrics: metrics, health: core.HealthHealthy}
}

func (p Plugin) ID() string {
	return "hikvision"
}

func (p Plugin) Manifest() core.Manifest {
	return core.Manifest{
		PluginID:    "hikvision",
		DisplayName: "Hikvision camera",
		Version:     "0.1.0",
		Services:    []string{"gohome.plugins.hikvision.v1.HikvisionService"},
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
	RegisterHikvisionService(server, p.client, p.stream)
}

func (p Plugin) Collectors() []prometheus.Collector {
	if p.metrics == nil {
		return nil
	}
	return []prometheus.Collector{p.metrics}
}

func (p Plugin) Health() core.HealthStatus {
	return p.health
}

func (p Plugin) HealthMessage() string {
	return p.healthMessage
}
