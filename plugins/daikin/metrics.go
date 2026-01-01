package daikin

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsCollector collects Daikin unit health metrics.
type MetricsCollector struct {
	client *Client

	cloudUp *prometheus.GaugeVec
	success prometheus.Gauge

	onOffMode       *prometheus.GaugeVec
	operationMode   *prometheus.GaugeVec
	roomTemp        *prometheus.GaugeVec
	outdoorTemp     *prometheus.GaugeVec
	roomHumidity    *prometheus.GaugeVec
	setpoint        *prometheus.GaugeVec
	errorState      *prometheus.GaugeVec
	warningState    *prometheus.GaugeVec
	cautionState    *prometheus.GaugeVec
	holidayMode     *prometheus.GaugeVec
	rateLimitLimit  *prometheus.GaugeVec
	rateLimitRemain *prometheus.GaugeVec
	rateRetryAfter  prometheus.Gauge
	rateResetAfter  prometheus.Gauge
	rateLastStatus  prometheus.Gauge
}

func NewMetricsCollector(client *Client) *MetricsCollector {
	labels := []string{"unit_id", "unit_name"}
	modeLabels := []string{"unit_id", "unit_name", "embedded_id", "mode"}
	embeddedLabels := []string{"unit_id", "unit_name", "embedded_id"}
	setpointLabels := []string{"unit_id", "unit_name", "embedded_id", "operation_mode", "setpoint"}
	return &MetricsCollector{
		client: client,
		cloudUp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_daikin_cloud_connected",
			Help: "Whether the unit reports cloud connectivity (1=up, 0=down)",
		}, labels),
		success: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_daikin_scrape_success",
			Help: "Last scrape success (1=ok, 0=error)",
		}),
		onOffMode: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_daikin_on_off",
			Help: "On/off mode for the management point (1=on, 0=off)",
		}, embeddedLabels),
		operationMode: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_daikin_operation_mode",
			Help: "Operation mode reported by the management point (1=active)",
		}, modeLabels),
		roomTemp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_daikin_room_temperature_celsius",
			Help: "Reported room temperature (celsius)",
		}, embeddedLabels),
		outdoorTemp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_daikin_outdoor_temperature_celsius",
			Help: "Reported outdoor temperature (celsius)",
		}, embeddedLabels),
		roomHumidity: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_daikin_room_humidity_percent",
			Help: "Reported room humidity (%)",
		}, embeddedLabels),
		setpoint: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_daikin_setpoint_celsius",
			Help: "Setpoint temperature for each operation mode (celsius)",
		}, setpointLabels),
		errorState: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_daikin_error_state",
			Help: "Whether the unit reports error state (1=error, 0=ok)",
		}, embeddedLabels),
		warningState: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_daikin_warning_state",
			Help: "Whether the unit reports warning state (1=warning, 0=ok)",
		}, embeddedLabels),
		cautionState: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_daikin_caution_state",
			Help: "Whether the unit reports caution state (1=caution, 0=ok)",
		}, embeddedLabels),
		holidayMode: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_daikin_holiday_mode_active",
			Help: "Whether holiday mode is active (1=active, 0=inactive)",
		}, embeddedLabels),
		rateLimitLimit: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_daikin_rate_limit",
			Help: "Daikin rate limit ceilings",
		}, []string{"window"}),
		rateLimitRemain: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_daikin_rate_limit_remaining",
			Help: "Daikin remaining requests for the window",
		}, []string{"window"}),
		rateRetryAfter: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_daikin_rate_limit_retry_after_seconds",
			Help: "Retry-after seconds returned by the Daikin API",
		}),
		rateResetAfter: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_daikin_rate_limit_reset_after_seconds",
			Help: "Rate limit reset time (seconds) returned by the Daikin API",
		}),
		rateLastStatus: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_daikin_last_status_code",
			Help: "Last HTTP status code returned by the Daikin API",
		}),
	}
}

func (c *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.cloudUp.Describe(ch)
	c.success.Describe(ch)
	c.onOffMode.Describe(ch)
	c.operationMode.Describe(ch)
	c.roomTemp.Describe(ch)
	c.outdoorTemp.Describe(ch)
	c.roomHumidity.Describe(ch)
	c.setpoint.Describe(ch)
	c.errorState.Describe(ch)
	c.warningState.Describe(ch)
	c.cautionState.Describe(ch)
	c.holidayMode.Describe(ch)
	c.rateLimitLimit.Describe(ch)
	c.rateLimitRemain.Describe(ch)
	c.rateRetryAfter.Describe(ch)
	c.rateResetAfter.Describe(ch)
	c.rateLastStatus.Describe(ch)
}

func (c *MetricsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	states, err := c.client.DeviceStates(ctx)
	if err != nil {
		c.success.Set(0)
		limits := c.client.RateLimits()
		c.rateLimitLimit.WithLabelValues("minute").Set(float64(limits.Minute))
		c.rateLimitLimit.WithLabelValues("day").Set(float64(limits.Day))
		c.rateLimitRemain.WithLabelValues("minute").Set(float64(limits.RemainingMinute))
		c.rateLimitRemain.WithLabelValues("day").Set(float64(limits.RemainingDay))
		c.rateRetryAfter.Set(float64(limits.RetryAfter))
		c.rateResetAfter.Set(float64(limits.ResetAfter))
		c.rateLastStatus.Set(float64(limits.LastStatusCode))
		c.cloudUp.Collect(ch)
		c.onOffMode.Collect(ch)
		c.operationMode.Collect(ch)
		c.roomTemp.Collect(ch)
		c.outdoorTemp.Collect(ch)
		c.roomHumidity.Collect(ch)
		c.setpoint.Collect(ch)
		c.errorState.Collect(ch)
		c.warningState.Collect(ch)
		c.cautionState.Collect(ch)
		c.holidayMode.Collect(ch)
		c.rateLimitLimit.Collect(ch)
		c.rateLimitRemain.Collect(ch)
		c.rateRetryAfter.Collect(ch)
		c.rateResetAfter.Collect(ch)
		c.rateLastStatus.Collect(ch)
		c.success.Collect(ch)
		return
	}

	c.cloudUp.Reset()
	c.onOffMode.Reset()
	c.operationMode.Reset()
	c.roomTemp.Reset()
	c.outdoorTemp.Reset()
	c.roomHumidity.Reset()
	c.setpoint.Reset()
	c.errorState.Reset()
	c.warningState.Reset()
	c.cautionState.Reset()
	c.holidayMode.Reset()

	for _, state := range states {
		device := state.Device
		labels := prometheus.Labels{
			"unit_id":   device.ID,
			"unit_name": device.Name,
		}
		if device.CloudConnected {
			c.cloudUp.With(labels).Set(1)
		} else {
			c.cloudUp.With(labels).Set(0)
		}

		for _, mp := range state.ManagementPoints {
			if mp.ManagementPointType != "climateControl" && mp.ManagementPointType != "climateControlMainZone" && mp.ManagementPointType != "domesticHotWaterTank" {
				continue
			}

			mpLabels := prometheus.Labels{
				"unit_id":     device.ID,
				"unit_name":   device.Name,
				"embedded_id": mp.EmbeddedID,
			}

			if mp.OnOffMode != nil {
				value := 0.0
				if mp.OnOffMode.Value == "on" {
					value = 1
				}
				c.onOffMode.With(mpLabels).Set(value)
			}

			if mp.OperationMode != nil {
				modeLabels := prometheus.Labels{
					"unit_id":     device.ID,
					"unit_name":   device.Name,
					"embedded_id": mp.EmbeddedID,
					"mode":        mp.OperationMode.Value,
				}
				c.operationMode.With(modeLabels).Set(1)
			}

			if mp.SensoryData != nil {
				if measurement, ok := mp.SensoryData.Value["roomTemperature"]; ok {
					c.roomTemp.With(mpLabels).Set(measurement.Value)
				}
				if measurement, ok := mp.SensoryData.Value["outdoorTemperature"]; ok {
					c.outdoorTemp.With(mpLabels).Set(measurement.Value)
				}
				if measurement, ok := mp.SensoryData.Value["roomHumidity"]; ok {
					c.roomHumidity.With(mpLabels).Set(measurement.Value)
				}
			}

			if mp.TemperatureControl != nil {
				for opMode, opData := range mp.TemperatureControl.Value.OperationModes {
					for setpointName, setpoint := range opData.Setpoints {
						setpointLabels := prometheus.Labels{
							"unit_id":        device.ID,
							"unit_name":      device.Name,
							"embedded_id":    mp.EmbeddedID,
							"operation_mode": opMode,
							"setpoint":       setpointName,
						}
						c.setpoint.With(setpointLabels).Set(setpoint.Value)
					}
				}
			}

			if mp.IsInErrorState != nil {
				c.errorState.With(mpLabels).Set(boolToFloat(mp.IsInErrorState.Value))
			}
			if mp.IsInWarningState != nil {
				c.warningState.With(mpLabels).Set(boolToFloat(mp.IsInWarningState.Value))
			}
			if mp.IsInCautionState != nil {
				c.cautionState.With(mpLabels).Set(boolToFloat(mp.IsInCautionState.Value))
			}
			if mp.IsHolidayModeActive != nil {
				c.holidayMode.With(mpLabels).Set(boolToFloat(mp.IsHolidayModeActive.Value))
			}
		}
	}

	c.success.Set(1)
	limits := c.client.RateLimits()
	c.rateLimitLimit.WithLabelValues("minute").Set(float64(limits.Minute))
	c.rateLimitLimit.WithLabelValues("day").Set(float64(limits.Day))
	c.rateLimitRemain.WithLabelValues("minute").Set(float64(limits.RemainingMinute))
	c.rateLimitRemain.WithLabelValues("day").Set(float64(limits.RemainingDay))
	c.rateRetryAfter.Set(float64(limits.RetryAfter))
	c.rateResetAfter.Set(float64(limits.ResetAfter))
	c.rateLastStatus.Set(float64(limits.LastStatusCode))
	c.cloudUp.Collect(ch)
	c.onOffMode.Collect(ch)
	c.operationMode.Collect(ch)
	c.roomTemp.Collect(ch)
	c.outdoorTemp.Collect(ch)
	c.roomHumidity.Collect(ch)
	c.setpoint.Collect(ch)
	c.errorState.Collect(ch)
	c.warningState.Collect(ch)
	c.cautionState.Collect(ch)
	c.holidayMode.Collect(ch)
	c.rateLimitLimit.Collect(ch)
	c.rateLimitRemain.Collect(ch)
	c.rateRetryAfter.Collect(ch)
	c.rateResetAfter.Collect(ch)
	c.rateLastStatus.Collect(ch)
	c.success.Collect(ch)
}

func boolToFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
