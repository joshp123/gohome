package core

import (
	"context"
	"testing"

	registryv1 "github.com/joshp123/gohome/proto/gen/registry/v1"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

type stubPlugin struct {
	id            string
	name          string
	version       string
	services      []string
	dashboards    []Dashboard
	agents        string
	health        HealthStatus
	healthMessage string
}

func (s stubPlugin) ID() string { return s.id }

func (s stubPlugin) Manifest() Manifest {
	return Manifest{
		PluginID:    s.id,
		DisplayName: s.name,
		Version:     s.version,
		Services:    s.services,
	}
}

func (s stubPlugin) AgentsMD() string { return s.agents }

func (s stubPlugin) Dashboards() []Dashboard { return s.dashboards }

func (s stubPlugin) RegisterGRPC(*grpc.Server) {}

func (s stubPlugin) Collectors() []prometheus.Collector { return nil }

func (s stubPlugin) Health() HealthStatus { return s.health }

func (s stubPlugin) HealthMessage() string { return s.healthMessage }

func newStubPlugin(id string) stubPlugin {
	return stubPlugin{
		id:         id,
		name:       "Demo",
		version:    "0.1.0",
		services:   []string{"gohome.plugins.demo.v1.DemoService"},
		agents:     "demo agents",
		health:     HealthHealthy,
		dashboards: []Dashboard{{Name: "demo", JSON: []byte("{}")}},
	}
}

func TestRegistryListPlugins(t *testing.T) {
	plugin := newStubPlugin("demo")
	svc := NewRegistryService([]Plugin{plugin})

	resp, err := svc.ListPlugins(context.Background(), &registryv1.ListPluginsRequest{})
	if err != nil {
		t.Fatalf("ListPlugins error: %v", err)
	}
	if len(resp.Plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(resp.Plugins))
	}

	got := resp.Plugins[0]
	if got.PluginId != "demo" || got.DisplayName != "Demo" || got.Version != "0.1.0" {
		t.Fatalf("unexpected plugin summary: %+v", got)
	}
	if got.Status != string(HealthHealthy) {
		t.Fatalf("unexpected health status: %s", got.Status)
	}
}

func TestRegistryDescribePlugin(t *testing.T) {
	plugin := newStubPlugin("demo")
	svc := NewRegistryService([]Plugin{plugin})

	resp, err := svc.DescribePlugin(context.Background(), &registryv1.DescribePluginRequest{PluginId: "demo"})
	if err != nil {
		t.Fatalf("DescribePlugin error: %v", err)
	}
	if resp.Plugin == nil {
		t.Fatalf("expected plugin descriptor")
	}
	if resp.Plugin.PluginId != "demo" {
		t.Fatalf("unexpected plugin id: %s", resp.Plugin.PluginId)
	}
	if len(resp.Plugin.Dashboards) != 1 {
		t.Fatalf("expected 1 dashboard, got %d", len(resp.Plugin.Dashboards))
	}
	if resp.Plugin.Dashboards[0].Path != "/dashboards/demo/demo.json" {
		t.Fatalf("unexpected dashboard path: %s", resp.Plugin.Dashboards[0].Path)
	}
}

func TestFilterPlugins(t *testing.T) {
	compiled := []Plugin{newStubPlugin("demo"), newStubPlugin("extra")}

	active := FilterPlugins(compiled, map[string]bool{"demo": true}, false)
	if len(active) != 1 || active[0].ID() != "demo" {
		t.Fatalf("unexpected active plugins: %v", active)
	}

	active = FilterPlugins(compiled, map[string]bool{}, true)
	if len(active) != 2 {
		t.Fatalf("expected all plugins, got %d", len(active))
	}
}

func TestValidateEnabledPlugins(t *testing.T) {
	compiled := []Plugin{newStubPlugin("demo")}

	if err := ValidateEnabledPlugins(compiled, map[string]bool{"demo": true}, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := ValidateEnabledPlugins(compiled, map[string]bool{"missing": true}, false); err == nil {
		t.Fatalf("expected error for missing plugin")
	}
}
