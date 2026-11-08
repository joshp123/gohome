package casambi

import (
	"context"
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
	buttonLabels = []string{"switch_id", "name", "button"}
)

var (
	scrapeSuccessDesc = prometheus.NewDesc("gohome_casambi_scrape_success",
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
	lightHueDesc = prometheus.NewDesc("gohome_casambi_light_hue_degrees",
		"The colour light's hue, 0 to 359 degrees; present only while it is online and on, with saturation above 0", lightLabels, nil)
	lightSaturationDesc = prometheus.NewDesc("gohome_casambi_light_saturation_percent",
		"The colour light's saturation, 0 (white) to 100 (pure colour); present only while it is online and on", lightLabels, nil)
	lightWhiteColourBalanceDesc = prometheus.NewDesc("gohome_casambi_light_white_colour_balance",
		"Casambi's balance between the colour and white channels, raw 0 to 63 (Casambi's default 31; which end is white only is undocumented); present only while online and on", lightLabels, nil)
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
		"Presses of the switch button the bridge has seen since it started", buttonLabels, nil)
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
	for _, desc := range []*prometheus.Desc{scrapeSuccessDesc, lastSuccessDesc, bluetoothConnectedDesc,
		lightOnlineDesc, lightOnDesc, lightBrightnessDesc, lightHueDesc, lightSaturationDesc,
		lightWhiteColourBalanceDesc, lightOverheatDesc, lightLastReportDesc,
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
		optionalGauge(lightHueDesc, light.HueDegrees)
		optionalGauge(lightSaturationDesc, light.SaturationPercent)
		optionalGauge(lightWhiteColourBalanceDesc, light.WhiteColourBalance)
		optionalGauge(lightOverheatDesc, light.OverheatIndicator)
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
			buttonLabels := []string{sw.GetId(), sw.GetName(), strconv.FormatUint(uint64(button.GetNumber()), 10)}
			emit(buttonPressesDesc, prometheus.CounterValue, float64(button.GetPresses()), buttonLabels...)
			gauge(buttonLastPressDesc, float64(button.GetLastPressTime().GetSeconds()), buttonLabels...)
		}
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

func boolValue(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
