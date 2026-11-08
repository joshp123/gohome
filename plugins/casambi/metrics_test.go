package casambi

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joshp123/gohome/internal/core"
	casambiv1 "github.com/joshp123/gohome/proto/gen/plugins/casambi/v1"
	registryv1 "github.com/joshp123/gohome/proto/gen/registry/v1"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// recordedBridge answers like the bridge does, or fails as an unreachable one would.
type recordedBridge struct {
	casambiv1.UnimplementedCasambiServiceServer
	response *casambiv1.ListLightsResponse
	switches *casambiv1.ListSwitchesResponse
	presses  *casambiv1.ListSwitchPressesResponse
}

func (b *recordedBridge) ListScenes(context.Context, *casambiv1.ListScenesRequest) (*casambiv1.ListScenesResponse, error) {
	return &casambiv1.ListScenesResponse{}, nil
}

func (b *recordedBridge) ListSwitchPresses(context.Context, *casambiv1.ListSwitchPressesRequest) (*casambiv1.ListSwitchPressesResponse, error) {
	return b.presses, nil
}

func (b *recordedBridge) ListLights(context.Context, *casambiv1.ListLightsRequest) (*casambiv1.ListLightsResponse, error) {
	if b.response == nil {
		return nil, status.Error(codes.Unavailable, "connection refused")
	}
	return b.response, nil
}

func (b *recordedBridge) ListSwitches(context.Context, *casambiv1.ListSwitchesRequest) (*casambiv1.ListSwitchesResponse, error) {
	if b.switches == nil {
		return nil, status.Error(codes.Unavailable, "connection refused")
	}
	return b.switches, nil
}

func TestLightValuesOnlyWhileTheBridgeHasCurrentState(t *testing.T) {
	reported := timestamppb.New(time.Unix(1794013890, 0))
	connected := &casambiv1.ListLightsResponse{BluetoothConnected: true, Lights: []*casambiv1.Light{
		{Id: "kitchen", Name: "Spotlights", Online: true, On: proto.Bool(true), BrightnessPercent: proto.Uint32(80),
			Colour: &casambiv1.Colour{Red: 255, Green: 230, Blue: 208}, LastReportTime: reported},
		{Id: "strip", Name: "Front LED Strip", Online: true, On: proto.Bool(true), BrightnessPercent: proto.Uint32(50),
			Colour: &casambiv1.Colour{Red: 194, Green: 255, Blue: 169}, LastReportTime: reported},
		// No white colour temperature configured: the bridge sends no colour.
		{Id: "rear-strip", Name: "Rear LED Strip", Online: true, On: proto.Bool(true), BrightnessPercent: proto.Uint32(100), LastReportTime: reported},
		{Id: "table", Name: "Table Lamp", Online: true, On: proto.Bool(false), DimmerRaw: proto.Uint32(255), OverheatIndicator: proto.Uint32(0), LastReportTime: reported},
		{Id: "sofa", Name: "Floor Lamp", Online: false, On: proto.Bool(true), BrightnessPercent: proto.Uint32(64), OverheatIndicator: proto.Uint32(1), LastReportTime: reported},
		{Id: "unreported", Name: "Unreported"},
	}}
	pressed := timestamppb.New(time.Unix(1794013900, 0))
	switches := &casambiv1.ListSwitchesResponse{BluetoothConnected: true, Switches: []*casambiv1.Switch{
		{Id: "hall", Name: "Xpress", Online: true, BatteryLevel: proto.Uint32(200), LastReportTime: reported,
			Buttons: []*casambiv1.SwitchButton{
				{Number: 1, Action: &casambiv1.ButtonAction{TargetKind: casambiv1.ButtonTargetKind_BUTTON_TARGET_KIND_GROUP, TargetName: "Kitchen"}},
				{Number: 2, Presses: 3, LastPressTime: pressed, Action: &casambiv1.ButtonAction{TargetKind: casambiv1.ButtonTargetKind_BUTTON_TARGET_KIND_SCENE, TargetName: "Evening"}},
				{Number: 3, Action: &casambiv1.ButtonAction{TargetKind: casambiv1.ButtonTargetKind_BUTTON_TARGET_KIND_LIGHT, TargetName: "Floor Lamp"}},
				{Number: 4, Action: &casambiv1.ButtonAction{TargetKind: casambiv1.ButtonTargetKind_BUTTON_TARGET_KIND_NONE}},
				{Number: 6, Presses: 1, LastPressTime: pressed}}},
		{Id: "unreported-switch", Name: "Xpress"},
	}}
	// Bluetooth lost between the two reads: the switch state is old.
	switchesDisconnected := proto.Clone(switches).(*casambiv1.ListSwitchesResponse)
	switchesDisconnected.BluetoothConnected = false
	for _, sw := range switchesDisconnected.Switches {
		sw.Online = false
	}
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

	connectedLights := map[string]float64{
		"gohome_casambi_light_dimmer_raw{table}":                         255,
		"gohome_casambi_bluetooth_connected":                             1,
		"gohome_casambi_light_online{kitchen}":                           1,
		"gohome_casambi_light_on{kitchen}":                               1,
		"gohome_casambi_light_brightness_percent{kitchen}":               80,
		"gohome_casambi_light_colour_info{kitchen,#ffe6d0}":              1,
		"gohome_casambi_light_last_report_timestamp_seconds{kitchen}":    1794013890,
		"gohome_casambi_light_online{strip}":                             1,
		"gohome_casambi_light_on{strip}":                                 1,
		"gohome_casambi_light_brightness_percent{strip}":                 50,
		"gohome_casambi_light_colour_info{strip,#c2ffa9}":                1,
		"gohome_casambi_light_online{rear-strip}":                        1,
		"gohome_casambi_light_on{rear-strip}":                            1,
		"gohome_casambi_light_brightness_percent{rear-strip}":            100,
		"gohome_casambi_light_last_report_timestamp_seconds{rear-strip}": 1794013890,
		"gohome_casambi_light_last_report_timestamp_seconds{strip}":      1794013890,
		"gohome_casambi_light_online{table}":                             1,
		"gohome_casambi_light_on{table}":                                 0,
		"gohome_casambi_light_overheat_indicator{table}":                 0,
		"gohome_casambi_light_last_report_timestamp_seconds{table}":      1794013890,
		"gohome_casambi_light_online{sofa}":                              0,
		"gohome_casambi_light_last_report_timestamp_seconds{sofa}":       1794013890,
		"gohome_casambi_light_online{unreported}":                        0,
	}
	withSwitches := map[string]float64{
		"gohome_casambi_scrape_success":                                                    1,
		"gohome_casambi_switch_online{hall}":                                               1,
		"gohome_casambi_switch_battery_level{hall}":                                        200,
		"gohome_casambi_switch_last_report_timestamp_seconds{hall}":                        1794013890,
		"gohome_casambi_switch_button_presses_total{hall,1,group Kitchen}":                 0,
		"gohome_casambi_switch_button_presses_total{hall,2,scene Evening}":                 3,
		"gohome_casambi_switch_button_last_press_timestamp_seconds{hall,2,scene Evening}":  1794013900,
		"gohome_casambi_switch_button_presses_total{hall,3,light Floor Lamp}":                    0,
		"gohome_casambi_switch_button_presses_total{hall,4,none}":                          0,
		"gohome_casambi_switch_button_presses_total{hall,6,not configured}":                1,
		"gohome_casambi_switch_button_last_press_timestamp_seconds{hall,6,not configured}": 1794013900,
		"gohome_casambi_switch_online{unreported-switch}":                                  0,
	}
	for key, value := range connectedLights {
		withSwitches[key] = value
	}
	// Lights read, switches not: the light values are current, but the read failed.
	switchesFailed := map[string]float64{"gohome_casambi_scrape_success": 0}
	for key, value := range connectedLights {
		switchesFailed[key] = value
	}

	cases := []struct {
		name     string
		response *casambiv1.ListLightsResponse
		switches *casambiv1.ListSwitchesResponse
		want     map[string]float64
	}{
		{"connected", connected, switches, withSwitches},
		{"switch read failed", connected, nil, switchesFailed},
		{"Bluetooth lost before the switch read", connected, switchesDisconnected, switchesFailed},
		{"bridge without Bluetooth", noBluetooth, switches, map[string]float64{
			"gohome_casambi_scrape_success":      0,
			"gohome_casambi_bluetooth_connected": 0,
		}},
		{"bridge unreachable", nil, nil, map[string]float64{
			"gohome_casambi_scrape_success": 0,
		}},
	}
	var firstSuccess float64
	for _, c := range cases {
		bridge.response = c.response
		bridge.switches = c.switches
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

func TestGohomeServesTheBridgesSwitchPresses(t *testing.T) {
	recorded := &casambiv1.ListSwitchPressesResponse{Presses: []*casambiv1.SwitchPress{{
		SwitchId: "hall", SwitchName: "Xpress", Button: 2, PressTime: timestamppb.New(time.Unix(1794013900, 0)),
		ConfiguredAction: &casambiv1.ButtonAction{TargetKind: casambiv1.ButtonTargetKind_BUTTON_TARGET_KIND_SCENE, TargetName: "Evening"},
		LightChanges: []*casambiv1.LightChange{{LightId: "kitchen", Name: "Spotlights", OnBefore: proto.Bool(false), OnAfter: true,
			BrightnessPercentAfter: proto.Uint32(100), ColourAfter: &casambiv1.Colour{Red: 255, Green: 230, Blue: 208}}},
	}}}
	bridge := serve(t, func(server *grpc.Server) {
		casambiv1.RegisterCasambiServiceServer(server, &recordedBridge{presses: recorded})
	})
	client, err := NewBridgeClient(bridge)
	if err != nil {
		t.Fatal(err)
	}
	gohome := serve(t, Plugin{bridge: client, health: core.HealthHealthy}.RegisterGRPC)
	conn, err := grpc.NewClient(gohome, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	got, err := casambiv1.NewCasambiServiceClient(conn).ListSwitchPresses(context.Background(), &casambiv1.ListSwitchPressesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(got, recorded) {
		t.Fatalf("got %v, want %v", got, recorded)
	}
}

func serve(t *testing.T, register func(*grpc.Server)) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	register(server)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	return listener.Addr().String()
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
			labels := map[string]string{}
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			var ids []string
			for _, name := range []string{"light_id", "switch_id", "button", "action", "colour"} {
				if value, ok := labels[name]; ok {
					ids = append(ids, value)
				}
			}
			if len(ids) > 0 {
				key += "{" + strings.Join(ids, ",") + "}"
			}
			if metric.GetCounter() != nil {
				values[key] = metric.GetCounter().GetValue()
			} else {
				values[key] = metric.GetGauge().GetValue()
			}
		}
	}
	return values
}
