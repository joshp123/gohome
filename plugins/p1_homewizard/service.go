package p1_homewizard

import (
	context "context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

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

func (s *service) GetReading(ctx context.Context, _ *p1v1.GetReadingRequest) (*p1v1.GetReadingResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "p1_homewizard client not configured")
	}
	data, err := s.client.Data(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get reading: %v", err)
	}
	reading := &p1v1.SmartMeterReading{
		MeterModel:            data.MeterModel,
		WifiStrengthPercent:   data.WifiStrengthPercent,
		TotalImportKwh:        data.TotalImportKWh,
		TotalImportT1Kwh:      data.TotalImportT1KWh,
		TotalImportT2Kwh:      data.TotalImportT2KWh,
		TotalExportKwh:        data.TotalExportKWh,
		TotalExportT1Kwh:      data.TotalExportT1KWh,
		TotalExportT2Kwh:      data.TotalExportT2KWh,
		ActivePowerWatts:      data.ActivePowerW,
		ActivePowerL1Watts:    data.ActivePowerL1W,
		ActiveCurrentAmperes:  data.ActiveCurrentA,
		ActiveCurrentL1Amperes: data.ActiveCurrentL1A,
		VoltageSagL1Count:     data.VoltageSagL1Count,
		VoltageSwellL1Count:   data.VoltageSwellL1Count,
		AnyPowerFailCount:     data.AnyPowerFailCount,
		LongPowerFailCount:    data.LongPowerFailCount,
	}
	if data.SMRVersion != nil {
		v := int32(*data.SMRVersion)
		reading.SmrVersion = &v
	}
	if data.ActiveTariff != nil {
		v := int32(*data.ActiveTariff)
		reading.ActiveTariff = &v
	}
	if rawTelegram, err := s.client.Telegram(ctx); err == nil {
		if parsed, ok := ParseTelegram(rawTelegram); ok {
			for _, ev := range parsed.PowerFailureEvents {
				reading.PowerFailureEvents = append(reading.PowerFailureEvents, &p1v1.PowerFailureRecord{
					RestoredAt:      timestamppb.New(ev.RestoredAt),
					DurationSeconds: ev.DurationSeconds,
				})
			}
		}
	}
	return &p1v1.GetReadingResponse{Reading: reading}, nil
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
