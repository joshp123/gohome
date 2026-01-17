package airgradient

import (
	"context"
	"sort"

	airgradientv1 "github.com/joshp123/gohome/proto/gen/plugins/airgradient/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type service struct {
	airgradientv1.UnimplementedAirGradientServiceServer
	client *Client
}

func RegisterAirGradientService(server *grpc.Server, client *Client) {
	airgradientv1.RegisterAirGradientServiceServer(server, &service{client: client})
}

func (s *service) GetCurrent(ctx context.Context, _ *airgradientv1.GetCurrentRequest) (*airgradientv1.GetCurrentResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "airgradient client not configured")
	}
	current, err := s.client.Current(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get current: %v", err)
	}

	return &airgradientv1.GetCurrentResponse{Reading: toCurrentReading(current)}, nil
}

func (s *service) GetSnapshot(ctx context.Context, _ *airgradientv1.GetSnapshotRequest) (*airgradientv1.GetSnapshotResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "airgradient client not configured")
	}
	payload, err := s.client.RawCurrent(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get snapshot: %v", err)
	}
	return &airgradientv1.GetSnapshotResponse{Json: string(payload)}, nil
}

func (s *service) GetMetrics(ctx context.Context, _ *airgradientv1.GetMetricsRequest) (*airgradientv1.GetMetricsResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "airgradient client not configured")
	}
	payload, err := s.client.Metrics(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get metrics: %v", err)
	}
	return &airgradientv1.GetMetricsResponse{Openmetrics: string(payload)}, nil
}

func (s *service) GetConfig(ctx context.Context, _ *airgradientv1.GetConfigRequest) (*airgradientv1.GetConfigResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "airgradient client not configured")
	}
	payload, err := s.client.Config(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get config: %v", err)
	}
	return &airgradientv1.GetConfigResponse{Json: string(payload)}, nil
}

func toCurrentReading(current CurrentMeasures) *airgradientv1.CurrentReading {
	reading := &airgradientv1.CurrentReading{
		Serial:                        optionalString(current.SerialNo),
		Firmware:                      optionalString(current.Firmware),
		Model:                         optionalString(current.Model),
		LedMode:                       optionalString(current.LedMode),
		WifiRssiDbm:                   current.Wifi,
		Co2Ppm:                        current.RCO2,
		Pm01Ugm3:                      current.PM01,
		Pm02Ugm3:                      current.PM02,
		Pm10Ugm3:                      current.PM10,
		Pm02CompensatedUgm3:           current.PM02Compensated,
		Pm01StandardUgm3:              current.PM01Standard,
		Pm02StandardUgm3:              current.PM02Standard,
		Pm10StandardUgm3:              current.PM10Standard,
		Pm003CountPerDl:               current.PM003Count,
		Pm005CountPerDl:               current.PM005Count,
		Pm01CountPerDl:                current.PM01Count,
		Pm02CountPerDl:                current.PM02Count,
		Pm50CountPerDl:                current.PM50Count,
		Pm10CountPerDl:                current.PM10Count,
		TemperatureCelsius:            current.Temperature,
		TemperatureCompensatedCelsius: current.TemperatureCorrected,
		HumidityPercent:               current.Humidity,
		HumidityCompensatedPercent:    current.HumidityCorrected,
		TvocIndex:                     current.TVOCIndex,
		TvocRaw:                       current.TVOCRaw,
		NoxIndex:                      current.NOxIndex,
		NoxRaw:                        current.NOxRaw,
		BootCount:                     pickBootCount(current),
	}

	if len(current.Satellites) > 0 {
		ids := make([]string, 0, len(current.Satellites))
		for id := range current.Satellites {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		readings := make([]*airgradientv1.SatelliteReading, 0, len(ids))
		for _, id := range ids {
			sat := current.Satellites[id]
			readings = append(readings, &airgradientv1.SatelliteReading{
				Id:                 id,
				TemperatureCelsius: sat.Temperature,
				HumidityPercent:    sat.Humidity,
				WifiRssiDbm:        sat.Wifi,
			})
		}
		reading.Satellites = readings
	}

	return reading
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func pickBootCount(current CurrentMeasures) *float64 {
	if current.BootCount != nil {
		return current.BootCount
	}
	return current.Boot
}
