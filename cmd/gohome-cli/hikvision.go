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
	default:
		hikvisionUsage()
		os.Exit(2)
	}
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
	fmt.Println("  events            print on-camera events as they arrive (Ctrl-C to stop)")
}
