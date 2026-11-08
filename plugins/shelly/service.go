package shelly

import (
	"context"

	shellyv1 "github.com/joshp123/gohome/proto/gen/plugins/shelly/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type service struct {
	shellyv1.UnimplementedShellyServiceServer
	client *Client
}

func RegisterShellyService(server *grpc.Server, client *Client) {
	shellyv1.RegisterShellyServiceServer(server, &service{client: client})
}

func (s *service) ListCovers(ctx context.Context, _ *shellyv1.ListCoversRequest) (*shellyv1.ListCoversResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "shelly plugin not configured")
	}
	readings := s.client.ReadAll(ctx)
	response := &shellyv1.ListCoversResponse{Covers: make([]*shellyv1.Cover, 0, len(readings))}
	for _, reading := range readings {
		cover := &shellyv1.Cover{
			DeviceId: reading.Cover.DeviceID,
			Name:     reading.Cover.Name,
			Address:  reading.Cover.Address,
		}
		if reading.Err != nil {
			cover.ReadError = reading.Err.Error()
		} else {
			cover.ReadAt = timestamppb.New(reading.ReadAt)
			cover.PositionPercent = optionalInt32(reading.PositionPercent)
			cover.TargetPositionPercent = optionalInt32(reading.TargetPositionPercent)
			cover.Moving = reading.MotorRunning
			cover.Calibrated = reading.Calibrated
			cover.State = reading.State
		}
		response.Covers = append(response.Covers, cover)
	}
	return response, nil
}

func optionalInt32(value *int) *int32 {
	if value == nil {
		return nil
	}
	v := int32(*value)
	return &v
}
