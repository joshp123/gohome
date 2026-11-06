package meaco

import (
	_ "embed"
	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/internal/oauth"
	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
	meacov1 "github.com/joshp123/gohome/proto/gen/plugins/meaco/v1"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"time"
)

//go:embed AGENTS.md
var agentsMD string

//go:embed dashboard.json
var dashboardJSON []byte

type Plugin struct {
	client      *Client
	configError error
}

func NewPlugin(cfg *meacov1.MeacoConfig, _ *configv1.OAuthConfig) (Plugin, bool) {
	if cfg == nil {
		return Plugin{}, false
	}
	runtime, err := ConfigFromProto(cfg)
	if err != nil {
		return Plugin{configError: err}, true
	}
	return Plugin{client: &Client{config: runtime}}, true
}
func (p Plugin) ID() string { return "meaco" }
func (p Plugin) Manifest() core.Manifest {
	return core.Manifest{PluginID: "meaco", DisplayName: "Meaco Arete Two", Version: "0.1.0", Services: []string{"gohome.plugins.meaco.v1.MeacoService"}}
}
func (p Plugin) AgentsMD() string                    { return agentsMD }
func (p Plugin) OAuthDeclaration() oauth.Declaration { return oauth.Declaration{} }
func (p Plugin) Dashboards() []core.Dashboard {
	return []core.Dashboard{{Name: "meaco-overview", JSON: dashboardJSON}}
}
func (p Plugin) RegisterGRPC(server *grpc.Server) {
	meacov1.RegisterMeacoServiceServer(server, &service{client: p.client})
}
func (p Plugin) Collectors() []prometheus.Collector {
	if p.client == nil {
		return nil
	}
	return []prometheus.Collector{NewMetricsCollector(p.client)}
}
func (p Plugin) Health() core.HealthStatus {
	if p.configError != nil || p.client == nil {
		return core.HealthError
	}
	if p.HealthMessage() != "" {
		return core.HealthDegraded
	}
	return core.HealthHealthy
}
func (p Plugin) HealthMessage() string {
	if p.configError != nil {
		return p.configError.Error()
	}
	if p.client == nil {
		return "meaco client not configured"
	}
	p.client.healthMutex.Lock()
	defer p.client.healthMutex.Unlock()
	if p.client.lastError != nil {
		return p.client.lastError.Error()
	}
	if p.client.lastRead.IsZero() {
		return "no successful device read yet"
	}
	if time.Since(p.client.lastRead) > 90*time.Second {
		return "no successful device read in 90 seconds"
	}
	return ""
}
