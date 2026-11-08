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
	RamSizeDesc    = coverDesc("ram_size_bytes", "Device ram_size in bytes")
	RamFreeDesc    = coverDesc("ram_free_bytes", "Device ram_free in bytes")
	RamMinFreeDesc = coverDesc("ram_min_free_bytes", "Device ram_min_free in bytes")
	FsSizeDesc     = coverDesc("filesystem_size_bytes", "Device fs_size in bytes")
	FsFreeDesc     = coverDesc("filesystem_free_bytes", "Device fs_free in bytes")

	UptimeSecondsDesc                 = coverDesc("uptime_seconds", "Relay uptime in seconds")
	FailsafeDesc                      = coverDesc("failsafe", "Relay failsafe mode")
	OverheatDesc                      = coverDesc("overheat", "Relay overheat protection active")
	HomekitRunningDesc                = coverDesc("homekit_running", "HomeKit server running")
	HomekitPairedDesc                 = coverDesc("homekit_paired", "HomeKit paired")
	HomekitConnectionsDesc            = coverDesc("homekit_connections", "Active HomeKit connections")
	HomekitPendingConnectionsDesc     = coverDesc("homekit_pending_connections", "Pending HomeKit connections")
	WifiConnectedDesc                 = coverDesc("wifi_connected", "Wi-Fi connected")
	WifiConnectingDesc                = coverDesc("wifi_connecting", "Wi-Fi connection in progress")
	OTAProgressDesc                   = coverDesc("ota_progress_percent", "Firmware update progress; minus one means no update")
	CalibrationTravelMillisecondsDesc = coverDesc("cover_calibration_travel_seconds", "Calibrated full travel time in seconds")
	CalibrationPowerWattsDesc         = coverDesc("cover_calibration_power_watts", "Motor power measured during calibration, not live power")
	stateDesc                         = prometheus.NewDesc("gohome_shelly_cover_state", "Current firmware cover state", []string{"device_id", "name", "state"}, nil)

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
	for _, desc := range []*prometheus.Desc{RamSizeDesc, RamFreeDesc, RamMinFreeDesc, FsSizeDesc, FsFreeDesc, scrapeSuccessDesc, lastSuccessDesc, positionDesc, targetPositionDesc,
		movingDesc, calibratedDesc, deviceTemperatureDesc, wifiRssiDesc, stateDesc, UptimeSecondsDesc, FailsafeDesc, OverheatDesc, HomekitRunningDesc, HomekitPairedDesc, HomekitConnectionsDesc, HomekitPendingConnectionsDesc, WifiConnectedDesc, WifiConnectingDesc, OTAProgressDesc, CalibrationTravelMillisecondsDesc, CalibrationPowerWattsDesc} {
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
			if reading.SystemReadError == "" {
				c.lastSuccess[reading.Cover.DeviceID] = reading.ReadAt
				emit(scrapeSuccessDesc, 1)
			} else {
				emit(scrapeSuccessDesc, 0)
				log.Printf("shelly system read: %s", reading.SystemReadError)
			}
			if reading.System != nil {
				if reading.System.RamSize != nil {
					emit(RamSizeDesc, *reading.System.RamSize)
				}
				if reading.System.RamFree != nil {
					emit(RamFreeDesc, *reading.System.RamFree)
				}
				if reading.System.RamMinFree != nil {
					emit(RamMinFreeDesc, *reading.System.RamMinFree)
				}
				if reading.System.FsSize != nil {
					emit(FsSizeDesc, *reading.System.FsSize)
				}
				if reading.System.FsFree != nil {
					emit(FsFreeDesc, *reading.System.FsFree)
				}
			}

			ch <- prometheus.MustNewConstMetric(stateDesc, prometheus.GaugeValue, 1, append(labels, reading.State)...)
			if reading.UptimeSeconds != nil {
				emit(UptimeSecondsDesc, *reading.UptimeSeconds)
			}
			if reading.Failsafe != nil {
				emit(FailsafeDesc, boolValue(*reading.Failsafe))
			}
			if reading.Overheat != nil {
				emit(OverheatDesc, boolValue(*reading.Overheat))
			}
			if reading.HomekitRunning != nil {
				emit(HomekitRunningDesc, boolValue(*reading.HomekitRunning))
			}
			if reading.HomekitPaired != nil {
				emit(HomekitPairedDesc, boolValue(*reading.HomekitPaired))
			}
			if reading.HomekitConnections != nil {
				emit(HomekitConnectionsDesc, *reading.HomekitConnections)
			}
			if reading.HomekitPendingConnections != nil {
				emit(HomekitPendingConnectionsDesc, *reading.HomekitPendingConnections)
			}
			if reading.WifiConnected != nil {
				emit(WifiConnectedDesc, boolValue(*reading.WifiConnected))
			}
			if reading.WifiConnecting != nil {
				emit(WifiConnectingDesc, boolValue(*reading.WifiConnecting))
			}
			if reading.OTAProgress != nil {
				emit(OTAProgressDesc, *reading.OTAProgress)
			}
			if reading.CalibrationTravelMilliseconds != nil {
				emit(CalibrationTravelMillisecondsDesc, *reading.CalibrationTravelMilliseconds/1000)
			}
			if reading.CalibrationPowerWatts != nil {
				emit(CalibrationPowerWattsDesc, *reading.CalibrationPowerWatts)
			}

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
