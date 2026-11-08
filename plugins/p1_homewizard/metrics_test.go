package p1_homewizard

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"

	"github.com/prometheus/client_golang/prometheus"
	"testing"
)

func TestCostBreakdownTreatsNegativeExportTariffAsCredit(t *testing.T) {
	importT1 := 10.0
	importT2 := 5.0
	exportT1 := 3.0
	exportT2 := 2.0
	data := Data{
		TotalImportT1KWh: &importT1,
		TotalImportT2KWh: &importT2,
		TotalExportT1KWh: &exportT1,
		TotalExportT2KWh: &exportT2,
	}
	tariffs := Tariffs{
		ImportT1EurPerKWh: 0.20,
		ImportT2EurPerKWh: 0.30,
		ExportT1EurPerKWh: -0.10,
		ExportT2EurPerKWh: -0.05,
	}

	importCost, exportCredit, totalCost := costBreakdown(data, tariffs)

	if !closeEnough(importCost, 3.5) {
		t.Fatalf("importCost = %v, want 3.5", importCost)
	}
	if !closeEnough(exportCredit, 0.4) {
		t.Fatalf("exportCredit = %v, want 0.4", exportCredit)
	}
	if !closeEnough(totalCost, 3.1) {
		t.Fatalf("totalCost = %v, want 3.1", totalCost)
	}
}

func TestCostBreakdownTreatsPositiveExportTariffAsCredit(t *testing.T) {
	importKWh := 10.0
	exportKWh := 3.0
	data := Data{
		TotalImportT1KWh: &importKWh,
		TotalExportT1KWh: &exportKWh,
	}
	tariffs := Tariffs{
		ImportT1EurPerKWh: 0.20,
		ExportT1EurPerKWh: 0.10,
	}

	_, exportCredit, totalCost := costBreakdown(data, tariffs)

	if !closeEnough(exportCredit, 0.3) {
		t.Fatalf("exportCredit = %v, want 0.3", exportCredit)
	}
	if !closeEnough(totalCost, 1.7) {
		t.Fatalf("totalCost = %v, want 1.7", totalCost)
	}
}

func closeEnough(got, want float64) bool {
	return math.Abs(got-want) < 0.0000001
}

func TestVoltageAndWifiReadingsFollowEachScrape(t *testing.T) {
	payload := `{}`
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/telegram" {
			fmt.Fprint(w, "1-0:1.7.0(00.327*kW)")
			return
		}
		w.WriteHeader(status)
		fmt.Fprint(w, payload)
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(NewMetricsCollector(client, Tariffs{}))

	for _, step := range []struct {
		name, response string
		expected       map[string]float64
		status         int
	}{
		{"reading", `{"wifi_strength":100}`, map[string]float64{"gohome_p1_homewizard_wifi_strength_percent": 100}, http.StatusOK},
		{"real zero", `{"wifi_strength":0}`, map[string]float64{"gohome_p1_homewizard_wifi_strength_percent": 0}, http.StatusOK},
		{"null", `{"wifi_strength":null}`, map[string]float64{}, http.StatusOK},
		{"failed read", `unavailable`, map[string]float64{}, http.StatusServiceUnavailable},
		{"malformed read", `{`, map[string]float64{}, http.StatusOK},
		{"no optional readings", `{}`, map[string]float64{}, http.StatusOK},
		{"recovery", `{"wifi_strength":64}`, map[string]float64{"gohome_p1_homewizard_wifi_strength_percent": 64}, http.StatusOK},
	} {
		t.Run(step.name, func(t *testing.T) {
			payload, status = step.response, step.status
			families, err := registry.Gather()
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]float64{}
			for _, family := range families {
				switch family.GetName() {
				case "gohome_p1_homewizard_wifi_strength_percent":
					got[family.GetName()] = family.Metric[0].GetGauge().GetValue()
				}
			}
			if !reflect.DeepEqual(got, step.expected) {
				t.Fatalf("readings = %v, want %v", got, step.expected)
			}
		})
	}
}
