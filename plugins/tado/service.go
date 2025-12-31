package tado

import (
	context "context"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1 "github.com/joshp123/gohome/proto/gen/plugins/tado/v1"
)

type service struct {
	v1.UnimplementedTadoServiceServer
	client *Client
}

func RegisterTadoService(server *grpc.Server, client *Client) {
	v1.RegisterTadoServiceServer(server, &service{client: client})
}

func (s *service) ListZones(ctx context.Context, _ *v1.ListZonesRequest) (*v1.ListZonesResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "tado client not configured")
	}

	zones, err := s.client.Zones(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list zones: %v", err)
	}

	resp := &v1.ListZonesResponse{}
	for _, zone := range zones {
		resp.Zones = append(resp.Zones, &v1.Zone{
			Id:   strconv.Itoa(zone.ID),
			Name: zone.Name,
		})
	}

	return resp, nil
}

func (s *service) SetTemperature(ctx context.Context, req *v1.SetTemperatureRequest) (*v1.SetTemperatureResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "tado client not configured")
	}

	zoneID, err := strconv.Atoi(req.ZoneId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid zone_id: %v", err)
	}

	if err := s.client.SetZoneTemperature(ctx, zoneID, req.TemperatureCelsius); err != nil {
		return nil, status.Errorf(codes.Internal, "set temperature: %v", err)
	}

	return &v1.SetTemperatureResponse{}, nil
}
