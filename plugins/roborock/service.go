package roborock

import (
	context "context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
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
	deviceID, err := s.requireDevice(ctx, req.GetDeviceId())
	if err != nil {
		return nil, err
	}

	statusData, err := s.client.Status(ctx, deviceID)
	if err != nil {
		return nil, mapClientError("get status", err)
	}

	return &roborockv1.GetStatusResponse{Status: mapStatus(statusData)}, nil
}

func (s *service) StartClean(ctx context.Context, req *roborockv1.StartCleanRequest) (*roborockv1.StartCleanResponse, error) {
	deviceID, err := s.requireDevice(ctx, req.GetDeviceId())
	if err != nil {
		return nil, err
	}
	if err := s.client.StartClean(ctx, deviceID); err != nil {
		return nil, mapClientError("start clean", err)
	}
	return &roborockv1.StartCleanResponse{}, nil
}

func (s *service) Pause(ctx context.Context, req *roborockv1.PauseRequest) (*roborockv1.PauseResponse, error) {
	deviceID, err := s.requireDevice(ctx, req.GetDeviceId())
	if err != nil {
		return nil, err
	}
	if err := s.client.Pause(ctx, deviceID); err != nil {
		return nil, mapClientError("pause", err)
	}
	return &roborockv1.PauseResponse{}, nil
}

func (s *service) Stop(ctx context.Context, req *roborockv1.StopRequest) (*roborockv1.StopResponse, error) {
	deviceID, err := s.requireDevice(ctx, req.GetDeviceId())
	if err != nil {
		return nil, err
	}
	if err := s.client.Stop(ctx, deviceID); err != nil {
		return nil, mapClientError("stop", err)
	}
	return &roborockv1.StopResponse{}, nil
}

func (s *service) Dock(ctx context.Context, req *roborockv1.DockRequest) (*roborockv1.DockResponse, error) {
	deviceID, err := s.requireDevice(ctx, req.GetDeviceId())
	if err != nil {
		return nil, err
	}
	if err := s.client.Dock(ctx, deviceID); err != nil {
		return nil, mapClientError("dock", err)
	}
	return &roborockv1.DockResponse{}, nil
}

func (s *service) Locate(ctx context.Context, req *roborockv1.LocateRequest) (*roborockv1.LocateResponse, error) {
	deviceID, err := s.requireDevice(ctx, req.GetDeviceId())
	if err != nil {
		return nil, err
	}
	if err := s.client.Locate(ctx, deviceID); err != nil {
		return nil, mapClientError("locate", err)
	}
	return &roborockv1.LocateResponse{}, nil
}

func (s *service) SetFanSpeed(ctx context.Context, req *roborockv1.SetFanSpeedRequest) (*roborockv1.SetFanSpeedResponse, error) {
	deviceID, err := s.requireDevice(ctx, req.GetDeviceId())
	if err != nil {
		return nil, err
	}
	if req.GetFanSpeed() == "" {
		return nil, status.Error(codes.InvalidArgument, "fan_speed is required")
	}
	if err := s.client.SetFanSpeed(ctx, deviceID, req.GetFanSpeed()); err != nil {
		return nil, mapClientError("set fan speed", err)
	}
	return &roborockv1.SetFanSpeedResponse{}, nil
}

func (s *service) SetMopMode(ctx context.Context, req *roborockv1.SetMopModeRequest) (*roborockv1.SetMopModeResponse, error) {
	deviceID, err := s.requireDevice(ctx, req.GetDeviceId())
	if err != nil {
		return nil, err
	}
	if req.GetMopMode() == "" {
		return nil, status.Error(codes.InvalidArgument, "mop_mode is required")
	}
	if err := s.client.SetMopMode(ctx, deviceID, req.GetMopMode()); err != nil {
		return nil, mapClientError("set mop mode", err)
	}
	return &roborockv1.SetMopModeResponse{}, nil
}

func (s *service) SetMopIntensity(ctx context.Context, req *roborockv1.SetMopIntensityRequest) (*roborockv1.SetMopIntensityResponse, error) {
	deviceID, err := s.requireDevice(ctx, req.GetDeviceId())
	if err != nil {
		return nil, err
	}
	if req.GetMopIntensity() == "" {
		return nil, status.Error(codes.InvalidArgument, "mop_intensity is required")
	}
	if err := s.client.SetMopIntensity(ctx, deviceID, req.GetMopIntensity()); err != nil {
		return nil, mapClientError("set mop intensity", err)
	}
	return &roborockv1.SetMopIntensityResponse{}, nil
}

func (s *service) CleanZone(ctx context.Context, req *roborockv1.CleanZoneRequest) (*roborockv1.CleanZoneResponse, error) {
	deviceID, err := s.requireDevice(ctx, req.GetDeviceId())
	if err != nil {
		return nil, err
	}
	if len(req.GetZones()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "zones are required")
	}
	zones := make([]Zone, 0, len(req.GetZones()))
	for _, zone := range req.GetZones() {
		zones = append(zones, Zone{X1: int(zone.GetX1()), Y1: int(zone.GetY1()), X2: int(zone.GetX2()), Y2: int(zone.GetY2())})
	}
	if err := s.client.CleanZone(ctx, deviceID, zones, int(req.GetRepeats())); err != nil {
		return nil, mapClientError("clean zone", err)
	}
	return &roborockv1.CleanZoneResponse{}, nil
}

func (s *service) CleanSegment(ctx context.Context, req *roborockv1.CleanSegmentRequest) (*roborockv1.CleanSegmentResponse, error) {
	deviceID, err := s.requireDevice(ctx, req.GetDeviceId())
	if err != nil {
		return nil, err
	}
	if len(req.GetSegments()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "segments are required")
	}
	segments := make([]int, 0, len(req.GetSegments()))
	for _, seg := range req.GetSegments() {
		segments = append(segments, int(seg))
	}
	if err := s.client.CleanSegment(ctx, deviceID, segments, int(req.GetRepeats())); err != nil {
		return nil, mapClientError("clean segment", err)
	}
	return &roborockv1.CleanSegmentResponse{}, nil
}

func (s *service) ListSegments(ctx context.Context, req *roborockv1.ListSegmentsRequest) (*roborockv1.ListSegmentsResponse, error) {
	deviceID, err := s.requireDevice(ctx, req.GetDeviceId())
	if err != nil {
		return nil, err
	}
	segments, err := s.client.SegmentsSnapshot(ctx, deviceID)
	if err != nil {
		return nil, mapClientError("list segments", err)
	}
	if len(segments) == 0 {
		log.Printf("roborock list segments: empty result (device_id=%s)", req.GetDeviceId())
	}
	resp := &roborockv1.ListSegmentsResponse{Segments: make([]*roborockv1.Segment, 0, len(segments))}
	for _, seg := range segments {
		resp.Segments = append(resp.Segments, mapSegment(seg))
	}
	return resp, nil
}

func (s *service) ListRooms(ctx context.Context, req *roborockv1.ListRoomsRequest) (*roborockv1.ListRoomsResponse, error) {
	deviceID, err := s.requireDevice(ctx, req.GetDeviceId())
	if err != nil {
		return nil, err
	}

	rooms := make([]*roborockv1.Room, 0)
	if len(s.client.cfg.SegmentNames) > 0 {
		ids := make([]int, 0, len(s.client.cfg.SegmentNames))
		for id := range s.client.cfg.SegmentNames {
			ids = append(ids, int(id))
		}
		sort.Ints(ids)
		for _, id := range ids {
			label := s.client.cfg.SegmentNames[uint32(id)]
			name := canonicalRoomName(label, id)
			rooms = append(rooms, &roborockv1.Room{
				Name:      name,
				SegmentId: uint32(id),
				Label:     displayRoomLabel(label, id),
			})
		}
		return &roborockv1.ListRoomsResponse{Rooms: rooms}, nil
	}

	segments, err := s.client.SegmentsSnapshot(ctx, deviceID)
	if err != nil {
		return nil, mapClientError("list rooms", err)
	}
	for _, seg := range segments {
		label := seg.label
		name := canonicalRoomName(label, seg.id)
		rooms = append(rooms, &roborockv1.Room{
			Name:      name,
			SegmentId: uint32(seg.id),
			Label:     displayRoomLabel(label, seg.id),
		})
	}
	sort.Slice(rooms, func(i, j int) bool { return rooms[i].SegmentId < rooms[j].SegmentId })
	return &roborockv1.ListRoomsResponse{Rooms: rooms}, nil
}

func (s *service) CleanRoom(ctx context.Context, req *roborockv1.CleanRoomRequest) (*roborockv1.CleanRoomResponse, error) {
	deviceID, err := s.requireDevice(ctx, req.GetDeviceId())
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.GetRoom()) == "" {
		return nil, status.Error(codes.InvalidArgument, "room is required")
	}
	segmentID, err := s.resolveRoomSegment(ctx, deviceID, req.GetRoom())
	if err != nil {
		return nil, err
	}
	if req.GetFanSpeed() != "" {
		if err := s.client.SetFanSpeed(ctx, deviceID, req.GetFanSpeed()); err != nil {
			return nil, mapClientError("set fan speed", err)
		}
	}
	if req.GetMopMode() != "" {
		if err := s.client.SetMopMode(ctx, deviceID, req.GetMopMode()); err != nil {
			return nil, mapClientError("set mop mode", err)
		}
	}
	if req.GetMopIntensity() != "" {
		if err := s.client.SetMopIntensity(ctx, deviceID, req.GetMopIntensity()); err != nil {
			return nil, mapClientError("set mop intensity", err)
		}
	}
	if !req.GetDryRun() {
		if err := s.client.CleanSegment(ctx, deviceID, []int{int(segmentID)}, int(req.GetRepeats())); err != nil {
			return nil, mapClientError("clean room", err)
		}
	}

	return &roborockv1.CleanRoomResponse{
		SegmentId: segmentID,
		Room:      canonicalRoomName(req.GetRoom(), int(segmentID)),
	}, nil
}

func (s *service) GoTo(ctx context.Context, req *roborockv1.GoToRequest) (*roborockv1.GoToResponse, error) {
	deviceID, err := s.requireDevice(ctx, req.GetDeviceId())
	if err != nil {
		return nil, err
	}
	if err := s.client.GoTo(ctx, deviceID, int(req.GetX()), int(req.GetY())); err != nil {
		return nil, mapClientError("go to", err)
	}
	return &roborockv1.GoToResponse{}, nil
}

func (s *service) SetDnd(ctx context.Context, req *roborockv1.SetDndRequest) (*roborockv1.SetDndResponse, error) {
	deviceID, err := s.requireDevice(ctx, req.GetDeviceId())
	if err != nil {
		return nil, err
	}
	if req.GetStartTime() == "" || req.GetEndTime() == "" {
		return nil, status.Error(codes.InvalidArgument, "start_time and end_time are required")
	}
	if err := s.client.SetDND(ctx, deviceID, req.GetStartTime(), req.GetEndTime(), req.GetEnabled()); err != nil {
		return nil, mapClientError("set dnd", err)
	}
	return &roborockv1.SetDndResponse{}, nil
}

func (s *service) ResetConsumable(ctx context.Context, req *roborockv1.ResetConsumableRequest) (*roborockv1.ResetConsumableResponse, error) {
	deviceID, err := s.requireDevice(ctx, req.GetDeviceId())
	if err != nil {
		return nil, err
	}
	if req.GetConsumable() == "" {
		return nil, status.Error(codes.InvalidArgument, "consumable is required")
	}
	if err := s.client.ResetConsumable(ctx, deviceID, req.GetConsumable()); err != nil {
		return nil, mapClientError("reset consumable", err)
	}
	return &roborockv1.ResetConsumableResponse{}, nil
}

func (s *service) requireDevice(ctx context.Context, deviceID string) (string, error) {
	if s.client == nil {
		return "", status.Error(codes.FailedPrecondition, "roborock client not configured")
	}
	if deviceID != "" {
		return deviceID, nil
	}

	devices, err := s.client.Devices(ctx)
	if err != nil {
		return "", mapClientError("list devices", err)
	}
	if len(devices) == 0 {
		return "", status.Error(codes.FailedPrecondition, "no roborock devices found")
	}
	if len(devices) == 1 {
		log.Printf("roborock: defaulting device_id to %s (%s)", devices[0].Name, devices[0].ID)
		return devices[0].ID, nil
	}
	names := make([]string, 0, len(devices))
	for _, dev := range devices {
		names = append(names, fmt.Sprintf("%s=%s", dev.Name, dev.ID))
	}
	sort.Strings(names)
	return "", status.Errorf(codes.InvalidArgument, "device_id is required (known devices: %s)", strings.Join(names, ", "))
}

func mapClientError(action string, err error) error {
	if errors.Is(err, ErrNotImplemented) {
		return status.Errorf(codes.Unimplemented, "roborock %s not implemented", action)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Errorf(codes.DeadlineExceeded, "roborock %s timed out", action)
	}
	msg := err.Error()
	if strings.Contains(msg, "not found on broadcast") {
		return status.Errorf(codes.FailedPrecondition, "roborock %s failed: %v (set roborock.device_ip_overrides for tailscale/subnet)", action, err)
	}
	if strings.Contains(msg, "cloud fallback not implemented") {
		return status.Errorf(codes.Unimplemented, "roborock %s failed: %v", action, err)
	}
	return status.Errorf(codes.Internal, "roborock %s failed: %v", action, err)
}

func (s *service) resolveRoomSegment(ctx context.Context, deviceID, room string) (uint32, error) {
	normalized := normalizeRoomName(room)
	if id, ok := parseRoomSegmentID(normalized); ok {
		return id, nil
	}
	for id, name := range s.client.cfg.SegmentNames {
		if normalizeRoomName(name) == normalized {
			return id, nil
		}
	}
	segments, err := s.client.SegmentsSnapshot(ctx, deviceID)
	if err != nil {
		return 0, mapClientError("list rooms", err)
	}
	for _, seg := range segments {
		if normalizeRoomName(seg.label) == normalized {
			return uint32(seg.id), nil
		}
		if normalizeRoomName(canonicalRoomName(seg.label, seg.id)) == normalized {
			return uint32(seg.id), nil
		}
	}
	return 0, status.Errorf(codes.NotFound, "room %q not found (try ListRooms)", room)
}

func normalizeRoomName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}
	return name
}

func canonicalRoomName(label string, id int) string {
	if strings.TrimSpace(label) != "" {
		return normalizeRoomName(label)
	}
	return fmt.Sprintf("segment_%d", id)
}

func displayRoomLabel(label string, id int) string {
	if strings.TrimSpace(label) == "" {
		label = fmt.Sprintf("segment_%d", id)
	}
	return strings.ReplaceAll(label, "_", " ")
}

func parseRoomSegmentID(room string) (uint32, bool) {
	if strings.HasPrefix(room, "segment_") {
		room = strings.TrimPrefix(room, "segment_")
	}
	if room == "" {
		return 0, false
	}
	for _, r := range room {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	val, err := strconv.Atoi(room)
	if err != nil || val <= 0 {
		return 0, false
	}
	return uint32(val), true
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
