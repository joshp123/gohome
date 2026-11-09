package weheat

import (
	"github.com/prometheus/client_golang/prometheus"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
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
			// History series were imported once from Weheat's hourly logs; the collector never exports them.
			if strings.HasPrefix(metric, "gohome_weheat_history_") {
				continue
			}
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
	// Every metric the collector describes, so this list cannot drift from the collector.
	descs := make(chan *prometheus.Desc, 1024)
	NewMetricsCollector(&Client{}).Describe(descs)
	close(descs)
	nameRE := regexp.MustCompile(`fqName: "([^"]+)"`)
	names := map[string]struct{}{}
	for desc := range descs {
		if m := nameRE.FindStringSubmatch(desc.String()); m != nil {
			names[m[1]] = struct{}{}
		}
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
	house, err := filepath.Glob(filepath.Join(dir, "..", "home", "dashboards", "*.json"))
	if err != nil || len(house) == 0 {
		t.Fatalf("find house dashboards: %v", err)
	}
	return append([]string{filepath.Join(dir, "dashboard.json")}, house...)
}
