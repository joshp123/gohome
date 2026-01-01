package tado

import (
	"context"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsCollector collects zone temperature and humidity metrics.
type MetricsCollector struct {
	client *Client

	temp           *prometheus.GaugeVec
	humidity       *prometheus.GaugeVec
	setpoint       *prometheus.GaugeVec
	heatingPower   *prometheus.GaugeVec
	powerOn        *prometheus.GaugeVec
	override       *prometheus.GaugeVec
	heatingActive  *prometheus.GaugeVec
	lastUpdated    *prometheus.GaugeVec
	outsideTemp    prometheus.Gauge
	solarIntensity prometheus.Gauge
	lastSuccess    prometheus.Gauge
	success        prometheus.Gauge
}

func NewMetricsCollector(client *Client) *MetricsCollector {
	labels := []string{"zone_id", "zone_name"}
	return &MetricsCollector{
		client: client,
		temp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_tado_inside_temperature_celsius",
			Help: "Current inside temperature per zone",
		}, labels),
		humidity: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_tado_humidity_percent",
			Help: "Current humidity per zone",
		}, labels),
		setpoint: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_tado_setpoint_celsius",
			Help: "Target temperature per zone",
		}, labels),
		heatingPower: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_tado_heating_power_percent",
			Help: "Heating power demand per zone",
		}, labels),
		powerOn: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_tado_power_on_bool",
			Help: "Power setting per zone (1=on, 0=off)",
		}, labels),
		override: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_tado_override_active_bool",
			Help: "Manual override active per zone (1=manual, 0=scheduled)",
		}, labels),
		heatingActive: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_tado_heating_active_bool",
			Help: "Heating active per zone (1=on, 0=off)",
		}, labels),
		lastUpdated: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_tado_zone_last_updated_timestamp_seconds",
			Help: "Last update timestamp per zone (epoch seconds)",
		}, labels),
		outsideTemp: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_tado_outside_temperature_celsius",
			Help: "Outside temperature from Tado",
		}),
		solarIntensity: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_tado_solar_intensity_percent",
			Help: "Solar intensity from Tado weather (percent)",
		}),
		lastSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_tado_last_success_timestamp_seconds",
			Help: "Last successful Tado scrape timestamp (epoch seconds)",
		}),
		success: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_tado_scrape_success",
			Help: "Last scrape success (1=ok, 0=error)",
		}),
	}
}

func (c *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.temp.Describe(ch)
	c.humidity.Describe(ch)
	c.setpoint.Describe(ch)
	c.heatingPower.Describe(ch)
	c.powerOn.Describe(ch)
	c.override.Describe(ch)
	c.heatingActive.Describe(ch)
	c.lastUpdated.Describe(ch)
	c.outsideTemp.Describe(ch)
	c.solarIntensity.Describe(ch)
	c.lastSuccess.Describe(ch)
	c.success.Describe(ch)
}

func (c *MetricsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	zones, err := c.client.Zones(ctx)
	if err != nil {
		c.success.Set(0)
		c.collectAll(ch)
		return
	}

	states, err := c.client.ZoneStates(ctx)
	if err != nil {
		c.success.Set(0)
		c.collectAll(ch)
		return
	}

	c.temp.Reset()
	c.humidity.Reset()
	c.setpoint.Reset()
	c.heatingPower.Reset()
	c.powerOn.Reset()
	c.override.Reset()
	c.heatingActive.Reset()
	c.lastUpdated.Reset()

	if weather, err := c.client.Weather(ctx); err == nil {
		if weather.OutsideTemperatureCelsius != nil {
			c.outsideTemp.Set(*weather.OutsideTemperatureCelsius)
		}
		if weather.SolarIntensityPercent != nil {
			c.solarIntensity.Set(*weather.SolarIntensityPercent)
		}
	}

	for _, zone := range zones {
		state, ok := states[zone.ID]
		if !ok {
			continue
		}
		labels := prometheus.Labels{
			"zone_id":   strconv.Itoa(zone.ID),
			"zone_name": zone.Name,
		}
		if state.InsideTemperatureCelsius != nil {
			c.temp.With(labels).Set(*state.InsideTemperatureCelsius)
		}
		if state.HumidityPercent != nil {
			c.humidity.With(labels).Set(*state.HumidityPercent)
		}
		if state.SetpointCelsius != nil {
			c.setpoint.With(labels).Set(*state.SetpointCelsius)
		}
		if state.HeatingPowerPercent != nil {
			c.heatingPower.With(labels).Set(*state.HeatingPowerPercent)
		}
		if state.PowerOn != nil {
			c.powerOn.With(labels).Set(boolToFloat(*state.PowerOn))
		}
		if state.OverrideActive != nil {
			c.override.With(labels).Set(boolToFloat(*state.OverrideActive))
		}
		if state.InsideTemperatureTimestamp != nil {
			c.lastUpdated.With(labels).Set(float64(state.InsideTemperatureTimestamp.Unix()))
		}
		if state.HeatingPowerPercent != nil {
			c.heatingActive.With(labels).Set(boolToFloat(*state.HeatingPowerPercent > 0))
		} else if state.PowerOn != nil {
			c.heatingActive.With(labels).Set(boolToFloat(*state.PowerOn))
		}
	}

	c.success.Set(1)
	c.lastSuccess.Set(float64(time.Now().Unix()))
	c.collectAll(ch)
}

func (c *MetricsCollector) collectAll(ch chan<- prometheus.Metric) {
	c.temp.Collect(ch)
	c.humidity.Collect(ch)
	c.setpoint.Collect(ch)
	c.heatingPower.Collect(ch)
	c.powerOn.Collect(ch)
	c.override.Collect(ch)
	c.heatingActive.Collect(ch)
	c.lastUpdated.Collect(ch)
	c.outsideTemp.Collect(ch)
	c.solarIntensity.Collect(ch)
	c.lastSuccess.Collect(ch)
	c.success.Collect(ch)
}

func boolToFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
