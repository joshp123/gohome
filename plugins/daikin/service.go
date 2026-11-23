package daikin

import (
	context "context"
	"sort"

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

func (s *service) GetUnitStatus(ctx context.Context, req *daikinv1.GetUnitStatusRequest) (*daikinv1.GetUnitStatusResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "daikin client not configured")
	}
	states, err := s.client.DeviceStates(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get unit status: %v", err)
	}
	var matched *DeviceState
	targetID := req.GetUnitId()
	for i := range states {
		if targetID == "" || states[i].Device.ID == targetID {
			matched = &states[i]
			break
		}
	}
	if matched == nil {
		return nil, status.Errorf(codes.NotFound, "unit %q not found", targetID)
	}
	out := &daikinv1.ClimateUnitStatus{
		UnitId:           matched.Device.ID,
		Name:             matched.Device.Name,
		Model:            matched.Device.Model,
		ClimateControlId: matched.Device.ClimateControlID,
		CloudConnected:   matched.Device.CloudConnected,
	}
	for _, mp := range matched.ManagementPoints {
		if mp.ManagementPointType != "climateControl" && mp.ManagementPointType != "climateControlMainZone" {
			continue
		}
		if mp.OnOffMode != nil && mp.OnOffMode.Value != "" {
			v := mp.OnOffMode.Value
			out.OnOffMode = &v
		}
		if mp.OperationMode != nil && mp.OperationMode.Value != "" {
			v := mp.OperationMode.Value
			out.OperationMode = &v
		}
		if mp.IsInErrorState != nil {
			v := mp.IsInErrorState.Value
			out.ErrorState = &v
		}
		if mp.IsInWarningState != nil {
			v := mp.IsInWarningState.Value
			out.WarningState = &v
		}
		if mp.IsHolidayModeActive != nil {
			v := mp.IsHolidayModeActive.Value
			out.HolidayModeActive = &v
		}
		if mp.SensoryData != nil {
			if m, ok := mp.SensoryData.Value["roomTemperature"]; ok {
				v := m.Value
				out.RoomTemperatureCelsius = &v
			}
			if m, ok := mp.SensoryData.Value["outdoorTemperature"]; ok {
				v := m.Value
				out.OutdoorTemperatureCelsius = &v
			}
			if m, ok := mp.SensoryData.Value["roomHumidity"]; ok {
				v := m.Value
				out.RoomHumidityPercent = &v
			}
		}
		if mp.TemperatureControl != nil {
			modes := make([]string, 0, len(mp.TemperatureControl.Value.OperationModes))
			for mode := range mp.TemperatureControl.Value.OperationModes {
				modes = append(modes, mode)
			}
			sort.Strings(modes)
			for _, mode := range modes {
				keys := make([]string, 0, len(mp.TemperatureControl.Value.OperationModes[mode].Setpoints))
				for k := range mp.TemperatureControl.Value.OperationModes[mode].Setpoints {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					sp := mp.TemperatureControl.Value.OperationModes[mode].Setpoints[k]
					out.Setpoints = append(out.Setpoints, &daikinv1.OperationModeSetpoint{
						OperationMode:      mode,
						SetpointKey:        k,
						TemperatureCelsius: sp.Value,
					})
				}
			}
		}
		break
	}
	return &daikinv1.GetUnitStatusResponse{Status: out}, nil
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
