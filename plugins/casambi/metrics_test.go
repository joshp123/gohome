package casambi

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/joshp123/gohome/internal/core"
	casambiv1 "github.com/joshp123/gohome/proto/gen/plugins/casambi/v1"
	registryv1 "github.com/joshp123/gohome/proto/gen/registry/v1"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// recordedBridge answers like the bridge does, or fails as an unreachable one would.
type recordedBridge struct {
	casambiv1.UnimplementedCasambiServiceServer
	response *casambiv1.ListLightsResponse
}

func (b *recordedBridge) ListLights(context.Context, *casambiv1.ListLightsRequest) (*casambiv1.ListLightsResponse, error) {
	if b.response == nil {
		return nil, status.Error(codes.Unavailable, "connection refused")
	}
	return b.response, nil
}

func TestLightValuesOnlyWhileTheBridgeHasCurrentState(t *testing.T) {
	reported := timestamppb.New(time.Unix(1794013890, 0))
	connected := &casambiv1.ListLightsResponse{BluetoothConnected: true, Lights: []*casambiv1.Light{
		{Id: "kitchen", Name: "Spotlights", Online: true, On: proto.Bool(true), BrightnessPercent: proto.Uint32(80), LastReportTime: reported},
		{Id: "sofa", Name: "Floor Lamp", Online: false, On: proto.Bool(false), BrightnessPercent: proto.Uint32(0), LastReportTime: reported},
		{Id: "unreported", Name: "Unreported"},
	}}
	noBluetooth := proto.Clone(connected).(*casambiv1.ListLightsResponse)
	noBluetooth.BluetoothConnected = false
	for _, light := range noBluetooth.Lights {
		light.Online = false
	}

	bridge := &recordedBridge{}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	casambiv1.RegisterCasambiServiceServer(server, bridge)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	client, err := NewBridgeClient(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	plugins := core.ObservePlugins([]core.Plugin{Plugin{bridge: client, health: core.HealthHealthy}})
	metrics := core.MetricsRegistry(plugins)

	cases := []struct {
		name     string
		response *casambiv1.ListLightsResponse
		want     map[string]float64
	}{
		{"connected", connected, map[string]float64{
			"gohome_casambi_scrape_success":                               1,
			"gohome_casambi_bluetooth_connected":                          1,
			"gohome_casambi_light_online{kitchen}":                        1,
			"gohome_casambi_light_on{kitchen}":                            1,
			"gohome_casambi_light_brightness_percent{kitchen}":            80,
			"gohome_casambi_light_last_report_timestamp_seconds{kitchen}": 1794013890,
			"gohome_casambi_light_online{sofa}":                           0,
			"gohome_casambi_light_last_report_timestamp_seconds{sofa}":    1794013890,
			"gohome_casambi_light_online{unreported}":                     0,
		}},
		{"bridge without Bluetooth", noBluetooth, map[string]float64{
			"gohome_casambi_scrape_success":      0,
			"gohome_casambi_bluetooth_connected": 0,
		}},
		{"bridge unreachable", nil, map[string]float64{
			"gohome_casambi_scrape_success": 0,
		}},
	}
	var firstSuccess float64
	for _, c := range cases {
		bridge.response = c.response
		got := gather(t, metrics)
		lastSuccess, ok := got["gohome_casambi_last_success_timestamp_seconds"]
		if !ok {
			t.Fatalf("%s: missing last success", c.name)
		}
		if firstSuccess == 0 {
			firstSuccess = lastSuccess
		} else if lastSuccess != firstSuccess {
			t.Fatalf("%s: failed read moved last success", c.name)
		}
		delete(got, "gohome_casambi_last_success_timestamp_seconds")
		if len(got) != len(c.want) {
			t.Fatalf("%s: got %v, want %v", c.name, got, c.want)
		}
		for key, value := range c.want {
			if got[key] != value {
				t.Fatalf("%s: %s = %v, want %v", c.name, key, got[key], value)
			}
		}
		healthy := c.want["gohome_casambi_scrape_success"] == 1
		if status := pluginStatus(t, plugins); (status == string(core.HealthHealthy)) != healthy {
			t.Fatalf("%s: registry status %s", c.name, status)
		}
	}
}

// Opt-in: reads a running light bridge (ListLights only).
func TestLiveBridge(t *testing.T) {
	address := os.Getenv("CASAMBI_LIVE_BRIDGE")
	if address == "" {
		t.Skip("requires CASAMBI_LIVE_BRIDGE=host:port")
	}
	client, err := NewBridgeClient(address)
	if err != nil {
		t.Fatal(err)
	}
	plugins := core.ObservePlugins([]core.Plugin{Plugin{bridge: client, health: core.HealthHealthy}})
	values := gather(t, core.MetricsRegistry(plugins))
	if values["gohome_casambi_scrape_success"] != 1 {
		t.Fatalf("bridge not read: %v", values)
	}
	for key, value := range values {
		t.Logf("%s = %v", key, value)
	}
	t.Logf("registry status %s", pluginStatus(t, plugins))
}

func pluginStatus(t *testing.T, plugins []core.Plugin) string {
	t.Helper()
	response, err := core.NewRegistryService(plugins).ListPlugins(context.Background(), &registryv1.ListPluginsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	return response.Plugins[0].Status
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
			key := family.GetName()
			for _, label := range metric.GetLabel() {
				if label.GetName() == "light_id" {
					key += "{" + label.GetValue() + "}"
				}
			}
			values[key] = metric.GetGauge().GetValue()
		}
	}
	return values
}
