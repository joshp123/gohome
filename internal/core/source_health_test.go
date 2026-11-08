package core

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// Before a plugin's first successful read its last-success gauge is still 0. Exporting that
// would tell dashboards and agents the source last answered in 1970.
func TestUnsetLastSuccessIsNotExported(t *testing.T) {
	lastSuccess := prometheus.NewGauge(prometheus.GaugeOpts{Name: "gohome_example_last_success_timestamp_seconds", Help: "h"})
	scrape := prometheus.NewGauge(prometheus.GaugeOpts{Name: "gohome_example_scrape_success", Help: "h"})
	collector := &observedCollector{Collector: gauges{lastSuccess, scrape}}

	if n := lastSuccessSamples(collector); n != 0 {
		t.Fatalf("exported %d last-success samples before any success, want 0", n)
	}
	lastSuccess.Set(1794133921)
	if n := lastSuccessSamples(collector); n != 1 {
		t.Fatalf("exported %d last-success samples after a success, want 1", n)
	}
}

type gauges []prometheus.Gauge

func (g gauges) Describe(ch chan<- *prometheus.Desc) {
	for _, gauge := range g {
		gauge.Describe(ch)
	}
}

func (g gauges) Collect(ch chan<- prometheus.Metric) {
	for _, gauge := range g {
		gauge.Collect(ch)
	}
}

func lastSuccessSamples(c prometheus.Collector) int {
	ch := make(chan prometheus.Metric)
	go func() { c.Collect(ch); close(ch) }()
	n := 0
	for m := range ch {
		if strings.Contains(m.Desc().String(), "_last_success_timestamp_seconds") {
			n++
		}
	}
	return n
}
