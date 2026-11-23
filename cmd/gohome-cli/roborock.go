package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"

	"github.com/joshp123/gohome/internal/render"
	roborockv1 "github.com/joshp123/gohome/proto/gen/plugins/roborock/v1"
	"google.golang.org/grpc"
)

func roborockCmd(ctx context.Context, conn *grpc.ClientConn, out render.Renderer, args []string) int {
	sub := "status"
	if len(args) > 0 {
		sub = args[0]
	}

	client := roborockv1.NewRoborockServiceClient(conn)
	switch sub {
	case "status":
		flags := flag.NewFlagSet("roborock status", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		deviceID := flags.String("device", "", "Optional device id")
		if err := flags.Parse(args[min(1, len(args)):]); err != nil {
			out.WriteError("roborock status", err, render.ExitUsageError)
			return render.ExitUsageError
		}
		resp, err := client.GetStatus(ctx, &roborockv1.DeviceStatusRequest{DeviceId: *deviceID})
		if err != nil {
			out.WriteError("roborock status", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteProto(resp)
			return render.ExitOK
		}
		st := resp.GetStatus()
		pairs := []render.KeyValue{
			{Key: "state", Value: st.GetState()},
			{Key: "battery_percent", Value: fmt.Sprintf("%d%%", st.GetBatteryPercent())},
		}
		if st.GetErrorCode() != "0" && st.GetErrorCode() != "" {
			pairs = append(pairs, render.KeyValue{Key: "error", Value: st.GetErrorMessage()})
		}
		_ = out.WriteKeyValues(pairs)
		return render.ExitOK

	case "rooms":
		flags := flag.NewFlagSet("roborock rooms", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		deviceID := flags.String("device", "", "Optional device id")
		if err := flags.Parse(args[min(1, len(args)):]); err != nil {
			out.WriteError("roborock rooms", err, render.ExitUsageError)
			return render.ExitUsageError
		}
		resp, err := client.ListRooms(ctx, &roborockv1.ListRoomsRequest{DeviceId: *deviceID})
		if err != nil {
			out.WriteError("roborock rooms", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteProto(resp)
			return render.ExitOK
		}
		rows := make([][]string, 0, len(resp.Rooms))
		for _, room := range resp.Rooms {
			rows = append(rows, []string{room.GetLabel(), strconv.FormatUint(uint64(room.GetSegmentId()), 10)})
		}
		_ = out.WriteTable([]string{"ROOM", "ID"}, rows)
		return render.ExitOK

	case "map":
		flags := flag.NewFlagSet("roborock map", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		deviceID := flags.String("device", "", "Optional device id")
		labels := flags.String("labels", "", "Optional labels: names|segments")
		withTrace := flags.Bool("path", true, "Show cleaning trace")
		if err := flags.Parse(args[min(1, len(args)):]); err != nil {
			out.WriteError("roborock map", err, render.ExitUsageError)
			return render.ExitUsageError
		}
		name, err := resolveRoborockDeviceName(ctx, client, *deviceID)
		if err != nil {
			out.WriteError("roborock map", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		endpoint := resolveHTTPBase()
		query := url.Values{}
		query.Set("device_name", name)
		if *labels != "" {
			query.Set("labels", *labels)
		}
		query.Set("path", strconv.FormatBool(*withTrace))
		mapURL := fmt.Sprintf("%s/roborock/map.png?%s", endpoint, query.Encode())
		if out.Format == render.FormatJSON {
			_ = out.WriteJSON(struct {
				URL string `json:"url"`
			}{URL: mapURL})
			return render.ExitOK
		}
		_ = out.WriteLine(mapURL)
		return render.ExitOK

	case "clean", "mop", "vacuum", "smart", "dock", "locate":
		out.WriteError("roborock "+sub, fmt.Errorf("gohome-cli is read-only; use 'gohome-act roborock %s ...' for policy-gated writes", sub), render.ExitUsageError)
		return render.ExitUsageError

	default:
		out.WriteHelp("gohome-cli roborock <status|rooms|map>", nil)
		return render.ExitUsageError
	}
}

func resolveRoborockDeviceName(ctx context.Context, client roborockv1.RoborockServiceClient, deviceID string) (string, error) {
	resp, err := client.ListDevices(ctx, &roborockv1.ListDevicesRequest{})
	if err != nil {
		return "", err
	}
	if deviceID == "" {
		if len(resp.Devices) == 1 {
			return resp.Devices[0].GetName(), nil
		}
		return "", fmt.Errorf("device id required; pass --device <id>")
	}
	for _, dev := range resp.Devices {
		if dev.GetId() == deviceID {
			return dev.GetName(), nil
		}
	}
	return "", fmt.Errorf("device id %q not found", deviceID)
}
