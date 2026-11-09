package hikvision

import (
	"context"
	"errors"
	"time"

	hikvisionv1 "github.com/joshp123/gohome/proto/gen/plugins/hikvision/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *service) ListAlarms(ctx context.Context, req *hikvisionv1.ListAlarmsRequest) (*hikvisionv1.ListAlarmsResponse, error) {
	if err := s.configured(); err != nil {
		return nil, err
	}
	if req.GetSince() == nil {
		return nil, status.Error(codes.InvalidArgument, "since is required")
	}
	until := time.Now()
	if req.GetUntil() != nil {
		until = req.GetUntil().AsTime()
	}
	since := req.GetSince().AsTime()
	if !since.Before(until) {
		return nil, status.Error(codes.InvalidArgument, "since must be before until")
	}
	alarms, err := s.client.ListAlarms(ctx, since, until)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "read alarm log: %v", err)
	}
	response := &hikvisionv1.ListAlarmsResponse{}
	for _, a := range alarms {
		response.Alarms = append(response.Alarms, alarmProto(a))
	}
	return response, nil
}

func (s *service) WatchAlarms(_ *hikvisionv1.WatchAlarmsRequest, server grpc.ServerStreamingServer[hikvisionv1.Alarm]) error {
	if err := s.configured(); err != nil {
		return err
	}
	sub := s.alarms.Subscribe()
	defer s.alarms.Unsubscribe(sub)
	for {
		select {
		case <-server.Context().Done():
			return nil
		case a, open := <-sub.alarms:
			if !open {
				// The only reason a subscription ends on its own.
				return status.Errorf(codes.ResourceExhausted, "%v; reconnect", sub.ended)
			}
			if err := server.Send(alarmProto(a)); err != nil {
				return err
			}
		}
	}
}

func (s *service) GetFrame(ctx context.Context, req *hikvisionv1.GetFrameRequest) (*hikvisionv1.GetFrameResponse, error) {
	if err := s.configured(); err != nil {
		return nil, err
	}
	if req.GetAt() == nil {
		return nil, status.Error(codes.InvalidArgument, "at is required")
	}
	jpeg, at, err := s.frames.Frame(ctx, req.GetAt().AsTime())
	switch {
	case errors.Is(err, errNoRecording):
		return nil, status.Error(codes.NotFound, err.Error())
	case errors.Is(err, errNoFFmpeg):
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	case err != nil:
		return nil, status.Errorf(codes.Unavailable, "recorded picture: %v", err)
	}
	return &hikvisionv1.GetFrameResponse{Jpeg: jpeg, At: timestamppb.New(at)}, nil
}

func alarmProto(a alarm) *hikvisionv1.Alarm {
	message := &hikvisionv1.Alarm{
		Kind:            a.Kind,
		State:           a.state(),
		ChannelId:       a.Channel,
		StartedAt:       timestamppb.New(a.Start),
		CameraAlarmType: a.LogType,
		SeenAt:          optionalTimestamp(a.SeenStart),
	}
	if !a.End.IsZero() {
		message.EndedAt = timestamppb.New(a.End)
		message.Duration = durationpb.New(a.End.Sub(a.Start))
		message.SeenAt = optionalTimestamp(a.SeenEnd)
	}
	return message
}
