package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	hikvisionv1 "github.com/joshp123/gohome/proto/gen/plugins/hikvision/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func hikvisionCmd(ctx context.Context, conn *grpc.ClientConn, args []string, jsonOutput bool) {
	if len(args) == 0 {
		hikvisionUsage()
		os.Exit(2)
	}

	client := hikvisionv1.NewHikvisionServiceClient(conn)
	switch args[0] {
	case "camera":
		resp, err := client.GetCamera(ctx, &hikvisionv1.GetCameraRequest{})
		if err != nil {
			fatal("hikvision camera", err)
		}
		fmt.Println(protojson.MarshalOptions{Multiline: true}.Format(resp.GetCamera()))
	case "snapshot":
		if len(args) < 2 {
			fatal("hikvision snapshot", fmt.Errorf("missing output file, e.g. snapshot camera.jpg"))
		}
		resp, err := client.GetSnapshot(ctx, &hikvisionv1.GetSnapshotRequest{})
		if err != nil {
			fatal("hikvision snapshot", err)
		}
		if err := os.WriteFile(args[1], resp.GetJpeg(), 0o600); err != nil {
			fatal("hikvision snapshot", err)
		}
		fmt.Printf("wrote %s (%d bytes, captured %s)\n", args[1], len(resp.GetJpeg()),
			resp.GetCapturedAt().AsTime().Local().Format(time.RFC3339))
	case "events":
		// Runs until interrupted, so it does not use the command's ten-second deadline.
		watchCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		stream, err := client.WatchEvents(watchCtx, &hikvisionv1.WatchEventsRequest{})
		if err != nil {
			fatal("hikvision events", err)
		}
		for {
			event, err := stream.Recv()
			if err != nil {
				if watchCtx.Err() != nil {
					return
				}
				fatal("hikvision events", err)
			}
			if jsonOutput {
				fmt.Println(protojson.MarshalOptions{}.Format(event))
				continue
			}
			fmt.Printf("%s\t%s\t%s\t%s\n",
				event.GetReceivedAt().AsTime().Local().Format(time.RFC3339),
				enumWord(event.GetKind().String(), "EVENT_KIND_"),
				enumWord(event.GetState().String(), "EVENT_STATE_"),
				enumWord(event.GetTarget().String(), "DETECTION_TARGET_"))
		}
	case "alarms":
		since := 24 * time.Hour
		if len(args) > 1 {
			parsed, err := time.ParseDuration(args[1])
			if err != nil {
				fatal("hikvision alarms", fmt.Errorf("since must be a duration such as 24h: %w", err))
			}
			since = parsed
		}
		resp, err := client.ListAlarms(ctx, &hikvisionv1.ListAlarmsRequest{Since: timestamppb.New(time.Now().Add(-since))})
		if err != nil {
			fatal("hikvision alarms", err)
		}
		for _, alarm := range resp.GetAlarms() {
			printAlarm(alarm, jsonOutput)
		}
	case "watch-alarms":
		watchCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		stream, err := client.WatchAlarms(watchCtx, &hikvisionv1.WatchAlarmsRequest{})
		if err != nil {
			fatal("hikvision watch-alarms", err)
		}
		for {
			alarm, err := stream.Recv()
			if err != nil {
				if watchCtx.Err() != nil {
					return
				}
				fatal("hikvision watch-alarms", err)
			}
			printAlarm(alarm, jsonOutput)
		}
	case "frame":
		if len(args) < 3 {
			fatal("hikvision frame", fmt.Errorf("usage: frame <RFC 3339 time> <file.jpg>"))
		}
		at, err := time.Parse(time.RFC3339Nano, args[1])
		if err != nil {
			fatal("hikvision frame", err)
		}
		frameCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		resp, err := client.GetFrame(frameCtx, &hikvisionv1.GetFrameRequest{At: timestamppb.New(at)})
		if err != nil {
			fatal("hikvision frame", err)
		}
		if err := os.WriteFile(args[2], resp.GetJpeg(), 0o600); err != nil {
			fatal("hikvision frame", err)
		}
		fmt.Printf("wrote %s (%d bytes, recorded %s)\n", args[2], len(resp.GetJpeg()),
			resp.GetAt().AsTime().Local().Format(time.RFC3339))
	default:
		hikvisionUsage()
		os.Exit(2)
	}
}

func printAlarm(alarm *hikvisionv1.Alarm, jsonOutput bool) {
	if jsonOutput {
		fmt.Println(protojson.MarshalOptions{}.Format(alarm))
		return
	}
	duration := "under way"
	if alarm.GetDuration() != nil {
		duration = alarm.GetDuration().AsDuration().String()
	}
	fmt.Printf("%s\t%s\t%s\t%s\n",
		alarm.GetStartedAt().AsTime().Local().Format(time.RFC3339),
		enumWord(alarm.GetKind().String(), "EVENT_KIND_"),
		enumWord(alarm.GetState().String(), "EVENT_STATE_"),
		duration)
}

func enumWord(name, prefix string) string {
	return strings.ToLower(strings.TrimPrefix(name, prefix))
}

func hikvisionUsage() {
	fmt.Println("gohome-cli hikvision <command>")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  camera            identity, firmware, detection settings, event-stream status")
	fmt.Println("  snapshot <file>   save one JPEG from the main stream")
	fmt.Println("  events            print raw alert-stream events as they arrive (Ctrl-C to stop)")
	fmt.Println("  alarms [24h]      list the alarms the camera logged in the last period")
	fmt.Println("  watch-alarms      print alarms as the camera logs them (Ctrl-C to stop)")
	fmt.Println("  frame <time> <file>  save the recorded JPEG at an RFC 3339 time")
}
