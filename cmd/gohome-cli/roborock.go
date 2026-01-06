package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	roborockv1 "github.com/joshp123/gohome/proto/gen/plugins/roborock/v1"
	"google.golang.org/grpc"
)

func roborockCmd(ctx context.Context, conn *grpc.ClientConn, args []string, jsonOutput bool) {
	out := outputMode{json: jsonOutput}
	if len(args) == 0 {
		roborockUsage()
		os.Exit(2)
	}

	client := roborockv1.NewRoborockServiceClient(conn)
	switch args[0] {
	case "status":
		flags := flag.NewFlagSet("roborock status", flag.ExitOnError)
		deviceID := flags.String("device", "", "Optional device id")
		_ = flags.Parse(args[1:])
		resp, err := client.GetStatus(ctx, &roborockv1.DeviceStatusRequest{DeviceId: *deviceID})
		if err != nil {
			fatal("roborock status", err)
		}
		if out.json {
			out.printJSON(resp)
			return
		}
		status := resp.GetStatus()
		fmt.Printf("STATE:   %s\n", status.GetState())
		fmt.Printf("BATTERY: %d%%\n", status.GetBatteryPercent())
		if status.GetErrorCode() != "0" && status.GetErrorCode() != "" {
			fmt.Printf("ERROR:   %s\n", status.GetErrorMessage())
		}
	case "rooms":
		flags := flag.NewFlagSet("roborock rooms", flag.ExitOnError)
		deviceID := flags.String("device", "", "Optional device id")
		_ = flags.Parse(args[1:])
		resp, err := client.ListRooms(ctx, &roborockv1.ListRoomsRequest{DeviceId: *deviceID})
		if err != nil {
			fatal("roborock rooms", err)
		}
		if out.json {
			out.printJSON(resp)
			return
		}
		rows := [][]string{{"ROOM", "ID"}}
		for _, room := range resp.Rooms {
			rows = append(rows, []string{room.Label, strconv.FormatUint(uint64(room.SegmentId), 10)})
		}
		out.table(rows)
	case "clean":
		flags := flag.NewFlagSet("roborock clean", flag.ExitOnError)
		deviceID := flags.String("device", "", "Optional device id")
		all := flags.Bool("all", false, "Clean all rooms")
		dryRun := flags.Bool("dry-run", false, "Dry run (no movement)")
		repeats := flags.Int("repeats", 0, "Repeat count")
		times := flags.Int("times", 0, "Repeat count (alias for --repeats)")
		fan := flags.String("fan", "", "Fan speed override")
		mopMode := flags.String("mop", "", "Mop mode override")
		mopIntensity := flags.String("intensity", "", "Mop intensity override")
		suction := flags.String("suction", "", "Suction level: quiet|balanced|turbo|max")
		water := flags.String("water", "", "Water flow: low|med|high|max")
		_ = flags.Parse(args[1:])
		if *times != 0 {
			*repeats = *times
		}
		rooms := flags.Args()
		if *all {
			_, err := client.StartClean(ctx, &roborockv1.StartCleanRequest{DeviceId: *deviceID})
			if err != nil {
				fatal("roborock clean all", err)
			}
			if out.json {
				out.printJSON(map[string]any{"status": "ok", "mode": "all"})
				return
			}
			fmt.Println("ok: started full clean")
			return
		}
		if len(rooms) == 0 {
			fatal("roborock clean", fmt.Errorf("usage: gohome-cli roborock clean <room>[,<room>]"))
		}
		if *suction != "" && *fan != "" {
			fatal("roborock clean", fmt.Errorf("--suction and --fan cannot be used together"))
		}
		if *water != "" && *mopIntensity != "" {
			fatal("roborock clean", fmt.Errorf("--water and --intensity cannot be used together"))
		}
		if *water != "" && *mopMode == "" {
			*mopMode = defaultMopModeStandard
		}
		if *suction != "" && *fan == "" {
			fanResolved, err := resolveSuction(*suction)
			if err != nil {
				fatal("roborock clean", err)
			}
			*fan = fanResolved
		}
		if *water != "" && *mopIntensity == "" {
			waterResolved, err := resolveWater(*water)
			if err != nil {
				fatal("roborock clean", err)
			}
			*mopIntensity = waterResolved
		}
		if *fan == "" {
			*fan = defaultSuctionBalanced
		}
		if *mopMode == "" {
			*mopMode = defaultMopModeStandard
		}
		if *mopIntensity == "" {
			*mopIntensity = defaultWaterHigh
		}
		if *repeats == 0 {
			*repeats = 1
		}
		roomList := splitCommaArgs(rooms)
		runRoomClean(ctx, client, out, "clean", *deviceID, roomList, cleanOptions{
			repeats:      *repeats,
			fanSpeed:     *fan,
			mopMode:      *mopMode,
			mopIntensity: *mopIntensity,
			dryRun:       *dryRun,
		})
	case "mop":
		flags := flag.NewFlagSet("roborock mop", flag.ExitOnError)
		deviceID := flags.String("device", "", "Optional device id")
		dryRun := flags.Bool("dry-run", false, "Dry run (no movement)")
		repeats := flags.Int("repeats", 0, "Repeat count")
		times := flags.Int("times", 0, "Repeat count (alias for --repeats)")
		water := flags.String("water", "", "Water flow: low|med|high|max")
		_ = flags.Parse(args[1:])
		if *times != 0 {
			*repeats = *times
		}
		roomList := splitCommaArgs(flags.Args())
		if len(roomList) == 0 {
			fatal("roborock mop", fmt.Errorf("usage: gohome-cli roborock mop <room>[,<room>]"))
		}
		waterResolved := defaultWaterHigh
		if *water != "" {
			parsed, err := resolveWater(*water)
			if err != nil {
				fatal("roborock mop", err)
			}
			waterResolved = parsed
		}
		if *repeats == 0 {
			*repeats = 1
		}
		runRoomClean(ctx, client, out, "mop", *deviceID, roomList, cleanOptions{
			repeats:      *repeats,
			fanSpeed:     "0",
			mopMode:      defaultMopModeStandard,
			mopIntensity: waterResolved,
			dryRun:       *dryRun,
		})
	case "vacuum":
		flags := flag.NewFlagSet("roborock vacuum", flag.ExitOnError)
		deviceID := flags.String("device", "", "Optional device id")
		dryRun := flags.Bool("dry-run", false, "Dry run (no movement)")
		repeats := flags.Int("repeats", 0, "Repeat count")
		times := flags.Int("times", 0, "Repeat count (alias for --repeats)")
		suction := flags.String("suction", "", "Suction level: quiet|balanced|turbo|max")
		_ = flags.Parse(args[1:])
		if *times != 0 {
			*repeats = *times
		}
		roomList := splitCommaArgs(flags.Args())
		if len(roomList) == 0 {
			fatal("roborock vacuum", fmt.Errorf("usage: gohome-cli roborock vacuum <room>[,<room>]"))
		}
		suctionResolved := defaultSuctionBalanced
		if *suction != "" {
			parsed, err := resolveSuction(*suction)
			if err != nil {
				fatal("roborock vacuum", err)
			}
			suctionResolved = parsed
		}
		if *repeats == 0 {
			*repeats = 1
		}
		runRoomClean(ctx, client, out, "vacuum", *deviceID, roomList, cleanOptions{
			repeats:      *repeats,
			fanSpeed:     suctionResolved,
			mopMode:      "0",
			mopIntensity: "0",
			dryRun:       *dryRun,
		})
	case "smart":
		flags := flag.NewFlagSet("roborock smart", flag.ExitOnError)
		deviceID := flags.String("device", "", "Optional device id")
		dryRun := flags.Bool("dry-run", false, "Dry run (no movement)")
		repeats := flags.Int("repeats", 0, "Repeat count")
		times := flags.Int("times", 0, "Repeat count (alias for --repeats)")
		_ = flags.Parse(args[1:])
		if *times != 0 {
			*repeats = *times
		}
		roomList := splitCommaArgs(flags.Args())
		if len(roomList) == 0 {
			fatal("roborock smart", fmt.Errorf("usage: gohome-cli roborock smart <room>[,<room>]"))
		}
		runRoomClean(ctx, client, out, "smart", *deviceID, roomList, cleanOptions{
			repeats: *repeats,
			dryRun:  *dryRun,
		})
	case "dock":
		flags := flag.NewFlagSet("roborock dock", flag.ExitOnError)
		deviceID := flags.String("device", "", "Optional device id")
		_ = flags.Parse(args[1:])
		_, err := client.Dock(ctx, &roborockv1.DockRequest{DeviceId: *deviceID})
		if err != nil {
			fatal("roborock dock", err)
		}
		fmt.Println("ok: dock")
	case "locate":
		flags := flag.NewFlagSet("roborock locate", flag.ExitOnError)
		deviceID := flags.String("device", "", "Optional device id")
		_ = flags.Parse(args[1:])
		_, err := client.Locate(ctx, &roborockv1.LocateRequest{DeviceId: *deviceID})
		if err != nil {
			fatal("roborock locate", err)
		}
		fmt.Println("ok: locate")
	case "map":
		flags := flag.NewFlagSet("roborock map", flag.ExitOnError)
		deviceID := flags.String("device", "", "Optional device id")
		labels := flags.String("labels", "", "Optional labels: names|segments")
		withTrace := flags.Bool("path", true, "Show cleaning trace")
		_ = flags.Parse(args[1:])
		name, err := resolveDeviceName(ctx, client, *deviceID)
		if err != nil {
			fatal("roborock map", err)
		}
		endpoint := resolveHTTPBase()
		query := url.Values{}
		query.Set("device_name", name)
		if *labels != "" {
			query.Set("labels", *labels)
		}
		if *withTrace {
			query.Set("path", "true")
		} else {
			query.Set("path", "false")
		}
		fmt.Printf("%s/roborock/map.png?%s\n", endpoint, query.Encode())
	default:
		roborockUsage()
		os.Exit(2)
	}
}

func roborockUsage() {
	fmt.Println("gohome-cli roborock <command>")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  status [--device id]")
	fmt.Println("  rooms [--device id]")
	fmt.Println("  clean <room>[,<room>] [--device id] [--dry-run] [--repeats N]")
	fmt.Println("  mop <room>[,<room>] [--device id] [--water low|med|high|max] [--repeats N]")
	fmt.Println("  vacuum <room>[,<room>] [--device id] [--suction quiet|balanced|turbo|max] [--repeats N]")
	fmt.Println("  smart <room>[,<room>] [--device id] [--repeats N]")
	fmt.Println("  clean --all [--device id]")
	fmt.Println("  dock [--device id]")
	fmt.Println("  locate [--device id]")
	fmt.Println("  map [--device id] [--labels names|segments] [--path]")
}

func resolveDeviceName(ctx context.Context, client roborockv1.RoborockServiceClient, deviceID string) (string, error) {
	resp, err := client.ListDevices(ctx, &roborockv1.ListDevicesRequest{})
	if err != nil {
		return "", err
	}
	if deviceID == "" {
		if len(resp.Devices) == 1 {
			return resp.Devices[0].Name, nil
		}
		return "", fmt.Errorf("device id required; run 'gohome-cli roborock status --device <id>'")
	}
	for _, dev := range resp.Devices {
		if dev.Id == deviceID {
			return dev.Name, nil
		}
	}
	return "", fmt.Errorf("device id %q not found", deviceID)
}

func fetchRoomMap(ctx context.Context, client roborockv1.RoborockServiceClient, deviceID string) (map[string]string, error) {
	resp, err := client.ListRooms(ctx, &roborockv1.ListRoomsRequest{DeviceId: deviceID})
	if err != nil {
		return nil, err
	}
	rooms := make(map[string]string)
	for _, room := range resp.Rooms {
		rooms[room.Label] = strconv.FormatUint(uint64(room.SegmentId), 10)
	}
	return rooms, nil
}

func splitCommaArgs(args []string) []string {
	var out []string
	for _, arg := range args {
		parts := strings.Split(arg, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}
	sort.Strings(out)
	return out
}

type cleanOptions struct {
	repeats      int
	fanSpeed     string
	mopMode      string
	mopIntensity string
	dryRun       bool
}

const (
	defaultSuctionBalanced = "102"
	defaultWaterHigh       = "202"
	defaultMopModeStandard = "300"
)

var suctionLevels = map[string]string{
	"quiet":    "101",
	"balanced": "102",
	"turbo":    "103",
	"max":      "104",
}

var waterLevels = map[string]string{
	"low":    "200",
	"med":    "201",
	"medium": "201",
	"high":   "202",
	"max":    "203",
}

func resolveSuction(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if _, err := strconv.Atoi(value); err == nil {
		return value, nil
	}
	key := strings.ToLower(strings.TrimSpace(value))
	if mapped, ok := suctionLevels[key]; ok {
		return mapped, nil
	}
	return "", fmt.Errorf("invalid suction %q (use quiet|balanced|turbo|max)", value)
}

func resolveWater(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if _, err := strconv.Atoi(value); err == nil {
		return value, nil
	}
	key := strings.ToLower(strings.TrimSpace(value))
	if mapped, ok := waterLevels[key]; ok {
		return mapped, nil
	}
	return "", fmt.Errorf("invalid water flow %q (use low|med|high|max)", value)
}

func runRoomClean(ctx context.Context, client roborockv1.RoborockServiceClient, out outputMode, label, deviceID string, rooms []string, opts cleanOptions) {
	roomMap, err := fetchRoomMap(ctx, client, deviceID)
	if err != nil {
		fatal(fmt.Sprintf("roborock %s", label), err)
	}
	for _, room := range rooms {
		_, err := resolveNamedID("room", room, roomMap)
		if err != nil {
			fatal(fmt.Sprintf("roborock %s", label), err)
		}
		req := &roborockv1.CleanRoomRequest{
			DeviceId:     deviceID,
			Room:         room,
			Repeats:      uint32(opts.repeats),
			FanSpeed:     opts.fanSpeed,
			MopMode:      opts.mopMode,
			MopIntensity: opts.mopIntensity,
			DryRun:       opts.dryRun,
		}
		resp, err := client.CleanRoom(ctx, req)
		if err != nil {
			fatal(fmt.Sprintf("roborock %s", label), err)
		}
		if out.json {
			out.printJSON(resp)
			continue
		}
		fmt.Printf("ok: %s %s (segment %d)\n", label, resp.Room, resp.SegmentId)
	}
}
