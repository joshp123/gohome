package weheat

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestBuildLogFieldsIncludesCurrentUpstreamPowerFields(t *testing.T) {
	names := make(map[string]struct{})
	for _, field := range buildLogFields() {
		names[field.metricName] = struct{}{}
	}

	required := []string{
		"gohome_weheat_log_cm_mass_power_in",
		"gohome_weheat_log_cm_mass_power_out",
		"gohome_weheat_log_compressor_power_low_accuracy",
	}
	for _, metric := range required {
		if _, ok := names[metric]; !ok {
			t.Fatalf("missing raw log metric %q", metric)
		}
	}
}

func TestBuildEnergyLogFieldsIncludesAveragePowerFields(t *testing.T) {
	names := make(map[string]struct{})
	for _, field := range buildEnergyLogFields() {
		names[field.metricName] = struct{}{}
	}

	required := []string{
		"gohome_weheat_energy_log_average_power_ein_heating",
		"gohome_weheat_energy_log_average_power_ein_standby",
		"gohome_weheat_energy_log_average_power_ein_dhw",
		"gohome_weheat_energy_log_average_power_ein_heating_defrost",
		"gohome_weheat_energy_log_average_power_ein_dhw_defrost",
		"gohome_weheat_energy_log_average_power_ein_cooling",
		"gohome_weheat_energy_log_average_power_eout_heating",
		"gohome_weheat_energy_log_average_power_eout_dhw",
		"gohome_weheat_energy_log_average_power_eout_heating_defrost",
		"gohome_weheat_energy_log_average_power_eout_dhw_defrost",
		"gohome_weheat_energy_log_average_power_eout_cooling",
	}
	for _, metric := range required {
		if _, ok := names[metric]; !ok {
			t.Fatalf("missing energy log metric %q", metric)
		}
	}
}

func TestApplyLogDropsFlow255SentinelAndKeepsValidFlow(t *testing.T) {
	collector := NewMetricsCollector(nil)
	labels := prometheus.Labels{"heat_pump_id": "example", "serial_number": "example", "name": "example", "model": "example"}

	sentinel := 255
	valid := 38
	collector.applyLog(labels, &latestLog{
		DHWFlow:            &sentinel,
		CentralHeatingFlow: &valid,
	})

	registry := prometheus.NewRegistry()
	registry.MustRegister(collector.logMetrics["dhwFlow"], collector.logMetrics["centralHeatingFlow"])
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	if len(families) != 1 || families[0].GetName() != "gohome_weheat_log_central_heating_flow" {
		t.Fatalf("expected only central_heating_flow (dhwFlow=255 dropped), got %v", families)
	}
	if got := families[0].Metric[0].Gauge.GetValue(); got != 38.0 {
		t.Fatalf("central_heating_flow = %v, want 38", got)
	}
}
