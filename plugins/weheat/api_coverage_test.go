package weheat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	weheatapi "github.com/joshp123/weheat-golang"
	"github.com/prometheus/client_golang/prometheus"
)

func TestVendorFieldsReachMetricsAndHistoryRPC(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected write: %s", r.Method)
		}
		switch r.URL.Path {
		case "/third_party/api/v1/energy-logs/example/total":
			w.Write([]byte(`{"totalEInIUHeating":12.5,"totalEInIUStandby":null}`))
		case "/third_party/api/v1/heat-pumps/example/logs":
			w.Write([]byte(`[{"timeBucket":"2026-11-07T00:00:00Z","dhwTargetTemperatureAverage":48.5,"coolingStopReasonSwitchOver":0,"tRoomMin":-1}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &Client{baseURL: server.URL + "/third_party"}
	totals, err := client.EnergyTotals(context.Background(), "example")
	if err != nil {
		t.Fatal(err)
	}
	if totals.TotalEInIUHeating == nil || *totals.TotalEInIUHeating != 12.5 || totals.TotalEInIUStandby != nil {
		t.Fatalf("wrong energy decode: %+v", totals)
	}
	collector := NewMetricsCollector(client)
	labels := prometheus.Labels{"heat_pump_id": "example", "serial_number": "example", "name": "example", "model": "example"}
	collector.applyEnergy(labels, totals)
	registry := prometheus.NewRegistry()
	registry.MustRegister(collector.energyMetrics["total_ein_iu_heating"], collector.energyMetrics["total_ein_iu_standby"])
	families, err := registry.Gather()
	if err != nil || len(families) != 1 || families[0].Metric[0].Gauge.GetValue() != 12.5 {
		t.Fatalf("wrong exported energy: %v %v", families, err)
	}
	stamp := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	collector.applyLog(labels, &latestLog{LastCoolingTime: &stamp})
	registry = prometheus.NewRegistry()
	registry.MustRegister(collector.logMetrics["lastCoolingTime"])
	families, err = registry.Gather()
	if err != nil || len(families) != 1 || families[0].Metric[0].Gauge.GetValue() != float64(stamp.Unix()) {
		t.Fatalf("wrong timestamp: %v %v", families, err)
	}
	views, err := client.LogViews(context.Background(), "example", weheatapi.LogQuery{Interval: weheatapi.LogInterval("Hour")})
	if err != nil || len(views) != 1 {
		t.Fatalf("history: %v %v", views, err)
	}
	var fields map[string]any
	if err := json.Unmarshal(views[0], &fields); err != nil {
		t.Fatal(err)
	}
	if fields["dhwTargetTemperatureAverage"] != 48.5 || fields["tRoomMin"] != float64(-1) {
		t.Fatalf("history fields lost: %v", fields)
	}
}
