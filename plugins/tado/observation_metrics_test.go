package tado

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joshp123/gohome/internal/oauth"
	"github.com/prometheus/client_golang/prometheus"
)

func TestCollectorExportsExistingResponsesWithoutExtraCalls(t *testing.T) {
	phase := "complete"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			w.Write([]byte(`{"access_token":"test","refresh_token":"test","expires_in":3600,"token_type":"Bearer"}`))
			return
		}
		calls++
		if r.Method != "GET" {
			t.Errorf("unexpected source write")
		}
		if phase == "failed" {
			w.WriteHeader(503)
			return
		}
		switch r.URL.Path {
		case "/homes/1/zones":
			if phase == "missing" {
				w.Write([]byte(`[{"id":2,"name":"Room","devices":[]}]`))
				return
			}
			w.Write([]byte(`[{"id":2,"name":"Room","devices":[{"serialNo":"example","deviceType":"VA02","connectionState":{"value":false,"timestamp":"2026-11-08T12:00:00Z"},"mountingState":{"value":"CALIBRATED","timestamp":"2026-11-08T12:00:00Z"},"childLockEnabled":false,"inPairingMode":false}]}]`))
		case "/homes/1/zoneStates":
			if phase == "active" {
				w.Write([]byte(`{"zoneStates":{"2":{"overlayType":"MANUAL","overlay":{"termination":{"type":"TIMER","remainingTimeInSeconds":120,"projectedExpiry":"2026-11-08T12:02:00Z"}},"openWindow":{"remainingTimeInSeconds":30,"detectedTime":"2026-11-08T12:00:00Z","expiry":"2026-11-08T12:00:30Z"}}}}`))
				return
			}

			if phase == "missing" {
				w.Write([]byte(`{"zoneStates":{"2":{"setting":{"power":"ON"}}}}`))
				return
			}
			w.Write([]byte(`{"zoneStates":{"2":{"tadoMode":"HOME","link":{"state":"ONLINE"},"openWindow":null,"overlayType":null,"runningOfflineSchedule":false,"geolocationOverride":false,"setting":{"power":"ON"},"activityDataPoints":{"heatingPower":{"percentage":0,"timestamp":"2026-11-08T12:00:00Z"}}}}}`))
		case "/homes/1/weather":
			if phase == "missing" {
				w.Write([]byte(`{}`))
				return
			}
			w.Write([]byte(`{"weatherState":{"value":"CLOUDY","timestamp":"2026-11-08T12:00:00Z"}}`))
		default:
			t.Errorf("unexpected new endpoint %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "state.json")
	if err := oauth.WriteState(path, oauth.State{SchemaVersion: oauth.SchemaVersion, ClientID: "test", ClientSecret: "test", RefreshToken: "test", Scope: "offline_access"}); err != nil {
		t.Fatal(err)
	}
	homeID := 1
	client, err := NewClientWithStore(Config{BaseURL: server.URL, BootstrapFile: path, HomeID: &homeID}, oauth.Declaration{Provider: "tado", TokenURL: server.URL + "/token", Scope: "offline_access", StatePath: path}, &memoryBlobStore{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client.oauth.StartWithInterval(ctx, time.Hour)
	registry := prometheus.NewRegistry()
	registry.MustRegister(NewMetricsCollector(client))
	for _, stage := range []string{"complete", "active", "missing", "failed", "complete"} {
		phase = stage
		calls = 0
		families, err := registry.Gather()
		if err != nil {
			t.Fatal(err)
		}
		values := map[string]float64{}
		for _, family := range families {
			values[family.GetName()] = family.Metric[0].Gauge.GetValue()
		}
		if stage == "complete" {
			for name, want := range map[string]float64{"zone_presence": 1, "zone_connection_state": 1, "zone_overlay_type": 1, "override_active_bool": 0, "zone_open_window_detected": 0, "device_connected": 0, "device_child_lock_enabled": 0, "weather_state": 1, "heating_power_percent": 0, "heating_active_bool": 0} {
				value, ok := values["gohome_tado_"+name]
				if !ok || value != want {
					t.Errorf("%s got %v present=%v want %v", name, value, ok, want)
				}
			}
		} else if stage == "active" {
			for name, want := range map[string]float64{"zone_open_window_detected": 1, "zone_open_window_remaining_seconds": 30, "override_active_bool": 1, "zone_overlay_remaining_seconds": 120, "zone_overlay_termination": 1} {
				if value, ok := values["gohome_tado_"+name]; !ok || value != want {
					t.Errorf("active %s got=%v exists=%v", name, value, ok)
				}
			}
		} else {
			for name := range values {
				if name != "gohome_tado_scrape_success" && name != "gohome_tado_last_success_timestamp_seconds" && name != "gohome_tado_power_on_bool" {
					t.Errorf("%s retained absent reading %s", stage, name)
				}
			}
		}
		wantCalls := 3
		if stage == "failed" {
			wantCalls = 1
		}
		if calls != wantCalls {
			t.Errorf("calls=%d want=%d", calls, wantCalls)
		}
	}
}

func TestDayReportsKeepCategoriesAndNullsHonest(t *testing.T) {
	var report dayReport
	err := json.Unmarshal([]byte(`{"measuredData":{"insideTemperature":{"dataPoints":[{"timestamp":"2026-11-07T00:00:00Z","value":null},{"timestamp":"","value":20}]},"humidity":{"dataPoints":[{"timestamp":"2026-11-07T00:00:00Z","value":{"percentage":0.5}}]}},"callForHeat":{"dataIntervals":[{"from":"2026-11-07T00:00:00Z","to":"2026-11-07T01:00:00Z","value":"LOW"}]},"weather":{"sunny":{"dataIntervals":[{"from":"2026-11-07T00:00:00Z","value":true}]}}}`), &report)
	if err != nil {
		t.Fatal(err)
	}
	samples := dayReportSamples(report, Zone{ID: 2, Name: "Room"})
	if len(samples) != 3 {
		t.Fatalf("got %d samples: %+v", len(samples), samples)
	}
	for _, s := range samples {
		if !strings.HasPrefix(s.Name, "gohome_tado_history_") {
			t.Fatal("history in live series")
		}
		switch s.Name {
		case "gohome_tado_history_humidity_percent":
			if s.Value != 0.5 {
				t.Fatal("humidity guessed from magnitude")
			}
		case "gohome_tado_history_call_for_heat_interval_seconds":
			if s.Value != 3600 || s.Labels["level"] != "LOW" {
				t.Fatal("category converted to power")
			}
		case "gohome_tado_history_sunny":
			if s.Value != 1 {
				t.Fatal("sunny converted to solar intensity")
			}
		}
	}
}

func TestDayReportHumidityUsesDeclaredUnit(t *testing.T) {
	var report dayReport
	err := json.Unmarshal([]byte(`{"measuredData":{"humidity":{"percentageUnit":"UNIT_INTERVAL","dataPoints":[{"timestamp":"2026-11-07T00:00:00Z","value":0.465},{"timestamp":"2026-11-07T00:15:00Z","value":0.005},{"timestamp":"2026-11-07T00:30:00Z","value":null},{"timestamp":"","value":0.5}]}}}`), &report)
	if err != nil {
		t.Fatal(err)
	}
	samples := dayReportSamples(report, Zone{ID: 2, Name: "Room"})
	if len(samples) != 2 {
		t.Fatalf("got %d samples: %+v", len(samples), samples)
	}
	for i, want := range []float64{46.5, 0.5} {
		if samples[i].Name != "gohome_tado_history_humidity_percent" || samples[i].Value != want {
			t.Fatalf("sample %d: got %+v, want humidity %g%%", i, samples[i], want)
		}
	}
}
