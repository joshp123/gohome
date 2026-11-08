package roborock

import (
	"encoding/json"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestConsumablesPreserveMissingAndZero(t *testing.T) {
	var counters Consumables
	if err := json.Unmarshal([]byte(`{"main_brush_work_time":12345,"filter_element_work_time":0,"strainer_work_times":24}`), &counters); err != nil {
		t.Fatal(err)
	}
	status := mapStatus(Status{Consumables: counters})
	if status.Consumables.MainBrushSeconds == nil || *status.Consumables.MainBrushSeconds != 12345 || status.Consumables.FilterElementSeconds == nil || *status.Consumables.FilterElementSeconds != 0 || status.Consumables.MopRollerSeconds != nil || status.Consumables.StrainerCycles == nil || *status.Consumables.StrainerCycles != 24 {
		t.Fatalf("incorrect consumables: %v", status.Consumables)
	}
	collector := NewMetricsCollector(nil)
	registry := prometheus.NewRegistry()
	registry.MustRegister(collector.consumableSeconds, collector.consumableCycles)
	collector.collectConsumables(DeviceState{Device: Device{ID: "test", Name: "Vacuum", Model: "test"}, Status: Status{Consumables: counters}})
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	if len(families) != 2 {
		t.Fatalf("got %d metric families", len(families))
	}
	values := map[string]float64{}
	for _, family := range families {
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if label.GetName() == "consumable" {
					values[label.GetValue()] = metric.Gauge.GetValue()
				}
			}
		}
	}
	if len(values) != 3 || values["main_brush"] != 12345 || values["filter_element"] != 0 || values["strainer"] != 24 {
		t.Fatalf("incorrect readings: %v", values)
	}
	collector.consumableSeconds.Reset()
	collector.consumableCycles.Reset()
	collector.collectConsumables(DeviceState{})
	families, err = registry.Gather()
	if err != nil || len(families) != 0 {
		t.Fatalf("missing readings repeated old values: %v, %v", families, err)
	}
}
