package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/joshp123/gohome/internal/render"
	hikvisionv1 "github.com/joshp123/gohome/proto/gen/plugins/hikvision/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func hikvisionCmd(ctx context.Context, conn *grpc.ClientConn, out render.Renderer, args []string) int {
	if len(args) == 0 {
		writeHikvisionUsage(out)
		return render.ExitUsageError
	}

	client := hikvisionv1.NewHikvisionServiceClient(conn)
	switch args[0] {
	case "camera":
		resp, err := client.GetCamera(ctx, &hikvisionv1.GetCameraRequest{})
		if err != nil {
			out.WriteError("hikvision camera", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		_ = out.WriteProto(resp.GetCamera())
		return render.ExitOK

	case "snapshot":
		if len(args) < 2 {
			out.WriteError("hikvision snapshot", fmt.Errorf("missing output file, e.g. snapshot camera.jpg"), render.ExitUsageError)
			return render.ExitUsageError
		}
		resp, err := client.GetSnapshot(ctx, &hikvisionv1.GetSnapshotRequest{})
		if err != nil {
			out.WriteError("hikvision snapshot", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if err := os.WriteFile(args[1], resp.GetJpeg(), 0o600); err != nil {
			out.WriteError("hikvision snapshot", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		_ = out.WriteLine(fmt.Sprintf("wrote %s (%d bytes, captured %s)", args[1], len(resp.GetJpeg()),
			resp.GetCapturedAt().AsTime().Local().Format(time.RFC3339)))
		return render.ExitOK

	case "events":
		watchCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		stream, err := client.WatchEvents(watchCtx, &hikvisionv1.WatchEventsRequest{})
		if err != nil {
			out.WriteError("hikvision events", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		for {
			event, err := stream.Recv()
			if err != nil {
				if watchCtx.Err() != nil {
					return render.ExitOK
				}
				out.WriteError("hikvision events", err, render.ExitRuntimeError)
				return render.ExitRuntimeError
			}
			if out.Format == render.FormatJSON {
				_ = out.WriteProtoCompact(event)
				continue
			}
			_ = out.WriteLine(fmt.Sprintf("%s\t%s\t%s\t%s",
				event.GetReceivedAt().AsTime().Local().Format(time.RFC3339),
				enumWord(event.GetKind().String(), "EVENT_KIND_"),
				enumWord(event.GetState().String(), "EVENT_STATE_"),
				enumWord(event.GetTarget().String(), "DETECTION_TARGET_")))
		}

	case "alarms":
		since := 24 * time.Hour
		if len(args) > 1 {
			parsed, err := time.ParseDuration(args[1])
			if err != nil {
				out.WriteError("hikvision alarms", fmt.Errorf("since must be a duration such as 24h: %w", err), render.ExitUsageError)
				return render.ExitUsageError
			}
			since = parsed
		}
		resp, err := client.ListAlarms(ctx, &hikvisionv1.ListAlarmsRequest{Since: timestamppb.New(time.Now().Add(-since))})
		if err != nil {
			out.WriteError("hikvision alarms", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteProto(resp)
			return render.ExitOK
		}
		rows := make([][]string, 0, len(resp.GetAlarms()))
		for _, alarm := range resp.GetAlarms() {
			rows = append(rows, alarmRow(alarm))
		}
		_ = out.WriteTable([]string{"STARTED", "KIND", "STATE", "DURATION"}, rows)
		return render.ExitOK

	case "watch-alarms":
		watchCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		stream, err := client.WatchAlarms(watchCtx, &hikvisionv1.WatchAlarmsRequest{})
		if err != nil {
			out.WriteError("hikvision watch-alarms", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		for {
			alarm, err := stream.Recv()
			if err != nil {
				if watchCtx.Err() != nil {
					return render.ExitOK
				}
				out.WriteError("hikvision watch-alarms", err, render.ExitRuntimeError)
				return render.ExitRuntimeError
			}
			if out.Format == render.FormatJSON {
				_ = out.WriteProtoCompact(alarm)
				continue
			}
			_ = out.WriteLine(strings.Join(alarmRow(alarm), "\t"))
		}

	case "frame":
		if len(args) < 3 {
			out.WriteError("hikvision frame", fmt.Errorf("usage: frame <RFC 3339 time> <file.jpg>"), render.ExitUsageError)
			return render.ExitUsageError
		}
		at, err := time.Parse(time.RFC3339Nano, args[1])
		if err != nil {
			out.WriteError("hikvision frame", err, render.ExitUsageError)
			return render.ExitUsageError
		}
		frameCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		resp, err := client.GetFrame(frameCtx, &hikvisionv1.GetFrameRequest{At: timestamppb.New(at)})
		if err != nil {
			out.WriteError("hikvision frame", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if err := os.WriteFile(args[2], resp.GetJpeg(), 0o600); err != nil {
			out.WriteError("hikvision frame", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		_ = out.WriteLine(fmt.Sprintf("wrote %s (%d bytes, recorded %s)", args[2], len(resp.GetJpeg()),
			resp.GetAt().AsTime().Local().Format(time.RFC3339)))
		return render.ExitOK

	default:
		writeHikvisionUsage(out)
		return render.ExitUsageError
	}
}

func alarmRow(alarm *hikvisionv1.Alarm) []string {
	duration := "under way"
	if alarm.GetDuration() != nil {
		duration = alarm.GetDuration().AsDuration().String()
	}
	return []string{
		alarm.GetStartedAt().AsTime().Local().Format(time.RFC3339),
		enumWord(alarm.GetKind().String(), "EVENT_KIND_"),
		enumWord(alarm.GetState().String(), "EVENT_STATE_"),
		duration,
	}
}

func enumWord(name, prefix string) string {
	return strings.ToLower(strings.TrimPrefix(name, prefix))
}

func writeHikvisionUsage(out render.Renderer) {
	out.WriteHelp("gohome-cli hikvision <command>", []render.HelpSection{
		{
			Heading: "Commands",
			Lines: []string{
				"camera               identity, firmware, detection settings, event-stream status",
				"snapshot <file>      save one JPEG from the main stream",
				"events               print raw alert-stream events as they arrive",
				"alarms [24h]         list the alarms the camera logged in the last period",
				"watch-alarms         print alarms as the camera logs them",
				"frame <time> <file>  save the recorded JPEG at an RFC 3339 time",
			},
		},
	})
}
