package growatt

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	v1 "github.com/joshp123/gohome/proto/gen/plugins/growatt/v1"
	"github.com/prometheus/client_golang/prometheus"
)

func TestCapturedMINResponse(t *testing.T) {
	payload, err := os.ReadFile("testdata/min.json")
	if err != nil {
		t.Fatal(err)
	}
	var row map[string]any
	if err := json.Unmarshal(payload, &row); err != nil {
		t.Fatal(err)
	}
	telemetry, err := decodeInverterTelemetry(row, "INVERTER", "GMT+1")
	if err != nil {
		t.Fatal(err)
	}
	if telemetry.ObservedAt != "2026-11-08T16:30:28+01:00" {
		t.Fatal(telemetry.ObservedAt)
	}
	values := map[string]float64{}
	for _, reading := range telemetry.Measurements {
		values[reading.Quantity.String()+"/"+reading.Channel] = reading.Value
	}
	for key, want := range map[string]float64{"GRID_VOLTAGE_VOLTS/1": 223.8, "GRID_FREQUENCY_HERTZ/": 49.97, "STRING_VOLTAGE_VOLTS/1": 221.7, "STRING_CURRENT_AMPERES/2": 0, "TEMPERATURE_CELSIUS/5": 34.2, "FAULT_WORD/0": 0} {
		if got, ok := values[key]; !ok || got != want {
			t.Fatalf("%s = %v, present %v; want %v", key, got, ok, want)
		}
	}
	for _, key := range []string{"GRID_VOLTAGE_VOLTS/2", "STRING_POWER_WATTS/3", "TEMPERATURE_CELSIUS/3"} {
		if _, ok := values[key]; ok {
			t.Fatalf("placeholder exported: %s", key)
		}
	}
	// Run the real collector against the captured response without another cloud call.
	now := time.Now().Truncate(time.Second)
	telemetry.ObservedAt = now.Format(time.RFC3339)
	collector := NewMetricsCollector(nil)
	collector.cached = &cachedSnapshot{plant: Plant{ID: 42, Name: "Plant"}, energy: PlantEnergy{Inverter: telemetry, LastUpdate: &now}, success: true, fetchedAt: now}
	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, family := range families {
		if family.GetName() == "gohome_growatt_inverter_grid_voltage_volts" {
			found = true
			m := family.Metric[0]
			if m.GetGauge().GetValue() != 223.8 || m.GetTimestampMs() != now.UnixMilli() {
				t.Fatal(m)
			}
		}
	}
	if !found {
		t.Fatal("grid voltage absent")
	}
	for _, state := range []string{"stale", "failed"} {
		if state == "stale" {
			telemetry.ObservedAt = now.Add(-16 * time.Minute).Format(time.RFC3339)
		} else {
			collector.cached.success = false
		}
		families, err = registry.Gather()
		if err != nil {
			t.Fatal(err)
		}
		for _, family := range families {
			if family.GetName() == "gohome_growatt_inverter_grid_voltage_volts" {
				t.Fatalf("voltage survives %s", state)
			}
		}
	}
	delete(row, "fac")
	row["ppv"] = "NaN"
	row["pac"] = nil
	telemetry, err = decodeInverterTelemetry(row, "INVERTER", "GMT+1")
	if err != nil {
		t.Fatal(err)
	}
	for _, reading := range telemetry.Measurements {
		if reading.Quantity == v1.InverterQuantity_GRID_FREQUENCY_HERTZ || reading.Quantity == v1.InverterQuantity_PV_POWER_WATTS || reading.Quantity == v1.InverterQuantity_OUTPUT_POWER_WATTS {
			t.Fatal("missing or invalid measurement exported", reading)
		}
	}
}

func TestGridOutageRetainsZeroVoltage(t *testing.T) {
	telemetry, err := decodeInverterTelemetry(map[string]any{"time": "2026-11-08 16:30:28", "vac1": 0.0, "vpv1": 0.0, "ipv1": 0.0}, "INVERTER", "GMT+1")
	if err != nil {
		t.Fatal(err)
	}
	if len(telemetry.Measurements) != 3 {
		t.Fatalf("zero measurements dropped: %v", telemetry.Measurements)
	}
}
