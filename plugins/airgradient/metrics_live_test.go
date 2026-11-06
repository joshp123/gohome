package airgradient

import (
	"context"
	"os"
	"testing"

	"github.com/joshp123/gohome/internal/core"
	registryv1 "github.com/joshp123/gohome/proto/gen/registry/v1"
)

// This opt-in probe reads a real sensor, then its unreachable former address.
func TestLiveSourceFailureRemovesReadings(t *testing.T) {
	liveURL, downURL := os.Getenv("AIRGRADIENT_LIVE_URL"), os.Getenv("AIRGRADIENT_DOWN_URL")
	if liveURL == "" || downURL == "" {
		t.Skip("requires live and unreachable source URLs")
	}
	client, err := NewClient(Config{BaseURL: liveURL})
	if err != nil {
		t.Fatal(err)
	}
	plugins := core.ObservePlugins([]core.Plugin{Plugin{client: client, health: core.HealthHealthy}})
	metrics := core.MetricsRegistry(plugins)
	registry := core.NewRegistryService(plugins)
	initial, err := registry.ListPlugins(context.Background(), &registryv1.ListPluginsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	pending := initial.Plugins[0]
	if pending.Status != string(core.HealthDegraded) || pending.LastSuccess != nil || pending.HealthMessage != "Awaiting first source read" {
		t.Fatalf("unexpected health before first scrape: %v", pending)
	}
	var lastSuccess int64
	for _, available := range []bool{true, false, true} {
		client.baseURL = liveURL
		if !available {
			client.baseURL = downURL
		}
		families, err := metrics.Gather()
		if err != nil {
			t.Fatal(err)
		}
		hasCO2 := false
		for _, family := range families {
			if family.GetName() == "gohome_airgradient_co2_ppm" {
				hasCO2 = true
			}
		}
		if hasCO2 != available {
			t.Fatalf("source available=%v, CO2 emitted=%v", available, hasCO2)
		}
		response, err := registry.ListPlugins(context.Background(), &registryv1.ListPluginsRequest{})
		if err != nil {
			t.Fatal(err)
		}
		source := response.Plugins[0]
		if (source.Status == string(core.HealthHealthy)) != available {
			t.Fatalf("source available=%v, registry=%s", available, source.Status)
		}
		if source.LastSuccess == nil {
			t.Fatal("missing last successful read")
		}
		if !available && source.LastSuccess.Seconds != lastSuccess {
			t.Fatal("failed read advanced success timestamp")
		}
		lastSuccess = source.LastSuccess.Seconds
	}
}
