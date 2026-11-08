package hikvision

import (
	"context"
	"errors"
	"time"

	hikvisionv1 "github.com/joshp123/gohome/proto/gen/plugins/hikvision/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type service struct {
	hikvisionv1.UnimplementedHikvisionServiceServer
	client *Client
	stream *alertStream
}

func RegisterHikvisionService(server *grpc.Server, client *Client, stream *alertStream) {
	hikvisionv1.RegisterHikvisionServiceServer(server, &service{client: client, stream: stream})
}

func (s *service) configured() error {
	if s.client == nil {
		return status.Error(codes.FailedPrecondition, "hikvision client not configured")
	}
	return nil
}

func (s *service) GetCamera(ctx context.Context, _ *hikvisionv1.GetCameraRequest) (*hikvisionv1.GetCameraResponse, error) {
	if err := s.configured(); err != nil {
		return nil, err
	}
	device, err := s.client.DeviceInfo(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "read device info: %v", err)
	}
	supported, err := s.client.SupportedEvents(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "read capabilities: %v", err)
	}
	motion, err := s.client.MotionDetection(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "read motion detection: %v", err)
	}
	line, err := s.readSmart(ctx, s.client.LineDetection, s.client.LineDetectionCapabilities)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "read line crossing: %v", err)
	}
	intrusion, err := s.readSmart(ctx, s.client.FieldDetection, s.client.FieldDetectionCapabilities)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "read intrusion: %v", err)
	}

	return &hikvisionv1.GetCameraResponse{Camera: &hikvisionv1.Camera{
		Identity: &hikvisionv1.Identity{
			DeviceName:          device.DeviceName,
			Model:               device.Model,
			SerialNumber:        device.SerialNumber,
			MacAddress:          device.MacAddress,
			FirmwareVersion:     device.FirmwareVersion,
			FirmwareBuild:       device.FirmwareBuild,
			EncoderVersion:      device.EncoderVersion,
			HardwareVersion:     device.HardwareVersion,
			FirmwareVersionInfo: device.FirmwareVersionInfo,
		},
		SupportedEvents: supported,
		Motion: &hikvisionv1.MotionDetection{
			Enabled:     motion.Enabled,
			Sensitivity: motion.Sensitivity,
			Targets:     parseTargetOptions(motion.TargetType),
		},
		LineCrossing: line,
		Intrusion:    intrusion,
		AlertStream:  alertStreamStatusProto(s.stream.Status()),
	}}, nil
}

func (s *service) readSmart(ctx context.Context,
	settings, capabilities func(context.Context) (smartDetectionXML, error)) (*hikvisionv1.SmartDetection, error) {
	current, err := settings(ctx)
	if err != nil {
		return nil, err
	}
	caps, err := capabilities(ctx)
	if err != nil {
		return nil, err
	}
	smart := &hikvisionv1.SmartDetection{Enabled: current.Enabled}
	if capRules := caps.rules(); len(capRules) > 0 {
		smart.SupportedTargets = parseTargetOptions(capRules[0].Target.Options)
	}
	for _, rule := range current.rules() {
		smart.Rules = append(smart.Rules, &hikvisionv1.SmartRule{
			Id:          rule.ID,
			Enabled:     rule.Enabled,
			Sensitivity: rule.Sensitivity,
			Target:      parseTarget(rule.Target.Value),
		})
	}
	return smart, nil
}

func (s *service) GetSnapshot(ctx context.Context, _ *hikvisionv1.GetSnapshotRequest) (*hikvisionv1.GetSnapshotResponse, error) {
	if err := s.configured(); err != nil {
		return nil, err
	}
	jpeg, err := s.client.Snapshot(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "snapshot: %v", err)
	}
	return &hikvisionv1.GetSnapshotResponse{Jpeg: jpeg, CapturedAt: timestamppb.Now()}, nil
}

func (s *service) WatchEvents(_ *hikvisionv1.WatchEventsRequest, server grpc.ServerStreamingServer[hikvisionv1.CameraEvent]) error {
	if err := s.configured(); err != nil {
		return err
	}
	sub, unsubscribe, err := s.stream.Subscribe()
	if err != nil {
		return status.Error(codes.Unavailable, err.Error())
	}
	defer unsubscribe()
	for {
		select {
		case <-server.Context().Done():
			return nil
		case event, open := <-sub.events:
			if !open {
				code := codes.Unavailable
				if errors.Is(sub.ended, errSubscriberBehind) {
					code = codes.ResourceExhausted
				}
				return status.Errorf(code, "%v; reconnect", sub.ended)
			}
			if err := server.Send(cameraEventProto(event)); err != nil {
				return err
			}
		}
	}
}

func cameraEventProto(event cameraEvent) *hikvisionv1.CameraEvent {
	return &hikvisionv1.CameraEvent{
		Kind:            event.Kind,
		State:           event.State,
		Target:          event.Target,
		CameraTime:      optionalTimestamp(event.CameraTime),
		ReceivedAt:      timestamppb.New(event.ReceivedAt),
		ChannelId:       event.ChannelID,
		RuleId:          event.RuleID,
		CameraEventType: event.EventType,
		CameraTarget:    event.CameraWords,
	}
}

func alertStreamStatusProto(status AlertStreamStatus) *hikvisionv1.AlertStreamStatus {
	return &hikvisionv1.AlertStreamStatus{
		Connected:      status.Connected,
		ConnectedSince: optionalTimestamp(status.ConnectedSince),
		LastMessage:    optionalTimestamp(status.LastMessage),
		LastError:      status.LastError,
	}
}

func optionalTimestamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}
