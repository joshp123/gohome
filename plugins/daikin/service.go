package daikin

import (
	context "context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	daikinv1 "github.com/joshp123/gohome/proto/gen/plugins/daikin/v1"
)

type service struct {
	daikinv1.UnimplementedDaikinServiceServer
	client *Client
}

func RegisterDaikinService(server *grpc.Server, client *Client) {
	daikinv1.RegisterDaikinServiceServer(server, &service{client: client})
}

func (s *service) ListUnits(ctx context.Context, _ *daikinv1.ListUnitsRequest) (*daikinv1.ListUnitsResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "daikin client not configured")
	}

	devices, err := s.client.Devices(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list units: %v", err)
	}

	resp := &daikinv1.ListUnitsResponse{}
	for _, device := range devices {
		resp.Units = append(resp.Units, &daikinv1.Unit{
			Id:               device.ID,
			Name:             device.Name,
			Model:            device.Model,
			ClimateControlId: device.ClimateControlID,
		})
	}

	return resp, nil
}

func (s *service) GetUnitState(ctx context.Context, req *daikinv1.GetUnitStateRequest) (*daikinv1.GetUnitStateResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "daikin client not configured")
	}
	if req.GetUnitId() == "" {
		return nil, status.Error(codes.InvalidArgument, "unit_id is required")
	}

	payload, err := s.client.DeviceStateJSON(ctx, req.UnitId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get unit state: %v", err)
	}

	return &daikinv1.GetUnitStateResponse{Json: payload}, nil
}

func (s *service) SetOnOff(ctx context.Context, req *daikinv1.SetOnOffRequest) (*daikinv1.SetOnOffResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "daikin client not configured")
	}
	if req.GetUnitId() == "" {
		return nil, status.Error(codes.InvalidArgument, "unit_id is required")
	}
	if req.GetOnOffMode() == "" {
		return nil, status.Error(codes.InvalidArgument, "on_off_mode is required")
	}

	embeddedID, err := s.resolveEmbeddedID(ctx, req.UnitId, req.ClimateControlId)
	if err != nil {
		return nil, err
	}

	if err := s.client.SetOnOff(ctx, req.UnitId, embeddedID, req.OnOffMode); err != nil {
		return nil, status.Errorf(codes.Internal, "set on/off: %v", err)
	}

	return &daikinv1.SetOnOffResponse{}, nil
}

func (s *service) SetOperationMode(ctx context.Context, req *daikinv1.SetOperationModeRequest) (*daikinv1.SetOperationModeResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "daikin client not configured")
	}
	if req.GetUnitId() == "" {
		return nil, status.Error(codes.InvalidArgument, "unit_id is required")
	}
	if req.GetOperationMode() == "" {
		return nil, status.Error(codes.InvalidArgument, "operation_mode is required")
	}

	embeddedID, err := s.resolveEmbeddedID(ctx, req.UnitId, req.ClimateControlId)
	if err != nil {
		return nil, err
	}

	if err := s.client.SetOperationMode(ctx, req.UnitId, embeddedID, req.OperationMode); err != nil {
		return nil, status.Errorf(codes.Internal, "set operation mode: %v", err)
	}

	return &daikinv1.SetOperationModeResponse{}, nil
}

func (s *service) SetTemperature(ctx context.Context, req *daikinv1.SetTemperatureRequest) (*daikinv1.SetTemperatureResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "daikin client not configured")
	}
	if req.GetUnitId() == "" {
		return nil, status.Error(codes.InvalidArgument, "unit_id is required")
	}
	if req.GetOperationMode() == "" {
		return nil, status.Error(codes.InvalidArgument, "operation_mode is required")
	}
	if req.GetSetpoint() == "" {
		return nil, status.Error(codes.InvalidArgument, "setpoint is required")
	}

	embeddedID, err := s.resolveEmbeddedID(ctx, req.UnitId, req.ClimateControlId)
	if err != nil {
		return nil, err
	}

	if err := s.client.SetTemperature(ctx, req.UnitId, embeddedID, req.OperationMode, req.Setpoint, req.TemperatureCelsius); err != nil {
		return nil, status.Errorf(codes.Internal, "set temperature: %v", err)
	}

	return &daikinv1.SetTemperatureResponse{}, nil
}

func (s *service) resolveEmbeddedID(ctx context.Context, unitID, provided string) (string, error) {
	if provided != "" {
		return provided, nil
	}

	embeddedID, err := s.client.resolveClimateControlID(ctx, unitID)
	if err != nil {
		return "", status.Errorf(codes.InvalidArgument, "resolve climate_control_id: %v", err)
	}
	return embeddedID, nil
}
