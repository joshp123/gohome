package oauth

import "github.com/prometheus/client_golang/prometheus"

var (
	refreshSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gohome_oauth_refresh_success_total",
			Help: "Successful OAuth refreshes",
		},
		[]string{"provider"},
	)
	refreshFailure = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gohome_oauth_refresh_failure_total",
			Help: "Failed OAuth refreshes",
		},
		[]string{"provider"},
	)
	tokenValid = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gohome_oauth_token_valid",
			Help: "OAuth access token validity (1=valid, 0=invalid)",
		},
		[]string{"provider"},
	)
	remotePersistOK = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gohome_oauth_remote_persist_ok",
			Help: "Remote blob persistence health (1=ok, 0=error)",
		},
		[]string{"provider"},
	)
	scopeMismatch = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gohome_oauth_scope_mismatch_total",
			Help: "Scope mismatches between declaration and state",
		},
		[]string{"provider"},
	)
)

// MetricsCollectors returns collectors for the shared OAuth module.
func MetricsCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		refreshSuccess,
		refreshFailure,
		tokenValid,
		remotePersistOK,
		scopeMismatch,
	}
}
