package roborock

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsCollector collects Roborock metrics.
type MetricsCollector struct {
	client *Client

	success prometheus.Gauge

	batteryPercent     *prometheus.GaugeVec
	state              *prometheus.GaugeVec
	errorCode          *prometheus.GaugeVec
	cleaningArea       *prometheus.GaugeVec
	cleaningTime       *prometheus.GaugeVec
	totalCleaningArea  *prometheus.GaugeVec
	totalCleaningTime  *prometheus.GaugeVec
	totalCleaningCount *prometheus.GaugeVec
	fanSpeed           *prometheus.GaugeVec
	mopMode            *prometheus.GaugeVec
	mopIntensity       *prometheus.GaugeVec
	waterTankAttached  *prometheus.GaugeVec
	mopAttached        *prometheus.GaugeVec
	waterShortage      *prometheus.GaugeVec
	charging           *prometheus.GaugeVec
	lastCleanStart     *prometheus.GaugeVec
	lastCleanEnd       *prometheus.GaugeVec
}

func NewMetricsCollector(client *Client) *MetricsCollector {
	labels := []string{"device_id", "device_name", "model"}
	stateLabels := []string{"device_id", "device_name", "model", "state"}
	fanLabels := []string{"device_id", "device_name", "model", "fan_speed"}
	mopModeLabels := []string{"device_id", "device_name", "model", "mop_mode"}
	mopIntensityLabels := []string{"device_id", "device_name", "model", "mop_intensity"}
	errorLabels := []string{"device_id", "device_name", "model", "error_code"}
	return &MetricsCollector{
		client: client,
		success: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_roborock_scrape_success",
			Help: "Last scrape success (1=ok, 0=error)",
		}),
		batteryPercent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_roborock_battery_percent",
			Help: "Battery percentage (0-100)",
		}, labels),
		state: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_roborock_state",
			Help: "Vacuum state (label) reported by the device",
		}, stateLabels),
		errorCode: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_roborock_error_code",
			Help: "Vacuum error code (label)",
		}, errorLabels),
		cleaningArea: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_roborock_cleaning_area_square_meters",
			Help: "Current cleaning area (square meters)",
		}, labels),
		cleaningTime: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_roborock_cleaning_time_seconds",
			Help: "Current cleaning time (seconds)",
		}, labels),
		totalCleaningArea: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_roborock_total_cleaning_area_square_meters",
			Help: "Total cleaning area (square meters)",
		}, labels),
		totalCleaningTime: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_roborock_total_cleaning_time_seconds",
			Help: "Total cleaning time (seconds)",
		}, labels),
		totalCleaningCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_roborock_total_cleaning_count",
			Help: "Total cleaning count",
		}, labels),
		fanSpeed: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_roborock_fan_speed",
			Help: "Fan speed (label)",
		}, fanLabels),
		mopMode: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_roborock_mop_mode",
			Help: "Mop mode (label)",
		}, mopModeLabels),
		mopIntensity: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_roborock_mop_intensity",
			Help: "Mop intensity (label)",
		}, mopIntensityLabels),
		waterTankAttached: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_roborock_water_tank_attached",
			Help: "Whether the water tank is attached (1=yes, 0=no)",
		}, labels),
		mopAttached: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_roborock_mop_attached",
			Help: "Whether the mop is attached (1=yes, 0=no)",
		}, labels),
		waterShortage: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_roborock_water_shortage",
			Help: "Whether the water tank reports a shortage (1=yes, 0=no)",
		}, labels),
		charging: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_roborock_charging",
			Help: "Whether the vacuum is charging (1=yes, 0=no)",
		}, labels),
		lastCleanStart: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_roborock_last_clean_start_timestamp_seconds",
			Help: "Last clean start timestamp (seconds since epoch)",
		}, labels),
		lastCleanEnd: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_roborock_last_clean_end_timestamp_seconds",
			Help: "Last clean end timestamp (seconds since epoch)",
		}, labels),
	}
}

func (c *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.success.Describe(ch)
	c.batteryPercent.Describe(ch)
	c.state.Describe(ch)
	c.errorCode.Describe(ch)
	c.cleaningArea.Describe(ch)
	c.cleaningTime.Describe(ch)
	c.totalCleaningArea.Describe(ch)
	c.totalCleaningTime.Describe(ch)
	c.totalCleaningCount.Describe(ch)
	c.fanSpeed.Describe(ch)
	c.mopMode.Describe(ch)
	c.mopIntensity.Describe(ch)
	c.waterTankAttached.Describe(ch)
	c.mopAttached.Describe(ch)
	c.waterShortage.Describe(ch)
	c.charging.Describe(ch)
	c.lastCleanStart.Describe(ch)
	c.lastCleanEnd.Describe(ch)
}

func (c *MetricsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	states, err := c.client.DeviceStates(ctx)
	if err != nil {
		c.success.Set(0)
		c.success.Collect(ch)
		c.batteryPercent.Collect(ch)
		c.state.Collect(ch)
		c.errorCode.Collect(ch)
		c.cleaningArea.Collect(ch)
		c.cleaningTime.Collect(ch)
		c.totalCleaningArea.Collect(ch)
		c.totalCleaningTime.Collect(ch)
		c.totalCleaningCount.Collect(ch)
		c.fanSpeed.Collect(ch)
		c.mopMode.Collect(ch)
		c.mopIntensity.Collect(ch)
		c.waterTankAttached.Collect(ch)
		c.mopAttached.Collect(ch)
		c.waterShortage.Collect(ch)
		c.charging.Collect(ch)
		c.lastCleanStart.Collect(ch)
		c.lastCleanEnd.Collect(ch)
		return
	}

	c.success.Set(1)
	c.batteryPercent.Reset()
	c.state.Reset()
	c.errorCode.Reset()
	c.cleaningArea.Reset()
	c.cleaningTime.Reset()
	c.totalCleaningArea.Reset()
	c.totalCleaningTime.Reset()
	c.totalCleaningCount.Reset()
	c.fanSpeed.Reset()
	c.mopMode.Reset()
	c.mopIntensity.Reset()
	c.waterTankAttached.Reset()
	c.mopAttached.Reset()
	c.waterShortage.Reset()
	c.charging.Reset()
	c.lastCleanStart.Reset()
	c.lastCleanEnd.Reset()

	for _, state := range states {
		labels := prometheus.Labels{
			"device_id":   state.Device.ID,
			"device_name": state.Device.Name,
			"model":       state.Device.Model,
		}
		c.batteryPercent.With(labels).Set(float64(state.Status.BatteryPercent))
		c.cleaningArea.With(labels).Set(state.Status.CleaningAreaSquareMeters)
		c.cleaningTime.With(labels).Set(float64(state.Status.CleaningTimeSeconds))
		c.totalCleaningArea.With(labels).Set(state.Status.TotalCleaningAreaSquareM)
		c.totalCleaningTime.With(labels).Set(float64(state.Status.TotalCleaningTimeSeconds))
		c.totalCleaningCount.With(labels).Set(float64(state.Status.TotalCleaningCount))
		if state.Status.WaterTankAttached {
			c.waterTankAttached.With(labels).Set(1)
		} else {
			c.waterTankAttached.With(labels).Set(0)
		}
		if state.Status.MopAttached {
			c.mopAttached.With(labels).Set(1)
		} else {
			c.mopAttached.With(labels).Set(0)
		}
		if state.Status.WaterShortage {
			c.waterShortage.With(labels).Set(1)
		} else {
			c.waterShortage.With(labels).Set(0)
		}
		if state.Status.Charging {
			c.charging.With(labels).Set(1)
		} else {
			c.charging.With(labels).Set(0)
		}
		if !state.Status.LastCleanStart.IsZero() {
			c.lastCleanStart.With(labels).Set(float64(state.Status.LastCleanStart.Unix()))
		}
		if !state.Status.LastCleanEnd.IsZero() {
			c.lastCleanEnd.With(labels).Set(float64(state.Status.LastCleanEnd.Unix()))
		}

		stateLabels := prometheus.Labels{
			"device_id":   state.Device.ID,
			"device_name": state.Device.Name,
			"model":       state.Device.Model,
			"state":       state.Status.State,
		}
		if state.Status.State != "" {
			c.state.With(stateLabels).Set(1)
		}
		errorLabels := prometheus.Labels{
			"device_id":   state.Device.ID,
			"device_name": state.Device.Name,
			"model":       state.Device.Model,
			"error_code":  state.Status.ErrorCode,
		}
		if state.Status.ErrorCode != "" {
			c.errorCode.With(errorLabels).Set(1)
		}
		fanLabels := prometheus.Labels{
			"device_id":   state.Device.ID,
			"device_name": state.Device.Name,
			"model":       state.Device.Model,
			"fan_speed":   state.Status.FanSpeed,
		}
		if state.Status.FanSpeed != "" {
			c.fanSpeed.With(fanLabels).Set(1)
		}
		mopModeLabels := prometheus.Labels{
			"device_id":   state.Device.ID,
			"device_name": state.Device.Name,
			"model":       state.Device.Model,
			"mop_mode":    state.Status.MopMode,
		}
		if state.Status.MopMode != "" {
			c.mopMode.With(mopModeLabels).Set(1)
		}
		mopIntensityLabels := prometheus.Labels{
			"device_id":     state.Device.ID,
			"device_name":   state.Device.Name,
			"model":         state.Device.Model,
			"mop_intensity": state.Status.MopIntensity,
		}
		if state.Status.MopIntensity != "" {
			c.mopIntensity.With(mopIntensityLabels).Set(1)
		}
	}

	c.success.Collect(ch)
	c.batteryPercent.Collect(ch)
	c.state.Collect(ch)
	c.errorCode.Collect(ch)
	c.cleaningArea.Collect(ch)
	c.cleaningTime.Collect(ch)
	c.totalCleaningArea.Collect(ch)
	c.totalCleaningTime.Collect(ch)
	c.totalCleaningCount.Collect(ch)
	c.fanSpeed.Collect(ch)
	c.mopMode.Collect(ch)
	c.mopIntensity.Collect(ch)
	c.waterTankAttached.Collect(ch)
	c.mopAttached.Collect(ch)
	c.waterShortage.Collect(ch)
	c.charging.Collect(ch)
	c.lastCleanStart.Collect(ch)
	c.lastCleanEnd.Collect(ch)
}
