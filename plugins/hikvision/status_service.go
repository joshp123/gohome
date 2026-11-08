package hikvision

import (
	"context"
	"time"

	hikvisionv1 "github.com/joshp123/gohome/proto/gen/plugins/hikvision/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *service) GetDeviceStatus(ctx context.Context, _ *hikvisionv1.GetDeviceStatusRequest) (*hikvisionv1.GetDeviceStatusResponse, error) {
	if err := s.configured(); err != nil {
		return nil, err
	}
	device, err := s.client.DeviceStatus(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "device status: %v", err)
	}
	storage, err := s.client.Storage(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "storage: %v", err)
	}
	streaming, err := s.client.StreamingStatus(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "streaming status: %v", err)
	}
	tracks, err := s.client.RecordingTracks(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "recording tracks: %v", err)
	}
	clock, _ := time.Parse(time.RFC3339, device.CurrentTime)
	response := &hikvisionv1.GetDeviceStatusResponse{
		Device:            &hikvisionv1.DeviceStatus{DeviceTime: optionalTimestamp(clock), UptimeSeconds: device.Uptime, RebootCount: device.Reboots},
		StreamingSessions: streaming.Sessions,
	}
	for _, cpu := range device.CPUs {
		response.Device.Cpus = append(response.Device.Cpus, &hikvisionv1.CPUStatus{UsagePercent: cpu.Usage})
	}
	for _, memory := range device.Memory {
		response.Device.Memory = append(response.Device.Memory, &hikvisionv1.MemoryStatus{UsageRaw: memory.Usage, AvailableRaw: memory.Available})
	}
	for kind, devices := range map[string][]storageDeviceXML{"hdd": storage.Disks, "nas": storage.NAS} {
		for _, disk := range devices {
			response.Storage = append(response.Storage, &hikvisionv1.StorageDevice{Storage: kind, Id: disk.ID, Status: disk.Status, CapacityMegabytes: disk.Capacity, FreeMegabytes: disk.Free, Property: disk.Property})
		}
	}
	for _, track := range tracks.Tracks {
		response.RecordingTracks = append(response.RecordingTracks, &hikvisionv1.RecordingTrack{Id: track.ID, Enabled: track.Enabled, LoopEnabled: track.Loop, ScheduleEnabled: track.ScheduleEnabled})
	}
	return response, nil
}
