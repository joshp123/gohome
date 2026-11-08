package shelly

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var coverLabels = []string{"device_id", "name"}

func coverDesc(name, help string) *prometheus.Desc {
	return prometheus.NewDesc("gohome_shelly_"+name, help, coverLabels, nil)
}

var (
	scrapeSuccessDesc     = coverDesc("scrape_success", "Last read of this relay succeeded (1) or failed (0)")
	lastSuccessDesc       = coverDesc("last_success_timestamp_seconds", "When this relay was last read successfully (epoch seconds)")
	positionDesc          = coverDesc("cover_position_percent", "Shutter position estimated by the relay: 0 closed, 100 open")
	targetPositionDesc    = coverDesc("cover_target_position_percent", "Position the shutter is heading to: 0 closed, 100 open")
	movingDesc            = coverDesc("cover_moving", "The relay is running the shutter motor (1) or not (0)")
	calibratedDesc        = coverDesc("cover_calibrated", "The relay has measured the shutter's full travel (1) or not (0)")
	deviceTemperatureDesc = coverDesc("device_temperature_celsius", "Relay chip temperature (celsius)")
	wifiRssiDesc          = coverDesc("wifi_rssi_dbm", "Relay Wi-Fi signal strength (dBm)")
)

// MetricsCollector reads every relay at scrape time. A failed read emits only
// scrape_success=0 and the last success time: never the previous state.
type MetricsCollector struct {
	client *Client

	mu          sync.Mutex
	lastSuccess map[string]time.Time // by device ID
}

func NewMetricsCollector(client *Client) *MetricsCollector {
	return &MetricsCollector{client: client, lastSuccess: map[string]time.Time{}}
}

func (c *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range []*prometheus.Desc{scrapeSuccessDesc, lastSuccessDesc, positionDesc, targetPositionDesc,
		movingDesc, calibratedDesc, deviceTemperatureDesc, wifiRssiDesc} {
		ch <- desc
	}
}

func (c *MetricsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c.mu.Lock()
	defer c.mu.Unlock()
	for _, reading := range c.client.ReadAll(ctx) {
		labels := []string{reading.Cover.DeviceID, reading.Cover.Name}
		emit := func(desc *prometheus.Desc, value float64) {
			ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, value, labels...)
		}
		if reading.Err != nil {
			log.Printf("shelly %s (%s) read failed: %v", reading.Cover.Name, reading.Cover.DeviceID, reading.Err)
			emit(scrapeSuccessDesc, 0)
		} else {
			c.lastSuccess[reading.Cover.DeviceID] = reading.ReadAt
			emit(scrapeSuccessDesc, 1)
			if reading.PositionPercent != nil {
				emit(positionDesc, float64(*reading.PositionPercent))
			}
			if reading.TargetPositionPercent != nil {
				emit(targetPositionDesc, float64(*reading.TargetPositionPercent))
			}
			emit(movingDesc, boolValue(reading.MotorRunning))
			emit(calibratedDesc, boolValue(reading.Calibrated))
			if reading.DeviceTemperatureCelsius != nil {
				emit(deviceTemperatureDesc, *reading.DeviceTemperatureCelsius)
			}
			if reading.WifiRssiDbm != nil {
				emit(wifiRssiDesc, *reading.WifiRssiDbm)
			}
		}
		if last, ok := c.lastSuccess[reading.Cover.DeviceID]; ok {
			emit(lastSuccessDesc, float64(last.Unix()))
		}
	}
}

func boolValue(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
