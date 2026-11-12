package weheat

import (
	_ "embed"
	"time"

	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/internal/oauth"
	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
	weheatv1 "github.com/joshp123/gohome/proto/gen/plugins/weheat/v1"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

// Weheat publishes these OAuth client credentials for public use.
// They are not secret — Weheat intentionally distributes them for third-party integrations.
// Source: https://support.weheat.nl/s/article/Is-er-een-offici%C3%ABle-Home-Assistant-integratie
const (
	weheatClientID     = "HomeAssistantAPI"
	weheatClientSecret = "TqpNpiJDKbGXF8jaL9D1Y8yzl1pI1Fly"
)

//go:embed AGENTS.md
var agentsMD string

// Plugin implements the GoHome plugin contract.
type Plugin struct {
	client        *Client
	collector     *MetricsCollector
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

	bootstrap := oauth.Bootstrap{
		SchemaVersion: 1,
		ClientID:      weheatClientID,
		ClientSecret:  weheatClientSecret,
		Scope:         "openid offline_access",
	}

	decl := Plugin{}.OAuthDeclaration()
	client, err := NewClient(runtimeCfg, bootstrap, decl, oauthCfg)
	if err != nil {
		return Plugin{health: core.HealthError, healthMessage: err.Error()}, true
	}

	return Plugin{client: client, collector: NewMetricsCollector(client)}, true
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
		Provider:     "weheat",
		Flow:         oauth.FlowAuthCode,
		AuthorizeURL: "https://auth.weheat.nl/auth/realms/Weheat/protocol/openid-connect/auth/",
		TokenURL:     "https://auth.weheat.nl/auth/realms/Weheat/protocol/openid-connect/token/",
		Scope:        "openid offline_access",
		StatePath:    "/var/lib/gohome/weheat-credentials.json",
	}
}

func (p Plugin) RegisterGRPC(server *grpc.Server) {
	RegisterWeheatService(server, p.client)
}

func (p Plugin) Collectors() []prometheus.Collector {
	if p.client == nil {
		return nil
	}
	return []prometheus.Collector{p.collector}
}

func (p Plugin) Health() core.HealthStatus {
	if p.collector == nil {
		return p.health
	}
	p.collector.collectMu.Lock()
	defer p.collector.collectMu.Unlock()
	if p.collector.readError != "" || p.collector.lastRead.IsZero() || time.Since(p.collector.lastRead) > 5*time.Minute {
		return core.HealthError
	}
	return core.HealthHealthy
}

func (p Plugin) HealthMessage() string {
	if p.collector == nil {
		return p.healthMessage
	}
	p.collector.collectMu.Lock()
	defer p.collector.collectMu.Unlock()
	if p.collector.readError != "" {
		return p.collector.readError
	}
	if p.collector.lastRead.IsZero() || time.Since(p.collector.lastRead) > 5*time.Minute {
		return "no recent successful heat pump read"
	}
	return ""
}
