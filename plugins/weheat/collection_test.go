package weheat

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/joshp123/gohome/internal/core"
	weheatapi "github.com/joshp123/weheat-golang"
	"github.com/prometheus/client_golang/prometheus"
)

func TestScrapeDropsUnavailableReadings(t *testing.T) {
	phase := "complete"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/third_party/api/v1/") {
			t.Errorf("wrong API path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/heat-pumps"):
			if phase == "discovery failure" {
				http.Error(w, "unavailable", 503)
				return
			}
			fmt.Fprint(w, `{"data":[{"id":"test-pump"}]}`)
		case strings.HasSuffix(r.URL.Path, "/logs/latest"):
			if phase == "read failure" {
				http.Error(w, "unavailable", 503)
				return
			}
			timestamp := time.Now()
			if phase == "stale" {
				timestamp = timestamp.Add(-time.Hour)
			}
			fields := `,"cmMassPowerIn":442`
			if phase == "missing" {
				fields = ""
			}
			fmt.Fprintf(w, `{"timestamp":%q,"tRoom":20.9%s}`, timestamp.UTC().Format(time.RFC3339Nano), fields)
		case strings.HasSuffix(r.URL.Path, "/total"):
			fmt.Fprint(w, `{"totalEInHeating":10}`)
		default:
			fmt.Fprintf(w, `[{"timeBucket":%q,"averagePowerEInHeating":0.4}]`, time.Now().UTC().Format(time.RFC3339Nano))
		}
	}))
	defer server.Close()
	api, err := weheatapi.NewClient(weheatapi.WithBaseURL(server.URL+"/third_party"), weheatapi.WithHTTPClient(&http.Client{Transport: apiPathTransport{prefix: "/third_party"}}))
	if err != nil {
		t.Fatal(err)
	}
	c := NewMetricsCollector(&Client{api: api, baseURL: server.URL + "/third_party"})
	plugin := Plugin{client: c.client, collector: c}
	if plugin.Health() != core.HealthError {
		t.Fatal("startup reports healthy before first read")
	}
	registry := prometheus.NewRegistry()
	registry.MustRegister(c)
	gather := func() map[string]float64 {
		t.Helper()
		families, err := registry.Gather()
		if err != nil {
			t.Fatal(err)
		}
		values := map[string]float64{}
		for _, f := range families {
			for _, m := range f.Metric {
				values[f.GetName()] = m.GetGauge().GetValue()
			}
		}
		return values
	}
	initial := gather()
	if initial["gohome_weheat_log_cm_mass_power_in"] != 442 {
		t.Fatal("initial power missing")
	}
	if _, ok := initial["gohome_weheat_energy_log_total_ein_heating"]; ok {
		t.Fatal("missing energy field exported as zero")
	}
	if _, ok := initial["gohome_weheat_log_interval"]; ok {
		t.Fatal("missing interval exported as zero")
	}
	if plugin.Health() != core.HealthHealthy {
		t.Fatal(plugin.HealthMessage())
	}
	readAt := c.lastRead
	gather()
	if !c.lastRead.Equal(readAt) {
		t.Fatal("cache hit advanced successful read time")
	}
	for _, next := range []string{"missing", "read failure", "stale", "discovery failure"} {
		phase = next
		c.cachedAt = time.Time{}
		c.logFetchedAt["test-pump"] = time.Time{}
		values := gather()
		if _, ok := values["gohome_weheat_log_cm_mass_power_in"]; ok {
			t.Fatalf("%s retained power", phase)
		}
		if phase != "missing" {
			if plugin.Health() != core.HealthError {
				t.Fatal("failed read reports healthy")
			}
			if _, ok := values["gohome_weheat_log_t_room"]; ok {
				t.Fatalf("%s retained temperature", phase)
			}
			if values["gohome_weheat_scrape_success"] != 0 {
				t.Fatal("failed read reports success")
			}
		}
		// A scrape during the retry interval must not revive an earlier observation.
		values = gather()
		if _, ok := values["gohome_weheat_log_cm_mass_power_in"]; ok {
			t.Fatalf("%s revived cached power", phase)
		}
	}
}
