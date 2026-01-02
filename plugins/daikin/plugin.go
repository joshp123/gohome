package daikin

import (
	_ "embed"
	"time"

	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/internal/oauth"
	"github.com/joshp123/gohome/internal/rate"
	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
	daikinv1 "github.com/joshp123/gohome/proto/gen/plugins/daikin/v1"
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

var _ rate.RateLimited = (*Plugin)(nil)

// NewPlugin constructs a Daikin plugin from config.
func NewPlugin(cfg *daikinv1.DaikinConfig, oauthCfg *configv1.OAuthConfig) (Plugin, bool) {
	if cfg == nil {
		return Plugin{}, false
	}

	runtimeCfg, err := ConfigFromProto(cfg)
	if err != nil {
		return Plugin{health: core.HealthError, healthMessage: err.Error()}, true
	}

	decl := Plugin{}.OAuthDeclaration()
	rateDecl := Plugin{}.RateLimits()
	client, err := NewClient(runtimeCfg, decl, rateDecl, oauthCfg)
	if err != nil {
		return Plugin{health: core.HealthError, healthMessage: err.Error()}, true
	}

	return Plugin{client: client, health: core.HealthHealthy}, true
}

func (p Plugin) ID() string {
	return "daikin"
}

func (p Plugin) Manifest() core.Manifest {
	return core.Manifest{
		PluginID:    "daikin",
		DisplayName: "Daikin Onecta",
		Version:     "0.1.0",
		Services:    []string{"gohome.plugins.daikin.v1.DaikinService"},
	}
}

func (p Plugin) AgentsMD() string {
	return agentsMD
}

func (p Plugin) OAuthDeclaration() oauth.Declaration {
	return oauth.Declaration{
		Provider:     "daikin",
		Flow:         oauth.FlowAuthCode,
		AuthorizeURL: "https://idp.onecta.daikineurope.com/v1/oidc/authorize",
		TokenURL:     "https://idp.onecta.daikineurope.com/v1/oidc/token",
		Scope:        "openid onecta:basic.integration",
		StatePath:    "/var/lib/gohome/daikin-credentials.json",
	}
}

func (p Plugin) RateLimits() rate.Declaration {
	return rate.Provider("daikin").
		MaxRequestsPer(rate.Minute, 20).
		MaxRequestsPer(rate.Day, 200).
		CacheFor(10 * time.Minute).
		ReadHeaders(rate.StandardHeaders())
}

func (p Plugin) Dashboards() []core.Dashboard {
	return []core.Dashboard{{Name: "daikin-overview", JSON: dashboardJSON}}
}

func (p Plugin) RegisterGRPC(server *grpc.Server) {
	RegisterDaikinService(server, p.client)
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
