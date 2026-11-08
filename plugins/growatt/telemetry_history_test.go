package growatt

import (
	"bytes"
	"strings"
	"testing"
)

func TestInverterHistoryUsesSourceTime(t *testing.T) {
	var output bytes.Buffer
	payload := []byte(`{"code":0,"data":{"datas":[{"time":"2026-11-07 12:00:00","vac1":230.5,"fac":50}],"haveNext":false}}`)
	count, err := WriteInverterHistory(&output, payload, Plant{ID: 42, Name: "Solar"}, "INVERTER", "GMT+1")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || !strings.Contains(output.String(), `gohome_growatt_inverter_grid_voltage_volts{plant_id="42",plant_name="Solar",serial="INVERTER",channel="1"} 230.5 1794049200000`) {
		t.Fatal(output.String())
	}
	for _, payload := range []string{`{"code":20,"message":"DATE_WITHIN_3_MONTHS"}`, `{"code":0,"data":{"haveNext":true}}`, `{"code":0,"data":{"datas":[{"vac1":230.5}]}}`} {
		output.Reset()
		if _, err := WriteInverterHistory(&output, []byte(payload), Plant{ID: 42}, "INVERTER", "GMT+1"); err == nil {
			t.Fatalf("accepted invalid history: %s", payload)
		}
	}
}
