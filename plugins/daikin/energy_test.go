package daikin

import (
	"encoding/json"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"os"
	"testing"
	"time"
)

func energyFixture(at string) []byte {
	values := func(n int) []any {
		v := make([]any, n)
		for i := range v {
			v[i] = float64(i) / 10
		}
		return v
	}
	b, _ := json.Marshal(map[string]any{"id": "unit", "timestamp": at, "managementPoints": []any{
		map[string]any{"embeddedId": "gateway", "timeZone": map[string]any{"value": "Europe/Amsterdam"}},
		map[string]any{"embeddedId": "climateControl", "managementPointType": "climateControl", "consumptionData": map[string]any{"value": map[string]any{"electrical": map[string]any{"unit": "kWh", "cooling": map[string]any{"d": values(24), "w": values(14), "m": values(24)}}}}},
	}})
	return b
}

func TestEnergyCalendarAndRawValues(t *testing.T) {
	readings, _, err := DecodeEnergyReadings(energyFixture("2026-11-08T15:25:12Z"))
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, r := range readings {
		if r.Resolution == "month" && r.Slot == 0 {
			if r.Start.Format(time.RFC3339) != "2025-01-01T00:00:00+01:00" {
				t.Fatal(r)
			}
			found++
		}
		if r.Resolution == "day" && r.Slot == 7 {
			if r.Start.Format("2006-01-02") != "2026-11-02" || r.KWh != 0.7 {
				t.Fatal(r)
			}
			found++
		}
		if r.Resolution == "2h" && r.Slot == 12 {
			if r.Start.Format(time.RFC3339) != "2026-11-08T00:00:00+01:00" {
				t.Fatal(r)
			}
			found++
		}
		if r.Start.After(time.Date(2026, 11, 8, 15, 25, 12, 0, time.UTC)) {
			t.Fatal("future bucket", r)
		}
	}
	if found != 3 {
		t.Fatal(found)
	}
}

func TestEnergyMissingIsNotZero(t *testing.T) {
	var raw map[string]any
	json.Unmarshal(energyFixture("2026-01-01T10:00:00Z"), &raw)
	points := raw["managementPoints"].([]any)
	energy := points[1].(map[string]any)["consumptionData"].(map[string]any)["value"].(map[string]any)["electrical"].(map[string]any)
	energy["cooling"].(map[string]any)["w"].([]any)[0] = nil
	b, _ := json.Marshal(raw)
	readings, _, err := DecodeEnergyReadings(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range readings {
		if r.Resolution == "day" && r.Slot == 0 {
			t.Fatal("null exported")
		}
	}
	energy["unit"] = "Wh"
	b, _ = json.Marshal(raw)
	if _, _, err = DecodeEnergyReadings(b); err == nil {
		t.Fatal("wrong unit accepted")
	}
}

func TestEnergyDSTDoesNotInventTwoHourIntervals(t *testing.T) {
	readings, _, err := DecodeEnergyReadings(energyFixture("2026-10-26T12:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range readings {
		if r.Resolution == "2h" && r.Slot < 12 {
			t.Fatal("ambiguous DST slot exported")
		}
		if r.Resolution == "day" && r.Start.Format("2006-01-02") == "2026-10-25" && r.End.Sub(r.Start) != 25*time.Hour {
			t.Fatal(r)
		}
	}
}

// An optional private response proves the decoder against hardware without
// storing household data in the repository or making another cloud request.
func TestCapturedGatewayResponse(t *testing.T) {
	path := os.Getenv("DAIKIN_CAPTURE")
	if path == "" {
		t.Skip("no private capture supplied")
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var entry deviceStateEntry
	if err = json.Unmarshal(payload, &entry); err != nil {
		t.Fatal(err)
	}
	state := entry.toDeviceState()
	readings, err := state.energyReadings()
	if err != nil {
		t.Fatal(err)
	}
	if len(readings) == 0 {
		t.Fatal("no energy readings")
	}
	ch := make(chan prometheus.Metric, 512)
	collector := NewMetricsCollector(nil)
	collector.collectDeviceReadings(ch, []DeviceState{state})
	collector.errorState.Collect(ch)
	collector.warningState.Collect(ch)
	collector.cautionState.Collect(ch)
	close(ch)
	counts := map[string]int{}
	var captured snapshotMetrics
	for metric := range ch {
		captured = append(captured, metric)
		var decoded dto.Metric
		if err := metric.Write(&decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Gauge == nil {
			t.Fatal("not a gauge")
		}
		counts[metric.Desc().String()]++
	}
	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(captured)
	if _, err := registry.Gather(); err != nil {
		t.Fatal(err)
	}
	for descriptor, count := range counts {
		t.Logf("%s samples=%d", descriptor, count)
	}
	t.Logf("decoded %d raw energy slots at %s", len(readings), state.Timestamp)
}

type snapshotMetrics []prometheus.Metric

func (s snapshotMetrics) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range s {
		ch <- metric.Desc()
	}
}
func (s snapshotMetrics) Collect(ch chan<- prometheus.Metric) {
	for _, metric := range s {
		ch <- metric
	}
}
