package casambi

import (
	"context"
	"log"
	"sync"
	"time"

	casambiv1 "github.com/joshp123/gohome/proto/gen/plugins/casambi/v1"
	"github.com/prometheus/client_golang/prometheus"
)

const readTimeout = 5 * time.Second

var lightLabels = []string{"light_id", "name"}

var (
	scrapeSuccessDesc = prometheus.NewDesc("gohome_casambi_scrape_success",
		"gohome read the lights (1): the bridge answered and holds a Bluetooth session. Otherwise 0", nil, nil)
	lastSuccessDesc = prometheus.NewDesc("gohome_casambi_last_success_timestamp_seconds",
		"When gohome last read the lights successfully (epoch seconds)", nil, nil)
	bluetoothConnectedDesc = prometheus.NewDesc("gohome_casambi_bluetooth_connected",
		"The bridge holds an authenticated Bluetooth session to the Casambi network (1) or not (0)", nil, nil)
	lightOnlineDesc = prometheus.NewDesc("gohome_casambi_light_online",
		"The light's last report said it was online (1) or not (0)", lightLabels, nil)
	lightOnDesc = prometheus.NewDesc("gohome_casambi_light_on",
		"The light is on (1) or off (0); absent while it is offline", lightLabels, nil)
	lightBrightnessDesc = prometheus.NewDesc("gohome_casambi_light_brightness_percent",
		"The light's brightness, 0 to 100; absent while it is offline", lightLabels, nil)
	lightLastReportDesc = prometheus.NewDesc("gohome_casambi_light_last_report_timestamp_seconds",
		"When the bridge last received a report from the light (epoch seconds)", lightLabels, nil)
)

// MetricsCollector reads the bridge at scrape time. Without a Bluetooth
// session the bridge's light state is old, so a scrape then emits no light values.
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
		lightOnlineDesc, lightOnDesc, lightBrightnessDesc, lightLastReportDesc} {
		ch <- desc
	}
}

func (c *MetricsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()

	c.mu.Lock()
	defer c.mu.Unlock()
	gauge := func(desc *prometheus.Desc, value float64, labels ...string) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, value, labels...)
	}

	response, err := c.bridge.ListLights(ctx, &casambiv1.ListLightsRequest{})
	switch {
	case err != nil:
		log.Printf("casambi bridge read failed: %v", err)
		gauge(scrapeSuccessDesc, 0)
	case !response.GetBluetoothConnected():
		log.Printf("casambi bridge has no Bluetooth session")
		gauge(scrapeSuccessDesc, 0)
		gauge(bluetoothConnectedDesc, 0)
	default:
		c.lastSuccess = time.Now()
		gauge(scrapeSuccessDesc, 1)
		gauge(bluetoothConnectedDesc, 1)
		for _, light := range response.GetLights() {
			labels := []string{light.GetId(), light.GetName()}
			gauge(lightOnlineDesc, boolValue(light.GetOnline()), labels...)
			if light.LastReportTime != nil {
				gauge(lightLastReportDesc, float64(light.GetLastReportTime().GetSeconds()), labels...)
			}
			if !light.GetOnline() {
				continue
			}
			if light.On != nil {
				gauge(lightOnDesc, boolValue(light.GetOn()), labels...)
			}
			if light.BrightnessPercent != nil {
				gauge(lightBrightnessDesc, float64(light.GetBrightnessPercent()), labels...)
			}
		}
	}
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
