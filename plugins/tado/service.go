package tado

import (
	context "context"

	"google.golang.org/grpc"

	v1 "github.com/elliot-alderson/gohome/proto/gen/plugins/tado/v1"
)

type service struct {
	v1.UnimplementedTadoServiceServer
}

func RegisterTadoService(server *grpc.Server) {
	v1.RegisterTadoServiceServer(server, &service{})
}

func (s *service) ListZones(ctx context.Context, _ *v1.ListZonesRequest) (*v1.ListZonesResponse, error) {
	_ = ctx
	return &v1.ListZonesResponse{}, nil
}

func (s *service) SetTemperature(ctx context.Context, _ *v1.SetTemperatureRequest) (*v1.SetTemperatureResponse, error) {
	_ = ctx
	return &v1.SetTemperatureResponse{}, nil
}
