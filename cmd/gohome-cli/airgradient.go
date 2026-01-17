package main

import (
	"context"
	"fmt"
	"os"

	airgradientv1 "github.com/joshp123/gohome/proto/gen/plugins/airgradient/v1"
	"google.golang.org/grpc"
)

func airgradientCmd(ctx context.Context, conn *grpc.ClientConn, args []string, jsonOutput bool) {
	out := outputMode{json: jsonOutput}
	if len(args) == 0 {
		airgradientUsage()
		os.Exit(2)
	}

	client := airgradientv1.NewAirGradientServiceClient(conn)
	switch args[0] {
	case "current", "status":
		resp, err := client.GetCurrent(ctx, &airgradientv1.GetCurrentRequest{})
		if err != nil {
			fatal("airgradient current", err)
		}
		if out.json {
			out.printJSON(resp)
			return
		}
		reading := resp.GetReading()
		if reading == nil {
			fmt.Println("no data")
			return
		}
		rows := [][]string{{"METRIC", "VALUE"}}
		addStringRow(&rows, "serial", reading.Serial)
		addStringRow(&rows, "model", reading.Model)
		addStringRow(&rows, "firmware", reading.Firmware)
		addStringRow(&rows, "led_mode", reading.LedMode)
		addFloatRow(&rows, "co2_ppm", reading.Co2Ppm, "ppm")
		addFloatRow(&rows, "pm01_ugm3", reading.Pm01Ugm3, "ug/m3")
		addFloatRow(&rows, "pm02_ugm3", reading.Pm02Ugm3, "ug/m3")
		addFloatRow(&rows, "pm02_comp_ugm3", reading.Pm02CompensatedUgm3, "ug/m3")
		addFloatRow(&rows, "pm10_ugm3", reading.Pm10Ugm3, "ug/m3")
		addFloatRow(&rows, "temperature_c", reading.TemperatureCelsius, "C")
		addFloatRow(&rows, "temperature_comp_c", reading.TemperatureCompensatedCelsius, "C")
		addFloatRow(&rows, "humidity_percent", reading.HumidityPercent, "%")
		addFloatRow(&rows, "humidity_comp_percent", reading.HumidityCompensatedPercent, "%")
		addFloatRow(&rows, "tvoc_index", reading.TvocIndex, "")
		addFloatRow(&rows, "nox_index", reading.NoxIndex, "")
		addFloatRow(&rows, "wifi_rssi_dbm", reading.WifiRssiDbm, "dBm")
		addFloatRow(&rows, "boot_count", reading.BootCount, "")
		out.table(rows)
	case "snapshot":
		resp, err := client.GetSnapshot(ctx, &airgradientv1.GetSnapshotRequest{})
		if err != nil {
			fatal("airgradient snapshot", err)
		}
		if out.json {
			out.printJSON(resp)
			return
		}
		fmt.Println(resp.GetJson())
	case "metrics":
		resp, err := client.GetMetrics(ctx, &airgradientv1.GetMetricsRequest{})
		if err != nil {
			fatal("airgradient metrics", err)
		}
		if out.json {
			out.printJSON(resp)
			return
		}
		fmt.Print(resp.GetOpenmetrics())
	case "config":
		resp, err := client.GetConfig(ctx, &airgradientv1.GetConfigRequest{})
		if err != nil {
			fatal("airgradient config", err)
		}
		if out.json {
			out.printJSON(resp)
			return
		}
		fmt.Println(resp.GetJson())
	default:
		airgradientUsage()
		os.Exit(2)
	}
}

func airgradientUsage() {
	fmt.Println("gohome-cli airgradient <command>")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  current")
	fmt.Println("  snapshot")
	fmt.Println("  metrics")
	fmt.Println("  config")
}

func addStringRow(rows *[][]string, label string, value *string) {
	if value == nil || *value == "" {
		return
	}
	*rows = append(*rows, []string{label, *value})
}

func addFloatRow(rows *[][]string, label string, value *float64, unit string) {
	if value == nil {
		return
	}
	formatted := fmt.Sprintf("%.2f", *value)
	if unit != "" {
		formatted = fmt.Sprintf("%s %s", formatted, unit)
	}
	*rows = append(*rows, []string{label, formatted})
}
