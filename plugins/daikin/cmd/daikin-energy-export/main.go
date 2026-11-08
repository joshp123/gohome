// daikin-energy-export converts a saved GetUnitState JSON payload to VictoriaMetrics JSONL.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/joshp123/gohome/plugins/daikin"
)

func main() {
	input := flag.String("input", "", "Saved raw gateway device JSON (not the gRPC envelope)")
	flag.Parse()
	if err := run(*input); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(input string) error {
	payload, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	readings, observed, err := daikin.DecodeEnergyReadings(payload)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	count := 0
	for _, r := range readings {
		// Daikin can publish a completed bucket up to two hours late.
		if r.End.After(observed.Add(-2 * time.Hour)) {
			continue
		}
		sample := struct {
			Metric     map[string]string `json:"metric"`
			Values     []float64         `json:"values"`
			Timestamps []int64           `json:"timestamps"`
		}{
			map[string]string{"__name__": "gohome_daikin_electricity_consumption_kwh", "unit_id": r.UnitID, "unit_name": r.UnitName, "embedded_id": r.EmbeddedID, "operation_mode": r.Mode, "resolution": r.Resolution, "job": "gohome", "instance": "gohome", "source": "daikin"},
			[]float64{r.KWh}, []int64{r.Start.UnixMilli()},
		}
		if err := encoder.Encode(sample); err != nil {
			return err
		}
		count++
	}
	if count == 0 {
		return fmt.Errorf("no completed consumption buckets available")
	}
	fmt.Fprintf(os.Stderr, "Exported %d completed raw buckets; gateway timestamp %s\n", count, observed.Format(time.RFC3339))
	return nil
}
