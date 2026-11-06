package home

import (
	"embed"

	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/internal/oauth"
	homev1 "github.com/joshp123/gohome/proto/gen/plugins/home/v1"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

//go:embed AGENTS.md
var agentsMD string

//go:embed dashboards/*.json
var dashboardFiles embed.FS

// houseDashboards are the whole-house views, in the order they appear in Grafana's House folder.
var houseDashboards = []string{"home", "rooms", "heating", "curve", "humidity", "energy"}

// Plugin implements the GoHome plugin contract.
type Plugin struct {
	health        core.HealthStatus
	healthMessage string
}

// NewPlugin constructs the My Home dashboard plugin.
func NewPlugin(cfg *homev1.HomeConfig) (Plugin, bool) {
	if cfg == nil {
		return Plugin{}, false
	}
	return Plugin{health: core.HealthHealthy}, true
}

func (p Plugin) ID() string {
	return "home"
}

func (p Plugin) Manifest() core.Manifest {
	return core.Manifest{
		PluginID:    "home",
		DisplayName: "My Home",
		Version:     "0.1.0",
		Services:    nil,
	}
}

func (p Plugin) AgentsMD() string {
	return agentsMD
}

func (p Plugin) OAuthDeclaration() oauth.Declaration {
	return oauth.Declaration{}
}

func (p Plugin) Dashboards() []core.Dashboard {
	dashboards := make([]core.Dashboard, 0, len(houseDashboards))
	for _, name := range houseDashboards {
		content, err := dashboardFiles.ReadFile("dashboards/house-" + name + ".json")
		if err != nil {
			panic(err) // embedded at build time, so a missing file is a build mistake
		}
		dashboards = append(dashboards, core.Dashboard{Name: name, Folder: "House", JSON: content})
	}
	return dashboards
}

func (p Plugin) RegisterGRPC(*grpc.Server) {}

func (p Plugin) Collectors() []prometheus.Collector {
	return nil
}

func (p Plugin) Health() core.HealthStatus {
	return p.health
}

func (p Plugin) HealthMessage() string {
	return p.healthMessage
}
