package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	tadov1 "github.com/joshp123/gohome/proto/gen/plugins/tado/v1"
	"google.golang.org/grpc"
)

func tadoCmd(ctx context.Context, conn *grpc.ClientConn, args []string, jsonOutput bool) {
	out := outputMode{json: jsonOutput}
	if len(args) == 0 {
		tadoUsage()
		os.Exit(2)
	}

	client := tadov1.NewTadoServiceClient(conn)
	switch args[0] {
	case "zones", "list":
		resp, err := client.ListZones(ctx, &tadov1.ListZonesRequest{})
		if err != nil {
			fatal("tado list zones", err)
		}
		if out.json {
			out.printJSON(resp)
			return
		}
		rows := [][]string{{"ZONE", "ID"}}
		for _, zone := range resp.Zones {
			rows = append(rows, []string{zone.Name, zone.Id})
		}
		out.table(rows)
	case "set":
		if len(args) < 3 {
			fatal("tado set", fmt.Errorf("usage: gohome-cli tado set <zone> <temp>"))
		}
		zoneName := args[1]
		temp, err := strconv.ParseFloat(args[2], 64)
		if err != nil {
			fatal("tado set", fmt.Errorf("invalid temperature %q", args[2]))
		}
		zones, err := client.ListZones(ctx, &tadov1.ListZonesRequest{})
		if err != nil {
			fatal("tado list zones", err)
		}
		zoneMap := make(map[string]string)
		for _, zone := range zones.Zones {
			zoneMap[zone.Name] = zone.Id
		}
		zoneID, err := resolveNamedID("zone", zoneName, zoneMap)
		if err != nil {
			fatal("tado set", err)
		}
		_, err = client.SetTemperature(ctx, &tadov1.SetTemperatureRequest{ZoneId: zoneID, TemperatureCelsius: temp})
		if err != nil {
			fatal("tado set", err)
		}
		if out.json {
			out.printJSON(map[string]any{"zone": zoneName, "temperature_celsius": temp, "status": "ok"})
			return
		}
		fmt.Printf("ok: %s -> %.1f°C\n", strings.ToLower(zoneName), temp)
	default:
		tadoUsage()
		os.Exit(2)
	}
}

func tadoUsage() {
	fmt.Println("gohome-cli tado <command>")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  zones")
	fmt.Println("  set <zone> <temp>")
}
