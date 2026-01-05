package roborock

import (
	context "context"
	"errors"
	"fmt"
	"time"

	roborockv1 "github.com/joshp123/gohome/proto/gen/plugins/roborock/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type service struct {
	roborockv1.UnimplementedRoborockServiceServer
	client *Client
}

func RegisterRoborockService(server *grpc.Server, client *Client) {
	roborockv1.RegisterRoborockServiceServer(server, &service{client: client})
}

func (s *service) ListDevices(ctx context.Context, _ *roborockv1.ListDevicesRequest) (*roborockv1.ListDevicesResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "roborock client not configured")
	}

	devices, err := s.client.Devices(ctx)
	if err != nil {
		return nil, mapClientError("list devices", err)
	}

	resp := &roborockv1.ListDevicesResponse{}
	for _, device := range devices {
		resp.Devices = append(resp.Devices, &roborockv1.Device{
			Id:          device.ID,
			Name:        device.Name,
			Model:       device.Model,
			Firmware:    device.Firmware,
			SupportsMop: device.SupportsMop,
		})
	}

	return resp, nil
}

func (s *service) GetStatus(ctx context.Context, req *roborockv1.DeviceStatusRequest) (*roborockv1.GetStatusResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "roborock client not configured")
	}
	if req.GetDeviceId() == "" {
		return nil, status.Error(codes.InvalidArgument, "device_id is required")
	}

	statusData, err := s.client.Status(ctx, req.GetDeviceId())
	if err != nil {
		return nil, mapClientError("get status", err)
	}

	return &roborockv1.GetStatusResponse{Status: mapStatus(statusData)}, nil
}

func (s *service) StartClean(ctx context.Context, req *roborockv1.StartCleanRequest) (*roborockv1.StartCleanResponse, error) {
	if err := s.requireDevice(req.GetDeviceId()); err != nil {
		return nil, err
	}
	if err := s.client.StartClean(ctx, req.GetDeviceId()); err != nil {
		return nil, mapClientError("start clean", err)
	}
	return &roborockv1.StartCleanResponse{}, nil
}

func (s *service) Pause(ctx context.Context, req *roborockv1.PauseRequest) (*roborockv1.PauseResponse, error) {
	if err := s.requireDevice(req.GetDeviceId()); err != nil {
		return nil, err
	}
	if err := s.client.Pause(ctx, req.GetDeviceId()); err != nil {
		return nil, mapClientError("pause", err)
	}
	return &roborockv1.PauseResponse{}, nil
}

func (s *service) Stop(ctx context.Context, req *roborockv1.StopRequest) (*roborockv1.StopResponse, error) {
	if err := s.requireDevice(req.GetDeviceId()); err != nil {
		return nil, err
	}
	if err := s.client.Stop(ctx, req.GetDeviceId()); err != nil {
		return nil, mapClientError("stop", err)
	}
	return &roborockv1.StopResponse{}, nil
}

func (s *service) Dock(ctx context.Context, req *roborockv1.DockRequest) (*roborockv1.DockResponse, error) {
	if err := s.requireDevice(req.GetDeviceId()); err != nil {
		return nil, err
	}
	if err := s.client.Dock(ctx, req.GetDeviceId()); err != nil {
		return nil, mapClientError("dock", err)
	}
	return &roborockv1.DockResponse{}, nil
}

func (s *service) Locate(ctx context.Context, req *roborockv1.LocateRequest) (*roborockv1.LocateResponse, error) {
	if err := s.requireDevice(req.GetDeviceId()); err != nil {
		return nil, err
	}
	if err := s.client.Locate(ctx, req.GetDeviceId()); err != nil {
		return nil, mapClientError("locate", err)
	}
	return &roborockv1.LocateResponse{}, nil
}

func (s *service) SetFanSpeed(ctx context.Context, req *roborockv1.SetFanSpeedRequest) (*roborockv1.SetFanSpeedResponse, error) {
	if err := s.requireDevice(req.GetDeviceId()); err != nil {
		return nil, err
	}
	if req.GetFanSpeed() == "" {
		return nil, status.Error(codes.InvalidArgument, "fan_speed is required")
	}
	if err := s.client.SetFanSpeed(ctx, req.GetDeviceId(), req.GetFanSpeed()); err != nil {
		return nil, mapClientError("set fan speed", err)
	}
	return &roborockv1.SetFanSpeedResponse{}, nil
}

func (s *service) SetMopMode(ctx context.Context, req *roborockv1.SetMopModeRequest) (*roborockv1.SetMopModeResponse, error) {
	if err := s.requireDevice(req.GetDeviceId()); err != nil {
		return nil, err
	}
	if req.GetMopMode() == "" {
		return nil, status.Error(codes.InvalidArgument, "mop_mode is required")
	}
	if err := s.client.SetMopMode(ctx, req.GetDeviceId(), req.GetMopMode()); err != nil {
		return nil, mapClientError("set mop mode", err)
	}
	return &roborockv1.SetMopModeResponse{}, nil
}

func (s *service) SetMopIntensity(ctx context.Context, req *roborockv1.SetMopIntensityRequest) (*roborockv1.SetMopIntensityResponse, error) {
	if err := s.requireDevice(req.GetDeviceId()); err != nil {
		return nil, err
	}
	if req.GetMopIntensity() == "" {
		return nil, status.Error(codes.InvalidArgument, "mop_intensity is required")
	}
	if err := s.client.SetMopIntensity(ctx, req.GetDeviceId(), req.GetMopIntensity()); err != nil {
		return nil, mapClientError("set mop intensity", err)
	}
	return &roborockv1.SetMopIntensityResponse{}, nil
}

func (s *service) CleanZone(ctx context.Context, req *roborockv1.CleanZoneRequest) (*roborockv1.CleanZoneResponse, error) {
	if err := s.requireDevice(req.GetDeviceId()); err != nil {
		return nil, err
	}
	if len(req.GetZones()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "zones are required")
	}
	zones := make([]Zone, 0, len(req.GetZones()))
	for _, zone := range req.GetZones() {
		zones = append(zones, Zone{X1: int(zone.GetX1()), Y1: int(zone.GetY1()), X2: int(zone.GetX2()), Y2: int(zone.GetY2())})
	}
	if err := s.client.CleanZone(ctx, req.GetDeviceId(), zones, int(req.GetRepeats())); err != nil {
		return nil, mapClientError("clean zone", err)
	}
	return &roborockv1.CleanZoneResponse{}, nil
}

func (s *service) CleanSegment(ctx context.Context, req *roborockv1.CleanSegmentRequest) (*roborockv1.CleanSegmentResponse, error) {
	if err := s.requireDevice(req.GetDeviceId()); err != nil {
		return nil, err
	}
	if len(req.GetSegments()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "segments are required")
	}
	segments := make([]int, 0, len(req.GetSegments()))
	for _, seg := range req.GetSegments() {
		segments = append(segments, int(seg))
	}
	if err := s.client.CleanSegment(ctx, req.GetDeviceId(), segments, int(req.GetRepeats())); err != nil {
		return nil, mapClientError("clean segment", err)
	}
	return &roborockv1.CleanSegmentResponse{}, nil
}

func (s *service) ListSegments(ctx context.Context, req *roborockv1.ListSegmentsRequest) (*roborockv1.ListSegmentsResponse, error) {
	if err := s.requireDevice(req.GetDeviceId()); err != nil {
		return nil, err
	}
	segments, err := s.client.SegmentsSnapshot(ctx, req.GetDeviceId())
	if err != nil {
		return nil, mapClientError("list segments", err)
	}
	resp := &roborockv1.ListSegmentsResponse{Segments: make([]*roborockv1.Segment, 0, len(segments))}
	for _, seg := range segments {
		resp.Segments = append(resp.Segments, mapSegment(seg))
	}
	return resp, nil
}

func (s *service) GoTo(ctx context.Context, req *roborockv1.GoToRequest) (*roborockv1.GoToResponse, error) {
	if err := s.requireDevice(req.GetDeviceId()); err != nil {
		return nil, err
	}
	if err := s.client.GoTo(ctx, req.GetDeviceId(), int(req.GetX()), int(req.GetY())); err != nil {
		return nil, mapClientError("go to", err)
	}
	return &roborockv1.GoToResponse{}, nil
}

func (s *service) SetDnd(ctx context.Context, req *roborockv1.SetDndRequest) (*roborockv1.SetDndResponse, error) {
	if err := s.requireDevice(req.GetDeviceId()); err != nil {
		return nil, err
	}
	if req.GetStartTime() == "" || req.GetEndTime() == "" {
		return nil, status.Error(codes.InvalidArgument, "start_time and end_time are required")
	}
	if err := s.client.SetDND(ctx, req.GetDeviceId(), req.GetStartTime(), req.GetEndTime(), req.GetEnabled()); err != nil {
		return nil, mapClientError("set dnd", err)
	}
	return &roborockv1.SetDndResponse{}, nil
}

func (s *service) ResetConsumable(ctx context.Context, req *roborockv1.ResetConsumableRequest) (*roborockv1.ResetConsumableResponse, error) {
	if err := s.requireDevice(req.GetDeviceId()); err != nil {
		return nil, err
	}
	if req.GetConsumable() == "" {
		return nil, status.Error(codes.InvalidArgument, "consumable is required")
	}
	if err := s.client.ResetConsumable(ctx, req.GetDeviceId(), req.GetConsumable()); err != nil {
		return nil, mapClientError("reset consumable", err)
	}
	return &roborockv1.ResetConsumableResponse{}, nil
}

func (s *service) requireDevice(deviceID string) error {
	if s.client == nil {
		return status.Error(codes.FailedPrecondition, "roborock client not configured")
	}
	if deviceID == "" {
		return status.Error(codes.InvalidArgument, "device_id is required")
	}
	return nil
}

func mapClientError(action string, err error) error {
	if errors.Is(err, ErrNotImplemented) {
		return status.Errorf(codes.Unimplemented, "roborock %s not implemented", action)
	}
	return status.Errorf(codes.Internal, "%s: %v", action, err)
}

func mapStatus(statusData Status) *roborockv1.DeviceStatus {
	return &roborockv1.DeviceStatus{
		State:                         statusData.State,
		BatteryPercent:                uint32(statusData.BatteryPercent),
		ErrorCode:                     statusData.ErrorCode,
		ErrorMessage:                  statusData.ErrorMessage,
		CleaningAreaSquareMeters:      statusData.CleaningAreaSquareMeters,
		CleaningTimeSeconds:           uint32(statusData.CleaningTimeSeconds),
		TotalCleaningTimeSeconds:      uint32(statusData.TotalCleaningTimeSeconds),
		TotalCleaningAreaSquareMeters: statusData.TotalCleaningAreaSquareM,
		TotalCleaningCount:            uint32(statusData.TotalCleaningCount),
		FanSpeed:                      statusData.FanSpeed,
		MopMode:                       statusData.MopMode,
		MopIntensity:                  statusData.MopIntensity,
		WaterTankAttached:             statusData.WaterTankAttached,
		MopAttached:                   statusData.MopAttached,
		WaterShortage:                 statusData.WaterShortage,
		Charging:                      statusData.Charging,
		LastCleanStart:                formatTimestamp(statusData.LastCleanStart),
		LastCleanEnd:                  formatTimestamp(statusData.LastCleanEnd),
	}
}

func mapSegment(seg segmentSummary) *roborockv1.Segment {
	label := fmt.Sprintf("segment_%d", seg.id)
	if seg.label != "" {
		label = seg.label
	}
	return &roborockv1.Segment{
		Id:         uint32(seg.id),
		PixelCount: uint32(seg.pixelCount),
		CentroidX:  int32(seg.centroidX()),
		CentroidY:  int32(seg.centroidY()),
		MinX:       int32(seg.minX),
		MinY:       int32(seg.minY),
		MaxX:       int32(seg.maxX),
		MaxY:       int32(seg.maxY),
		Label:      label,
	}
}

func formatTimestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
