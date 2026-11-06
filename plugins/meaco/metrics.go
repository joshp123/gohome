package meaco

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"log/slog"
)

type MetricsCollector struct {
	client       *Client
	descriptions map[string]*prometheus.Desc
}

func NewMetricsCollector(client *Client) *MetricsCollector {
	descriptions := map[string]*prometheus.Desc{}
	for name, help := range map[string]string{
		"humidity_percent":               "Measured relative humidity (%)",
		"target_humidity_percent":        "Target relative humidity (%)",
		"power":                          "Reported power switch (not compressor activity)",
		"child_lock":                     "Reported child lock",
		"fault_code":                     "Raw device status bitmap (not all bits indicate faults)",
		"last_success_timestamp_seconds": "Time this status was read successfully",
	} {
		descriptions[name] = prometheus.NewDesc("gohome_meaco_"+name, help, nil, nil)
	}
	for _, name := range []string{"mode", "countdown", "on_timer"} {
		descriptions[name] = prometheus.NewDesc("gohome_meaco_"+name, "Reported "+name, []string{"value"}, nil)
	}
	return &MetricsCollector{client, descriptions}
}
func (c *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range c.descriptions {
		ch <- desc
	}
}
func (c *MetricsCollector) Collect(ch chan<- prometheus.Metric) {
	state, err := c.client.Status(context.Background())
	if err != nil {
		slog.Warn("meaco status unavailable", "error", err)
		return
	}
	emit := func(name string, value float64, labels ...string) {
		ch <- prometheus.MustNewConstMetric(c.descriptions[name], prometheus.GaugeValue, value, labels...)
	}
	if state.HumidityPercent != nil {
		emit("humidity_percent", float64(*state.HumidityPercent))
	}
	if state.TargetHumidityPercent != nil {
		emit("target_humidity_percent", float64(*state.TargetHumidityPercent))
	}
	if state.FaultCode != nil {
		emit("fault_code", float64(*state.FaultCode))
	}
	for name, value := range map[string]*bool{"power": state.Power, "child_lock": state.ChildLock} {
		if value != nil {
			number := 0.0
			if *value {
				number = 1
			}
			emit(name, number)
		}
	}
	for name, value := range map[string]*string{"mode": state.Mode, "countdown": state.Countdown, "on_timer": state.OnTimer} {
		if value != nil {
			emit(name, 1, *value)
		}
	}
	emit("last_success_timestamp_seconds", float64(state.ObservedAtUnix))
}
