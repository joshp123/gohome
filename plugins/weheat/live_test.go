package weheat

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	weheatapi "github.com/joshp123/weheat-golang"
	"github.com/prometheus/client_golang/prometheus"
)

type liveToken string

func (t liveToken) Token(context.Context) (string, error) { return string(t), nil }

func TestLiveReadOnly(t *testing.T) {
	path := os.Getenv("WEHEAT_TEST_TOKEN_FILE")
	if path == "" {
		t.Skip("set WEHEAT_TEST_TOKEN_FILE to run the live read-only probe")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err = json.Unmarshal(data, &token); err != nil {
		t.Fatal(err)
	}
	api, err := weheatapi.NewClient(weheatapi.WithBaseURL(defaultBaseURL), weheatapi.WithTokenSource(liveToken(token.AccessToken)), weheatapi.WithHTTPClient(&http.Client{Transport: apiPathTransport{prefix: "/third_party"}}))
	if err != nil {
		t.Fatal(err)
	}
	c := NewMetricsCollector(&Client{api: api, baseURL: defaultBaseURL, tokenSource: liveToken(token.AccessToken)})
	registry := prometheus.NewRegistry()
	registry.MustRegister(c)
	metrics, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]float64{}
	for _, family := range metrics {
		for _, metric := range family.Metric {
			if metric.Gauge != nil {
				found[family.GetName()] = metric.Gauge.GetValue()
			}
		}
	}
	for _, name := range []string{"gohome_weheat_scrape_success", "gohome_weheat_energy_scrape_success", "gohome_weheat_energy_log_scrape_success"} {
		if found[name] != 1 {
			t.Fatalf("%s=%v; %s", name, found[name], c.readError)
		}
	}
	for _, name := range []string{"gohome_weheat_log_t_room", "gohome_weheat_log_t_room_target", "gohome_weheat_log_cm_mass_power_in", "gohome_weheat_log_cm_mass_power_out", "gohome_weheat_log_rpm"} {
		v, ok := found[name]
		if !ok {
			t.Fatalf("missing %s", name)
		}
		t.Logf("%s=%g", name, v)
	}
	if time.Since(c.lastRead) > time.Minute {
		t.Fatal("read not recent")
	}
}
