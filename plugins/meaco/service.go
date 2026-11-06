package meaco

import (
	"context"
	meacov1 "github.com/joshp123/gohome/proto/gen/plugins/meaco/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type service struct {
	meacov1.UnimplementedMeacoServiceServer
	client *Client
}

func (s *service) GetStatus(ctx context.Context, _ *meacov1.GetStatusRequest) (*meacov1.DeviceStatus, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "meaco client not configured")
	}
	state, err := s.client.Status(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "read meaco: %v", err)
	}
	return state, nil
}
func (s *service) set(ctx context.Context, id string, value any) (*meacov1.CommandAccepted, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "meaco client not configured")
	}
	// Never send a command for a field this device does not report.
	state, err := s.client.Status(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "read before control: %v", err)
	}
	if id == "1" && state.Power == nil || id == "2" && state.TargetHumidityPercent == nil || id == "4" && state.Mode == nil {
		return nil, status.Error(codes.FailedPrecondition, "device did not report this control")
	}
	if err = s.client.Set(ctx, id, value); err != nil {
		return nil, status.Errorf(codes.Unavailable, "control meaco: %v", err)
	}
	return &meacov1.CommandAccepted{}, nil
}
func (s *service) SetPower(ctx context.Context, req *meacov1.SetPowerRequest) (*meacov1.CommandAccepted, error) {
	return s.set(ctx, "1", req.Power)
}
func (s *service) SetTargetHumidity(ctx context.Context, req *meacov1.SetTargetHumidityRequest) (*meacov1.CommandAccepted, error) {
	if req.HumidityPercent < 35 || req.HumidityPercent > 70 || req.HumidityPercent%5 != 0 {
		return nil, status.Error(codes.InvalidArgument, "target humidity must be 35–70 in steps of 5")
	}
	return s.set(ctx, "2", req.HumidityPercent)
}
func (s *service) SetMode(ctx context.Context, req *meacov1.SetModeRequest) (*meacov1.CommandAccepted, error) {
	modes := map[meacov1.Mode]string{meacov1.Mode_MODE_MANUAL: "manual", meacov1.Mode_MODE_LAUNDRY: "laundry", meacov1.Mode_MODE_SLEEP: "sleep", meacov1.Mode_MODE_PURIFY: "purify"}
	mode, ok := modes[req.Mode]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "select manual, laundry, sleep or purify mode")
	}
	return s.set(ctx, "4", mode)
}
