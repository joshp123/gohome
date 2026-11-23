package weheat

import (
	"context"
	"encoding/json"

	weheatv1 "github.com/joshp123/gohome/proto/gen/plugins/weheat/v1"
	weheatapi "github.com/joshp123/weheat-golang"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type service struct {
	weheatv1.UnimplementedWeheatServiceServer
	client *Client
}

func RegisterWeheatService(server *grpc.Server, client *Client) {
	weheatv1.RegisterWeheatServiceServer(server, &service{client: client})
}

func (s *service) ListHeatPumps(ctx context.Context, req *weheatv1.ListHeatPumpsRequest) (*weheatv1.ListHeatPumpsResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "weheat client not configured")
	}
	var state *weheatapi.DeviceState
	if req != nil && req.State != nil {
		value := weheatapi.DeviceState(req.GetState())
		state = &value
	}
	pumps, err := s.client.ListHeatPumps(ctx, state)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list heat pumps: %v", err)
	}

	resp := &weheatv1.ListHeatPumpsResponse{}
	for _, pump := range pumps {
		resp.HeatPumps = append(resp.HeatPumps, toHeatPump(pump))
	}
	return resp, nil
}

func (s *service) GetHeatPump(ctx context.Context, req *weheatv1.GetHeatPumpRequest) (*weheatv1.GetHeatPumpResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "weheat client not configured")
	}
	pump, err := s.client.HeatPump(ctx, req.GetHeatPumpId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get heat pump: %v", err)
	}
	return &weheatv1.GetHeatPumpResponse{HeatPump: toHeatPumpDetail(pump)}, nil
}

func (s *service) GetLatestTelemetry(ctx context.Context, req *weheatv1.GetLatestTelemetryRequest) (*weheatv1.GetLatestTelemetryResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "weheat client not configured")
	}
	heatPumpID := req.GetHeatPumpId()
	if heatPumpID == "" {
		pumps, err := s.client.ListHeatPumps(ctx, nil)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "list heat pumps: %v", err)
		}
		if len(pumps) == 0 {
			return nil, status.Error(codes.NotFound, "no heat pumps found")
		}
		heatPumpID = pumps[0].ID
	}
	entry, err := s.client.LatestLog(ctx, heatPumpID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get latest telemetry: %v", err)
	}
	telemetry := &weheatv1.HeatPumpTelemetry{
		HeatPumpId:           entry.HeatPumpID,
		Online:               entry.IsOnline,
		WaterOutCelsius:      entry.TWaterOut,
		WaterInCelsius:       entry.TWaterIn,
		WaterHouseInCelsius:  entry.TWaterHouseIn,
		WaterSetpointCelsius: entry.TThermostatSetpoint,
		OutsideAirCelsius:    entry.TAirIn,
		ExhaustAirCelsius:    entry.TAirOut,
		TankTopCelsius:       entry.T1,
		TankBottomCelsius:    entry.T2,
		TankTargetCelsius:    entry.DhwTargetTemperature,
		RoomCelsius:          entry.TRoom,
		RoomTargetCelsius:    entry.TRoomTarget,
		CompressorRpm:        entry.RPM,
		WaterPumpActive:      entry.ControlBridgeStatusDecodedWaterPump,
		DhwValveActive:       entry.ControlBridgeStatusDecodedDHWValve,
		ElectricHeaterActive: entry.ControlBridgeStatusDecodedElectricHeater,
	}
	if !entry.Timestamp.IsZero() {
		telemetry.ObservedAt = timestamppb.New(entry.Timestamp)
	}
	if entry.State != nil {
		v := int32(*entry.State)
		telemetry.OperatingStateCode = &v
	}
	if entry.CMMassPowerIn != nil {
		v := int32(*entry.CMMassPowerIn)
		telemetry.ElectricityInWatts = &v
	}
	if entry.CMMassPowerOut != nil {
		v := int32(*entry.CMMassPowerOut)
		telemetry.HeatOutWatts = &v
	}
	if entry.CentralHeatingFlow != nil && *entry.CentralHeatingFlow < 255 {
		v := int32(*entry.CentralHeatingFlow)
		telemetry.CentralHeatingFlowPercent = &v
	}
	if entry.DHWFlow != nil && *entry.DHWFlow < 255 {
		v := int32(*entry.DHWFlow)
		telemetry.DhwFlowPercent = &v
	}
	if entry.Error != nil {
		v := int32(*entry.Error)
		telemetry.ErrorCode = &v
	}
	return &weheatv1.GetLatestTelemetryResponse{Telemetry: telemetry}, nil
}

func (s *service) GetEnergySummary(ctx context.Context, req *weheatv1.GetEnergySummaryRequest) (*weheatv1.GetEnergySummaryResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "weheat client not configured")
	}
	heatPumpID := req.GetHeatPumpId()
	if heatPumpID == "" {
		pumps, err := s.client.ListHeatPumps(ctx, nil)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "list heat pumps: %v", err)
		}
		if len(pumps) == 0 {
			return nil, status.Error(codes.NotFound, "no heat pumps found")
		}
		heatPumpID = pumps[0].ID
	}
	totals, err := s.client.EnergyTotals(ctx, heatPumpID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get energy summary: %v", err)
	}
	return &weheatv1.GetEnergySummaryResponse{
		Summary: &weheatv1.HeatPumpEnergySummary{
			HeatPumpId:                   totals.HeatPumpID,
			ElectricityInHeatingKwh:      totals.TotalEInHeating,
			ElectricityInDhwKwh:          totals.TotalEInDHW,
			ElectricityInStandbyKwh:      totals.TotalEInStandby,
			ElectricityInHeatingDefrostKwh: totals.TotalEInHeatingDefrost,
			ElectricityInDhwDefrostKwh:   totals.TotalEInDHWDefrost,
			ElectricityInCoolingKwh:      totals.TotalEInCooling,
			HeatOutHeatingKwh:            totals.TotalEOutHeating,
			HeatOutDhwKwh:                totals.TotalEOutDHW,
			HeatOutHeatingDefrostKwh:     totals.TotalEOutHeatingDefrost,
			HeatOutDhwDefrostKwh:         totals.TotalEOutDHWDefrost,
			HeatOutCoolingKwh:            totals.TotalEOutCooling,
		},
	}, nil
}

func (s *service) GetLatestLog(ctx context.Context, req *weheatv1.GetLatestLogRequest) (*weheatv1.GetLatestLogResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "weheat client not configured")
	}
	log, err := s.client.LatestLog(ctx, req.GetHeatPumpId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get latest log: %v", err)
	}
	payload, err := json.Marshal(log)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal latest log: %v", err)
	}
	return &weheatv1.GetLatestLogResponse{Json: string(payload)}, nil
}

func (s *service) GetRawLogs(ctx context.Context, req *weheatv1.GetRawLogsRequest) (*weheatv1.GetRawLogsResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "weheat client not configured")
	}
	query := logQueryFromProto(req.GetQuery())
	logs, err := s.client.RawLogs(ctx, req.GetHeatPumpId(), query)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get raw logs: %v", err)
	}
	resp := &weheatv1.GetRawLogsResponse{}
	for _, entry := range logs {
		payload, err := json.Marshal(entry)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "marshal raw log: %v", err)
		}
		resp.Json = append(resp.Json, string(payload))
	}
	return resp, nil
}

func (s *service) GetLogViews(ctx context.Context, req *weheatv1.GetLogViewsRequest) (*weheatv1.GetLogViewsResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "weheat client not configured")
	}
	query := logQueryFromProto(req.GetQuery())
	views, err := s.client.LogViews(ctx, req.GetHeatPumpId(), query)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get log views: %v", err)
	}
	resp := &weheatv1.GetLogViewsResponse{}
	for _, entry := range views {
		payload, err := json.Marshal(entry)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "marshal log view: %v", err)
		}
		resp.Json = append(resp.Json, string(payload))
	}
	return resp, nil
}

func (s *service) GetEnergyTotals(ctx context.Context, req *weheatv1.GetEnergyTotalsRequest) (*weheatv1.GetEnergyTotalsResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "weheat client not configured")
	}
	energy, err := s.client.EnergyTotals(ctx, req.GetHeatPumpId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get energy totals: %v", err)
	}
	payload, err := json.Marshal(energy)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal energy totals: %v", err)
	}
	return &weheatv1.GetEnergyTotalsResponse{Json: string(payload)}, nil
}

func (s *service) GetEnergyLogs(ctx context.Context, req *weheatv1.GetEnergyLogsRequest) (*weheatv1.GetEnergyLogsResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "weheat client not configured")
	}
	query := energyQueryFromProto(req.GetQuery())
	logs, err := s.client.EnergyLogs(ctx, req.GetHeatPumpId(), query)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get energy logs: %v", err)
	}
	resp := &weheatv1.GetEnergyLogsResponse{}
	for _, entry := range logs {
		payload, err := json.Marshal(entry)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "marshal energy log: %v", err)
		}
		resp.Json = append(resp.Json, string(payload))
	}
	return resp, nil
}

func logQueryFromProto(query *weheatv1.LogQuery) weheatapi.LogQuery {
	if query == nil {
		return weheatapi.LogQuery{}
	}
	out := weheatapi.LogQuery{Interval: weheatapi.LogInterval(query.GetInterval())}
	if query.StartTime != nil {
		t := query.StartTime.AsTime()
		out.StartTime = &t
	}
	if query.EndTime != nil {
		t := query.EndTime.AsTime()
		out.EndTime = &t
	}
	return out
}

func energyQueryFromProto(query *weheatv1.EnergyQuery) weheatapi.EnergyLogQuery {
	if query == nil {
		return weheatapi.EnergyLogQuery{}
	}
	out := weheatapi.EnergyLogQuery{Interval: weheatapi.EnergyInterval(query.GetInterval())}
	if query.StartTime != nil {
		t := query.StartTime.AsTime()
		out.StartTime = &t
	}
	if query.EndTime != nil {
		t := query.EndTime.AsTime()
		out.EndTime = &t
	}
	return out
}

func toHeatPump(pump weheatapi.ReadAllHeatPump) *weheatv1.HeatPump {
	proto := &weheatv1.HeatPump{
		HeatPumpId:      pump.ID,
		Name:            pump.Name,
		SerialNumber:    pump.SerialNumber,
		FirmwareVersion: pump.FirmwareVersion,
	}
	if pump.Model != nil {
		model := int32(*pump.Model)
		proto.Model = &model
		if name := weheatapi.HeatPumpModelName(*pump.Model); name != "" {
			proto.ModelName = &name
		}
	}
	state := int32(pump.State)
	proto.State = &state
	if pump.Status != nil {
		status := int32(*pump.Status)
		proto.Status = &status
	}
	if pump.DHWType != nil {
		dhw := int32(*pump.DHWType)
		proto.DhwType = &dhw
		value := *pump.DHWType == weheatapi.DhwTypeAvailable
		proto.HasDhw = &value
	}
	if pump.BoilerType != nil {
		boiler := int32(*pump.BoilerType)
		proto.BoilerType = &boiler
	}
	return proto
}

func toHeatPumpDetail(pump *weheatapi.ReadHeatPump) *weheatv1.HeatPump {
	if pump == nil {
		return nil
	}
	proto := &weheatv1.HeatPump{
		HeatPumpId:   pump.ID,
		Name:         pump.Name,
		SerialNumber: pump.SerialNumber,
	}
	if pump.Model != nil {
		model := int32(*pump.Model)
		proto.Model = &model
		if name := weheatapi.HeatPumpModelName(*pump.Model); name != "" {
			proto.ModelName = &name
		}
	}
	state := int32(pump.State)
	proto.State = &state
	if pump.Status != nil {
		status := int32(*pump.Status)
		proto.Status = &status
	}
	if pump.DHWType != nil {
		dhw := int32(*pump.DHWType)
		proto.DhwType = &dhw
		value := *pump.DHWType == weheatapi.DhwTypeAvailable
		proto.HasDhw = &value
	}
	if pump.BoilerType != nil {
		boiler := int32(*pump.BoilerType)
		proto.BoilerType = &boiler
	}
	return proto
}
