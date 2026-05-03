package weheat

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"testing"
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

func TestDashboardsOnlyReferenceExportedWeheatMetrics(t *testing.T) {
	metricRE := regexp.MustCompile(`gohome_weheat_[a-z0-9_]+`)
	exported := exportedWeheatMetrics()
	paths := dashboardPaths(t)

	var missing []string
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, metric := range metricRE.FindAllString(string(body), -1) {
			if _, ok := exported[metric]; !ok {
				missing = append(missing, metric)
			}
		}
	}

	if len(missing) == 0 {
		return
	}
	slices.Sort(missing)
	missing = slices.Compact(missing)
	t.Fatalf("dashboard references unknown Weheat metrics: %v", missing)
}

func exportedWeheatMetrics() map[string]struct{} {
	names := map[string]struct{}{
		"gohome_weheat_scrape_success":                            {},
		"gohome_weheat_energy_scrape_success":                     {},
		"gohome_weheat_energy_log_scrape_success":                 {},
		"gohome_weheat_last_success_timestamp_seconds":            {},
		"gohome_weheat_energy_last_success_timestamp_seconds":     {},
		"gohome_weheat_energy_log_last_success_timestamp_seconds": {},
		"gohome_weheat_last_update_timestamp_seconds":             {},
		"gohome_weheat_energy_log_time_bucket_timestamp_seconds":  {},
		"gohome_weheat_energy_total_ein_heating_kwh":              {},
		"gohome_weheat_energy_total_ein_standby_kwh":              {},
		"gohome_weheat_energy_total_ein_dhw_kwh":                  {},
		"gohome_weheat_energy_total_ein_heating_defrost_kwh":      {},
		"gohome_weheat_energy_total_ein_dhw_defrost_kwh":          {},
		"gohome_weheat_energy_total_ein_cooling_kwh":              {},
		"gohome_weheat_energy_total_eout_heating_kwh":             {},
		"gohome_weheat_energy_total_eout_dhw_kwh":                 {},
		"gohome_weheat_energy_total_eout_heating_defrost_kwh":     {},
		"gohome_weheat_energy_total_eout_dhw_defrost_kwh":         {},
		"gohome_weheat_energy_total_eout_cooling_kwh":             {},
	}
	for _, field := range buildLogFields() {
		names[field.metricName] = struct{}{}
	}
	for _, field := range buildEnergyLogFields() {
		names[field.metricName] = struct{}{}
	}
	return names
}

func dashboardPaths(t *testing.T) []string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	return []string{
		filepath.Join(dir, "dashboard.json"),
		filepath.Join(dir, "..", "home", "dashboard.json"),
	}
}
