package weheat

import "testing"

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
