package rate

import "github.com/prometheus/client_golang/prometheus"

var (
	remainingGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gohome_rate_limit_remaining",
			Help: "Remaining requests for the provider rate-limit window",
		},
		[]string{"provider", "window"},
	)
	retryAfterGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gohome_rate_limit_retry_after_seconds",
			Help: "Retry-after seconds for provider rate limits",
		},
		[]string{"provider"},
	)
	lastStatusGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gohome_rate_limit_last_status_code",
			Help: "Last HTTP status code observed by the rate-limit wrapper",
		},
		[]string{"provider"},
	)
)

// MetricsCollectors exposes shared rate-limit collectors.
func MetricsCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		remainingGauge,
		retryAfterGauge,
		lastStatusGauge,
	}
}
