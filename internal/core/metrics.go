package core

import (
	"math"

	"github.com/joshp123/gohome/internal/oauth"
	"github.com/joshp123/gohome/internal/rate"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// MetricsRegistry builds a registry from plugin collectors.
func MetricsRegistry(plugins []Plugin) *prometheus.Registry {
	registry := prometheus.NewRegistry()

	for _, collector := range oauth.MetricsCollectors() {
		registry.MustRegister(collector)
	}
	for _, collector := range rate.MetricsCollectors() {
		registry.MustRegister(collector)
	}

	for _, plugin := range plugins {
		for _, collector := range plugin.Collectors() {
			registry.MustRegister(collector)
		}
	}

	return registry
}

// CollectPresent omits optional scalar readings represented internally by NaN.
func CollectPresent(collector prometheus.Collector, ch chan<- prometheus.Metric) {
	if gauge, ok := collector.(prometheus.Gauge); ok {
		var metric dto.Metric
		if gauge.Write(&metric) != nil || math.IsNaN(metric.GetGauge().GetValue()) {
			return
		}
	}
	collector.Collect(ch)
}
