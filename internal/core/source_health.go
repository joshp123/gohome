package core

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// ObservePlugins retains one set of collectors for both metrics and registry health.
// Health uses the collectors' scrape-success and last-success metrics, not construction.
func ObservePlugins(plugins []Plugin) []Plugin {
	result := make([]Plugin, 0, len(plugins))
	for _, p := range plugins {
		observed := &observedPlugin{Plugin: p}
		for _, c := range p.Collectors() {
			observed.collectors = append(observed.collectors, &observedCollector{Collector: c})
		}
		result = append(result, observed)
	}
	return result
}

type observedPlugin struct {
	Plugin
	collectors []*observedCollector
}

func (p *observedPlugin) Collectors() []prometheus.Collector {
	result := make([]prometheus.Collector, len(p.collectors))
	for i, c := range p.collectors {
		result[i] = c
	}
	return result
}
func (p *observedPlugin) RegisterHTTP(mux *http.ServeMux) {
	if registrant, ok := p.Plugin.(HTTPRegistrant); ok {
		registrant.RegisterHTTP(mux)
	}
}
func (p *observedPlugin) SourceHealth() (HealthStatus, string, time.Time) {
	if p.Plugin.Health() == HealthError {
		return HealthError, p.Plugin.HealthMessage(), time.Time{}
	}
	if len(p.collectors) == 0 {
		if len(p.Manifest().Services) == 0 {
			return HealthHealthy, "No data source (dashboard only)", time.Time{}
		}
		return HealthDegraded, "No source read available", time.Time{}
	}
	status, message := HealthHealthy, "Source read successfully"
	var lastGood time.Time
	for _, c := range p.collectors {
		c.mu.Lock()
		good, checked, success := c.lastGood, c.checked, c.success
		c.mu.Unlock()
		if lastGood.IsZero() || good.Before(lastGood) {
			lastGood = good
		}
		if checked.IsZero() {
			status, message = HealthDegraded, "Awaiting first source read"
		} else if !success || time.Since(checked) > time.Minute || good.IsZero() || time.Since(good) > 15*time.Minute {
			status, message = HealthDegraded, "Source read failed, missing, or overdue"
		}
	}
	return status, message, lastGood
}

type observedCollector struct {
	prometheus.Collector
	collectMu         sync.Mutex
	mu                sync.Mutex
	checked, lastGood time.Time
	success           bool
}

func (c *observedCollector) Collect(ch chan<- prometheus.Metric) {
	c.collectMu.Lock()
	defer c.collectMu.Unlock()
	metrics := make(chan prometheus.Metric)
	go func() { c.Collector.Collect(metrics); close(metrics) }()
	success, found := true, false
	timestampFound := false
	var lastGood time.Time
	for metric := range metrics {
		var value dto.Metric
		if metric.Write(&value) == nil {
			desc := metric.Desc().String()
			if strings.Contains(desc, "_scrape_success\"") {
				found = true
				success = success && value.GetGauge().GetValue() == 1
			}
			if strings.Contains(desc, "_last_success_timestamp_seconds\"") {
				timestampFound = true
				if value.GetGauge().GetValue() <= 0 {
					// No successful read since gohome started: the time is unknown, not 1970.
					continue
				}
				timestamp := time.Unix(int64(value.GetGauge().GetValue()), 0)
				if lastGood.IsZero() || timestamp.Before(lastGood) {
					lastGood = timestamp
				}
			}
		}
		ch <- metric
	}
	c.mu.Lock()
	c.checked, c.success = time.Now(), found && success
	if !lastGood.IsZero() {
		c.lastGood = lastGood
	} else if !timestampFound && found && success {
		c.lastGood = c.checked
	}
	c.mu.Unlock()
}
