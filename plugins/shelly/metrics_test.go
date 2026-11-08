package shelly

import (
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFailedSystemReadKeepsCoverButDropsSystemValues(t *testing.T) {
	systemDown := false
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rpc/Shelly.GetInfoExt":
			w.Write([]byte(`{"device_id":"example","uptime":100,"components":[{"type":4,"cal_done":true,"state_str":"idle","cur_pos":100,"move_time_ms":17280,"move_power":168}]}`))
		case "/rpc/Sys.GetInfo":
			if systemDown {
				http.Error(w, "unavailable", 503)
				return
			}
			w.Write([]byte(`{"ram_free":200000}`))
		default:
			t.Errorf("unexpected device request: %s", r.URL.Path)
			http.Error(w, "unexpected", 400)
		}
	}))
	defer relay.Close()
	registry := prometheus.NewRegistry()
	registry.MustRegister(NewMetricsCollector(NewClient([]CoverConfig{{DeviceID: "example", Address: strings.TrimPrefix(relay.URL, "http://"), Name: "Example shutter"}})))
	first := gather(t, registry)
	if first["gohome_shelly_ram_free_bytes"] != 200000 || first["gohome_shelly_cover_calibration_travel_seconds"] != 17.28 {
		t.Fatalf("readings: %v", first)
	}
	systemDown = true
	next := gather(t, registry)
	if _, present := next["gohome_shelly_ram_free_bytes"]; present {
		t.Fatal("failed system read repeated RAM value")
	}
	if next["gohome_shelly_scrape_success"] != 0 || next["gohome_shelly_cover_position_percent"] != 100 {
		t.Fatalf("partial read: %v", next)
	}
}
