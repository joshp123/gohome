package casambi

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	casambiv1 "github.com/joshp123/gohome/proto/gen/plugins/casambi/v1"
	"github.com/prometheus/client_golang/prometheus"
)

const readTimeout = 5 * time.Second

var (
	lightLabels  = []string{"light_id", "name"}
	switchLabels = []string{"switch_id", "name"}
	buttonLabels = []string{"switch_id", "name", "button", "action"}
)

var (
	lightDimmerRawDesc     = prometheus.NewDesc("gohome_casambi_light_dimmer_raw", "Driver dimmer setting 0 to 255, including while off; not emitted brightness", lightLabels, nil)
	lightMixerRawDesc      = prometheus.NewDesc("gohome_casambi_light_white_colour_balance_raw", "Driver RGB/white mixer setting 0 to 63, including while off", lightLabels, nil)
	lightColourSettingDesc = prometheus.NewDesc("gohome_casambi_light_colour_setting", "Driver RGB channel setting 0 to 255, including when colour LEDs are dark; not emitted colour", []string{"light_id", "name", "channel"}, nil)
	sceneAvailableDesc     = prometheus.NewDesc("gohome_casambi_scene_available", "Configured scene exists and Bluetooth is connected", []string{"scene_id", "name"}, nil)
	scrapeSuccessDesc      = prometheus.NewDesc("gohome_casambi_scrape_success",
		"gohome read the lights and switches (1): the bridge answered and holds a Bluetooth session. Otherwise 0", nil, nil)
	lastSuccessDesc = prometheus.NewDesc("gohome_casambi_last_success_timestamp_seconds",
		"When gohome last read the lights and switches successfully (epoch seconds)", nil, nil)
	bluetoothConnectedDesc = prometheus.NewDesc("gohome_casambi_bluetooth_connected",
		"The bridge holds an authenticated Bluetooth session to the Casambi network (1) or not (0)", nil, nil)

	lightOnlineDesc = prometheus.NewDesc("gohome_casambi_light_online",
		"The light's last report said it was online (1) or not (0)", lightLabels, nil)
	lightOnDesc = prometheus.NewDesc("gohome_casambi_light_on",
		"The light is on (1) or off (0); absent while it is offline", lightLabels, nil)
	lightBrightnessDesc = prometheus.NewDesc("gohome_casambi_light_brightness_percent",
		"The light's brightness, 0 to 100; present only while it is online and on", lightLabels, nil)
	lightColourDesc = prometheus.NewDesc("gohome_casambi_light_colour_info",
		"Always 1; colour is the colour of the light the light gives now, as #rrggbb at full brightness, from the white's configured (not measured) colour temperature and, for lights with colour LEDs, their colour and Casambi's white/colour mixer; present only while online and on", append(lightLabels, "colour"), nil)
	lightOverheatDesc = prometheus.NewDesc("gohome_casambi_light_overheat_indicator",
		"The driver's raw overheat indicator, 0 to 3 (0 is Casambi's default; other values are undocumented); absent while offline", lightLabels, nil)
	lightLastReportDesc = prometheus.NewDesc("gohome_casambi_light_last_report_timestamp_seconds",
		"When the bridge last received a report from the light (epoch seconds)", lightLabels, nil)

	switchOnlineDesc = prometheus.NewDesc("gohome_casambi_switch_online",
		"The wall switch's last report said it was online (1) or not (0)", switchLabels, nil)
	switchBatteryDesc = prometheus.NewDesc("gohome_casambi_switch_battery_level",
		"The wall switch's raw battery level from its last report, 0 to 255, on Casambi's undocumented scale", switchLabels, nil)
	switchLastReportDesc = prometheus.NewDesc("gohome_casambi_switch_last_report_timestamp_seconds",
		"When the bridge last received a state report from the switch (epoch seconds)", switchLabels, nil)
	buttonPressesDesc = prometheus.NewDesc("gohome_casambi_switch_button_presses_total",
		"Presses of the switch button the bridge has seen since it started; action is what the Casambi configuration says the button does", buttonLabels, nil)
	buttonLastPressDesc = prometheus.NewDesc("gohome_casambi_switch_button_last_press_timestamp_seconds",
		"When the bridge last saw the switch button pressed (epoch seconds)", buttonLabels, nil)
)

// MetricsCollector reads the bridge at scrape time. Without a Bluetooth
// session the bridge's state is old, so a scrape then emits no light or switch values.
type MetricsCollector struct {
	bridge casambiv1.CasambiServiceClient

	mu          sync.Mutex
	lastSuccess time.Time
}

func NewMetricsCollector(bridge casambiv1.CasambiServiceClient) *MetricsCollector {
	return &MetricsCollector{bridge: bridge}
}

func (c *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range []*prometheus.Desc{scrapeSuccessDesc, lastSuccessDesc, bluetoothConnectedDesc, lightDimmerRawDesc, lightMixerRawDesc, lightColourSettingDesc, sceneAvailableDesc,
		lightOnlineDesc, lightOnDesc, lightBrightnessDesc, lightColourDesc, lightOverheatDesc, lightLastReportDesc,
		switchOnlineDesc, switchBatteryDesc, switchLastReportDesc, buttonPressesDesc, buttonLastPressDesc} {
		ch <- desc
	}
}

func (c *MetricsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()

	c.mu.Lock()
	defer c.mu.Unlock()
	emit := func(desc *prometheus.Desc, kind prometheus.ValueType, value float64, labels ...string) {
		ch <- prometheus.MustNewConstMetric(desc, kind, value, labels...)
	}
	gauge := func(desc *prometheus.Desc, value float64, labels ...string) {
		emit(desc, prometheus.GaugeValue, value, labels...)
	}

	lights, err := c.bridge.ListLights(ctx, &casambiv1.ListLightsRequest{})
	if err != nil {
		log.Printf("casambi bridge read failed: %v", err)
		gauge(scrapeSuccessDesc, 0)
		c.emitLastSuccess(gauge)
		return
	}
	gauge(bluetoothConnectedDesc, boolValue(lights.GetBluetoothConnected()))
	if !lights.GetBluetoothConnected() {
		log.Printf("casambi bridge has no Bluetooth session")
		gauge(scrapeSuccessDesc, 0)
		c.emitLastSuccess(gauge)
		return
	}
	for _, light := range lights.GetLights() {
		labels := []string{light.GetId(), light.GetName()}
		gauge(lightOnlineDesc, boolValue(light.GetOnline()), labels...)
		if light.LastReportTime != nil {
			gauge(lightLastReportDesc, float64(light.GetLastReportTime().GetSeconds()), labels...)
		}
		if !light.GetOnline() {
			continue
		}
		optionalGauge := func(desc *prometheus.Desc, value *uint32) {
			if value != nil {
				gauge(desc, float64(*value), labels...)
			}
		}
		if light.On != nil {
			gauge(lightOnDesc, boolValue(light.GetOn()), labels...)
		}
		optionalGauge(lightBrightnessDesc, light.BrightnessPercent)
		if colour := light.GetColour(); colour != nil {
			gauge(lightColourDesc, 1, append(labels, hexColour(colour))...)
		}
		optionalGauge(lightOverheatDesc, light.OverheatIndicator)
		optionalGauge(lightDimmerRawDesc, light.DimmerRaw)
		optionalGauge(lightMixerRawDesc, light.WhiteColourBalanceRaw)
		if colour := light.ColourSetting; colour != nil {
			for channel, value := range map[string]uint32{"red": colour.Red, "green": colour.Green, "blue": colour.Blue} {
				gauge(lightColourSettingDesc, float64(value), light.Id, light.Name, channel)
			}
		}
	}

	switches, err := c.bridge.ListSwitches(ctx, &casambiv1.ListSwitchesRequest{})
	if err != nil || !switches.GetBluetoothConnected() {
		log.Printf("casambi bridge switch read failed: err=%v, bluetooth_connected=%t", err, switches.GetBluetoothConnected())
		gauge(scrapeSuccessDesc, 0)
		c.emitLastSuccess(gauge)
		return
	}
	for _, sw := range switches.GetSwitches() {
		labels := []string{sw.GetId(), sw.GetName()}
		gauge(switchOnlineDesc, boolValue(sw.GetOnline()), labels...)
		if sw.BatteryLevel != nil {
			gauge(switchBatteryDesc, float64(sw.GetBatteryLevel()), labels...)
		}
		if sw.LastReportTime != nil {
			gauge(switchLastReportDesc, float64(sw.GetLastReportTime().GetSeconds()), labels...)
		}
		for _, button := range sw.GetButtons() {
			buttonLabels := []string{sw.GetId(), sw.GetName(), strconv.FormatUint(uint64(button.GetNumber()), 10), actionLabel(button.GetAction())}
			emit(buttonPressesDesc, prometheus.CounterValue, float64(button.GetPresses()), buttonLabels...)
			if button.LastPressTime != nil {
				gauge(buttonLastPressDesc, float64(button.GetLastPressTime().GetSeconds()), buttonLabels...)
			}
		}
	}
	scenes, err := c.bridge.ListScenes(ctx, &casambiv1.ListScenesRequest{})
	if err != nil {
		log.Printf("casambi scenes: %v", err)
		gauge(scrapeSuccessDesc, 0)
		c.emitLastSuccess(gauge)
		return
	}
	for _, scene := range scenes.GetScenes() {
		gauge(sceneAvailableDesc, boolValue(scene.Available), strconv.FormatUint(uint64(scene.Id), 10), scene.Name)
	}
	c.lastSuccess = time.Now()
	gauge(scrapeSuccessDesc, 1)
	c.emitLastSuccess(gauge)
}

func (c *MetricsCollector) emitLastSuccess(gauge func(*prometheus.Desc, float64, ...string)) {
	if !c.lastSuccess.IsZero() {
		gauge(lastSuccessDesc, float64(c.lastSuccess.Unix()))
	}
}

// actionLabel names what a button does, such as "scene Evening" or "light Floor Lamp".
// One value per configured button, so the label stays bounded.
func actionLabel(action *casambiv1.ButtonAction) string {
	switch action.GetTargetKind() {
	case casambiv1.ButtonTargetKind_BUTTON_TARGET_KIND_LIGHT:
		return "light " + action.GetTargetName()
	case casambiv1.ButtonTargetKind_BUTTON_TARGET_KIND_GROUP:
		return "group " + action.GetTargetName()
	case casambiv1.ButtonTargetKind_BUTTON_TARGET_KIND_SCENE:
		return "scene " + action.GetTargetName()
	case casambiv1.ButtonTargetKind_BUTTON_TARGET_KIND_NONE:
		return "none"
	case casambiv1.ButtonTargetKind_BUTTON_TARGET_KIND_OTHER:
		return "other"
	default:
		return "not configured"
	}
}

// hexColour writes a colour as #rrggbb, the form Grafana takes for a colour.
func hexColour(colour *casambiv1.Colour) string {
	return fmt.Sprintf("#%02x%02x%02x", colour.GetRed(), colour.GetGreen(), colour.GetBlue())
}

func boolValue(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
