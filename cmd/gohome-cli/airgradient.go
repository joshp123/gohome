package main

import (
	"context"
	"fmt"

	"github.com/joshp123/gohome/internal/render"
	airgradientv1 "github.com/joshp123/gohome/proto/gen/plugins/airgradient/v1"
	"google.golang.org/grpc"
)

func airgradientCmd(ctx context.Context, conn *grpc.ClientConn, out render.Renderer, args []string) int {
	sub := "current"
	if len(args) > 0 {
		sub = args[0]
	}

	client := airgradientv1.NewAirGradientServiceClient(conn)
	switch sub {
	case "current", "status":
		resp, err := client.GetCurrent(ctx, &airgradientv1.GetCurrentRequest{})
		if err != nil {
			out.WriteError("airgradient current", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteProto(resp)
			return render.ExitOK
		}
		reading := resp.GetReading()
		if reading == nil {
			_ = out.WriteLine("no data")
			return render.ExitOK
		}
		pairs := make([]render.KeyValue, 0, 18)
		appendStringKV(&pairs, "serial", reading.Serial)
		appendStringKV(&pairs, "model", reading.Model)
		appendStringKV(&pairs, "firmware", reading.Firmware)
		appendStringKV(&pairs, "led_mode", reading.LedMode)
		appendFloatKV(&pairs, "co2_ppm", reading.Co2Ppm, "ppm")
		appendFloatKV(&pairs, "pm01_ugm3", reading.Pm01Ugm3, "ug/m3")
		appendFloatKV(&pairs, "pm02_ugm3", reading.Pm02Ugm3, "ug/m3")
		appendFloatKV(&pairs, "pm02_comp_ugm3", reading.Pm02CompensatedUgm3, "ug/m3")
		appendFloatKV(&pairs, "pm10_ugm3", reading.Pm10Ugm3, "ug/m3")
		appendFloatKV(&pairs, "temperature_c", reading.TemperatureCelsius, "C")
		appendFloatKV(&pairs, "temperature_comp_c", reading.TemperatureCompensatedCelsius, "C")
		appendFloatKV(&pairs, "humidity_percent", reading.HumidityPercent, "%")
		appendFloatKV(&pairs, "humidity_comp_percent", reading.HumidityCompensatedPercent, "%")
		appendFloatKV(&pairs, "tvoc_index", reading.TvocIndex, "")
		appendFloatKV(&pairs, "nox_index", reading.NoxIndex, "")
		appendFloatKV(&pairs, "wifi_rssi_dbm", reading.WifiRssiDbm, "dBm")
		appendFloatKV(&pairs, "boot_count", reading.BootCount, "")
		_ = out.WriteKeyValues(pairs)
		return render.ExitOK
	case "snapshot":
		resp, err := client.GetSnapshot(ctx, &airgradientv1.GetSnapshotRequest{})
		if err != nil {
			out.WriteError("airgradient snapshot", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteProto(resp)
			return render.ExitOK
		}
		_ = out.WriteLine(resp.GetJson())
		return render.ExitOK
	case "metrics":
		resp, err := client.GetMetrics(ctx, &airgradientv1.GetMetricsRequest{})
		if err != nil {
			out.WriteError("airgradient metrics", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteProto(resp)
			return render.ExitOK
		}
		_ = out.WriteRaw(resp.GetOpenmetrics())
		return render.ExitOK
	case "config":
		resp, err := client.GetConfig(ctx, &airgradientv1.GetConfigRequest{})
		if err != nil {
			out.WriteError("airgradient config", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteProto(resp)
			return render.ExitOK
		}
		_ = out.WriteLine(resp.GetJson())
		return render.ExitOK
	default:
		out.WriteHelp("gohome-cli airgradient <current|snapshot|metrics|config>", nil)
		return render.ExitUsageError
	}
}

func appendStringKV(pairs *[]render.KeyValue, label string, value *string) {
	if value == nil || *value == "" {
		return
	}
	*pairs = append(*pairs, render.KeyValue{Key: label, Value: *value})
}

func appendFloatKV(pairs *[]render.KeyValue, label string, value *float64, unit string) {
	if value == nil {
		return
	}
	formatted := fmt.Sprintf("%.2f", *value)
	if unit != "" {
		formatted = fmt.Sprintf("%s %s", formatted, unit)
	}
	*pairs = append(*pairs, render.KeyValue{Key: label, Value: formatted})
}
