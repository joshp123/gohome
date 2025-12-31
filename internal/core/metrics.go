package core

import "github.com/prometheus/client_golang/prometheus"

// MetricsRegistry builds a registry from plugin collectors.
func MetricsRegistry(plugins []Plugin) *prometheus.Registry {
	registry := prometheus.NewRegistry()

	for _, plugin := range plugins {
		for _, collector := range plugin.Collectors() {
			registry.MustRegister(collector)
		}
	}

	return registry
}
