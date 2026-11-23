package unifi

import (
	"context"

	unifiv1 "github.com/joshp123/gohome/proto/gen/plugins/unifi/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type grpcService struct {
	unifiv1.UnimplementedUniFiServiceServer
	client *Client
}

// RegisterUniFiService binds UniFiService to a gRPC server.
func RegisterUniFiService(server *grpc.Server, client *Client) {
	unifiv1.RegisterUniFiServiceServer(server, &grpcService{client: client})
}

func (s *grpcService) GetSiteSummary(ctx context.Context, _ *unifiv1.GetSiteSummaryRequest) (*unifiv1.GetSiteSummaryResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "unifi client not configured")
	}
	summary, err := s.client.FetchSiteSummary(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "fetch unifi site summary: %v", err)
	}
	out := &unifiv1.SiteSummary{
		ControllerName:    summary.ControllerName,
		HardwareShortname: summary.HardwareShortname,
		Site:              summary.Site,
		WiredClients:      summary.WiredClients,
		WirelessClients:   summary.WirelessClients,
		ObservedAt:        timestamppb.New(summary.ObservedAt),
	}
	for _, sub := range summary.Subsystems {
		out.Subsystems = append(out.Subsystems, &unifiv1.SubsystemHealth{
			Subsystem:              sub.Subsystem,
			Status:                 sub.Status,
			ActiveUsers:            sub.ActiveUsers,
			ActiveGuests:           sub.ActiveGuests,
			AdoptedDevices:         sub.AdoptedDevices,
			DisconnectedDevices:    sub.DisconnectedDevices,
			TxBytesPerSecond:       sub.TxBytesPerSecond,
			RxBytesPerSecond:       sub.RxBytesPerSecond,
			WanLatencyMilliseconds: sub.WANLatencyMilliseconds,
			WanUptimeSeconds:       sub.WANUptimeSeconds,
			WanIp:                  sub.WANIP,
			IspName:                sub.ISPName,
		})
	}
	for _, dev := range summary.Devices {
		out.Devices = append(out.Devices, &unifiv1.NetworkDevice{
			Mac:                 dev.MAC,
			Name:                dev.Name,
			Model:               dev.Model,
			DeviceType:          dev.DeviceType,
			Ip:                  dev.IP,
			Version:             dev.Version,
			Online:              dev.Online,
			UptimeSeconds:       dev.UptimeSeconds,
			CpuPercent:          dev.CPUPercent,
			MemoryPercent:       dev.MemoryPercent,
			TemperatureCelsius:  dev.TemperatureCelsius,
			ActiveClients:       dev.ActiveClients,
			SatisfactionPercent: dev.SatisfactionPercent,
		})
	}
	return &unifiv1.GetSiteSummaryResponse{Summary: out}, nil
}
