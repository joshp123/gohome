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

	temp     *prometheus.GaugeVec
	humidity *prometheus.GaugeVec
	success  prometheus.Gauge
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
		success: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_tado_scrape_success",
			Help: "Last scrape success (1=ok, 0=error)",
		}),
	}
}

func (c *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.temp.Describe(ch)
	c.humidity.Describe(ch)
	c.success.Describe(ch)
}

func (c *MetricsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	zones, err := c.client.Zones(ctx)
	if err != nil {
		c.success.Set(0)
		c.temp.Collect(ch)
		c.humidity.Collect(ch)
		c.success.Collect(ch)
		return
	}

	states, err := c.client.ZoneStates(ctx)
	if err != nil {
		c.success.Set(0)
		c.temp.Collect(ch)
		c.humidity.Collect(ch)
		c.success.Collect(ch)
		return
	}

	c.temp.Reset()
	c.humidity.Reset()

	for _, zone := range zones {
		state, ok := states[zone.ID]
		if !ok {
			continue
		}
		labels := prometheus.Labels{
			"zone_id":   strconv.Itoa(zone.ID),
			"zone_name": zone.Name,
		}
		c.temp.With(labels).Set(state.InsideTemperatureCelsius)
		c.humidity.With(labels).Set(state.HumidityPercent)
	}

	c.success.Set(1)
	c.temp.Collect(ch)
	c.humidity.Collect(ch)
	c.success.Collect(ch)
}
