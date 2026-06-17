package p1_homewizard

import (
	"math"
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
