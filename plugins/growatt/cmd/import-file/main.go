// import-file converts an existing Growatt history response to Prometheus text.
// It never contacts Growatt or writes to VictoriaMetrics.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/joshp123/gohome/plugins/growatt"
)

func main() {
	input := flag.String("input", "", "Captured queryHistoricalData JSON file")
	plantID := flag.Int64("plant-id", 0, "Plant ID")
	name := flag.String("plant-name", "", "Plant name")
	serial := flag.String("serial", "", "Inverter serial")
	zone := flag.String("timezone", "", "Source timezone, including the date's UTC offset")
	flag.Parse()
	if *input == "" || *plantID == 0 || *serial == "" || *zone == "" {
		flag.Usage()
		os.Exit(2)
	}
	payload, err := os.ReadFile(*input)
	if err == nil {
		_, err = growatt.WriteInverterHistory(os.Stdout, payload, growatt.Plant{ID: *plantID, Name: *name}, *serial, *zone)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
