package brother

import (
	"context"

	brotherv1 "github.com/joshp123/gohome/proto/gen/plugins/brother/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type grpcService struct {
	brotherv1.UnimplementedBrotherServiceServer
	client *Client
}

// RegisterBrotherService binds BrotherService to a gRPC server.
func RegisterBrotherService(server *grpc.Server, client *Client) {
	brotherv1.RegisterBrotherServiceServer(server, &grpcService{client: client})
}

func (s *grpcService) GetPrinterStatus(ctx context.Context, _ *brotherv1.GetPrinterStatusRequest) (*brotherv1.GetPrinterStatusResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "brother client not configured")
	}
	reading, err := s.client.FetchPrinterStatus(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "fetch brother printer status: %v", err)
	}
	out := &brotherv1.PrinterStatus{
		NodeName:               reading.NodeName,
		ModelName:              reading.ModelName,
		IpAddress:              reading.IPAddress,
		SerialNumber:           reading.SerialNumber,
		FirmwareVersion:        reading.FirmwareVersion,
		MemoryMegabytes:        reading.MemoryMegabytes,
		PageCounter:            reading.PageCounter,
		DuplexPageCounter:      reading.DuplexPageCounter,
		AverageCoveragePercent: reading.AverageCoveragePercent,
		DrumRemainingPercent:   reading.DrumRemainingPercent,
		TonerRemainingPercent:  reading.TonerRemainingPercent,
		TonerReplaceCount:      reading.TonerReplaceCount,
		DrumReplaceCount:       reading.DrumReplaceCount,
		TotalPaperJams:         reading.TotalPaperJams,
		JamTray1Count:          reading.JamTray1Count,
		JamInsideCount:         reading.JamInsideCount,
		JamRearCount:           reading.JamRearCount,
		JamDuplexCount:         reading.JamDuplexCount,
		ObservedAt:             timestamppb.New(reading.ObservedAt),
	}
	for _, entry := range reading.RecentErrors {
		out.RecentErrors = append(out.RecentErrors, &brotherv1.PrinterErrorEntry{
			Message:   entry.Message,
			PageCount: entry.PageCount,
		})
	}
	return &brotherv1.GetPrinterStatusResponse{Status: out}, nil
}
