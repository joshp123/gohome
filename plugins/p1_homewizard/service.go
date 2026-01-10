package p1_homewizard

import (
	context "context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	p1v1 "github.com/joshp123/gohome/proto/gen/plugins/p1_homewizard/v1"
)

type service struct {
	p1v1.UnimplementedP1HomewizardServiceServer
	client *Client
}

func RegisterP1HomewizardService(server *grpc.Server, client *Client) {
	p1v1.RegisterP1HomewizardServiceServer(server, &service{client: client})
}

func (s *service) GetInfo(ctx context.Context, _ *p1v1.GetInfoRequest) (*p1v1.GetInfoResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "p1_homewizard client not configured")
	}
	info, err := s.client.Info(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get info: %v", err)
	}

	return &p1v1.GetInfoResponse{Info: &p1v1.Info{
		ProductName:     info.ProductName,
		ProductType:     info.ProductType,
		Serial:          info.Serial,
		FirmwareVersion: info.Firmware,
		ApiVersion:      info.APIVersion,
	}}, nil
}

func (s *service) GetSnapshot(ctx context.Context, _ *p1v1.GetSnapshotRequest) (*p1v1.GetSnapshotResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "p1_homewizard client not configured")
	}
	payload, err := s.client.RawData(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get snapshot: %v", err)
	}
	return &p1v1.GetSnapshotResponse{Json: string(payload)}, nil
}

func (s *service) GetTelegram(ctx context.Context, _ *p1v1.GetTelegramRequest) (*p1v1.GetTelegramResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "p1_homewizard client not configured")
	}
	telegram, err := s.client.Telegram(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get telegram: %v", err)
	}
	return &p1v1.GetTelegramResponse{Telegram: telegram}, nil
}
