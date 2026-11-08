package shelly

import (
	"context"
	"os"
	"testing"

	"github.com/joshp123/gohome/internal/core"
	registryv1 "github.com/joshp123/gohome/proto/gen/registry/v1"
	"github.com/prometheus/client_golang/prometheus"
)

// This opt-in probe reads a real relay (Shelly.GetInfoExt only), then an
// unreachable address, then the real relay under the wrong device ID.
func TestLiveSourceFailureRemovesReadings(t *testing.T) {
	liveAddress, deviceID, downAddress := os.Getenv("SHELLY_LIVE_ADDRESS"), os.Getenv("SHELLY_LIVE_DEVICE_ID"), os.Getenv("SHELLY_DOWN_ADDRESS")
	if liveAddress == "" || deviceID == "" || downAddress == "" {
		t.Skip("requires SHELLY_LIVE_ADDRESS, SHELLY_LIVE_DEVICE_ID and SHELLY_DOWN_ADDRESS")
	}
	client := NewClient([]CoverConfig{{DeviceID: deviceID, Address: liveAddress, Name: "Test shutter"}})
	plugins := core.ObservePlugins([]core.Plugin{Plugin{client: client, health: core.HealthHealthy}})
	metrics := core.MetricsRegistry(plugins)
	registry := core.NewRegistryService(plugins)

	pluginHealth := func() *registryv1.PluginSummary {
		response, err := registry.ListPlugins(context.Background(), &registryv1.ListPluginsRequest{})
		if err != nil {
			t.Fatal(err)
		}
		return response.Plugins[0]
	}
	if pending := pluginHealth(); pending.Status != string(core.HealthDegraded) || pending.LastSuccess != nil {
		t.Fatalf("unexpected health before first scrape: %v", pending)
	}

	cases := []struct {
		name, address, deviceID string
		readable                bool
	}{
		{"live relay", liveAddress, deviceID, true},
		{"unreachable address", downAddress, deviceID, false},
		{"live relay again", liveAddress, deviceID, true},
		{"different relay at the address", liveAddress, "Shelly2PMG3-000000000000", false},
	}
	var lastSuccess int64
	for _, c := range cases {
		client.covers[0].Address, client.covers[0].DeviceID = c.address, c.deviceID
		values := gather(t, metrics)
		if got := values["gohome_shelly_scrape_success"]; got != boolValue(c.readable) {
			t.Fatalf("%s: scrape_success=%v", c.name, got)
		}
		for _, name := range []string{"gohome_shelly_cover_position_percent", "gohome_shelly_cover_moving",
			"gohome_shelly_cover_calibrated", "gohome_shelly_device_temperature_celsius", "gohome_shelly_wifi_rssi_dbm"} {
			if _, present := values[name]; present != c.readable {
				t.Fatalf("%s: %s present=%v", c.name, name, present)
			}
		}
		source := pluginHealth()
		if (source.Status == string(core.HealthHealthy)) != c.readable {
			t.Fatalf("%s: registry=%s", c.name, source.Status)
		}
		if source.LastSuccess == nil {
			t.Fatalf("%s: missing last successful read", c.name)
		}
		if !c.readable && source.LastSuccess.Seconds != lastSuccess {
			t.Fatalf("%s: failed read advanced success timestamp", c.name)
		}
		lastSuccess = source.LastSuccess.Seconds
		t.Logf("%s: %v", c.name, values)
	}
}

func gather(t *testing.T, registry *prometheus.Registry) map[string]float64 {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]float64{}
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			values[family.GetName()] = metric.GetGauge().GetValue()
		}
	}
	return values
}
