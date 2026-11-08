package tado

import (
	context "context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

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

func (s *service) GetSourceData(ctx context.Context, req *v1.GetSourceDataRequest) (*v1.GetSourceDataResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "tado client not configured")
	}
	var suffix string
	switch req.GetResource() {
	case v1.GetSourceDataRequest_ZONES:
		suffix = "/zones"
	case v1.GetSourceDataRequest_ZONE_STATES:
		suffix = "/zoneStates"
	case v1.GetSourceDataRequest_WEATHER:
		suffix = "/weather"
	case v1.GetSourceDataRequest_HOME_STATE:
		suffix = "/state"
	case v1.GetSourceDataRequest_DEVICES:
		suffix = "/devices"
	case v1.GetSourceDataRequest_DAY_REPORT:
		if _, err := time.Parse("2006-01-02", req.GetDate()); err != nil || req.GetZoneId() <= 0 {
			return nil, status.Error(codes.InvalidArgument, "day report requires a positive zone_id and YYYY-MM-DD date")
		}
		suffix = fmt.Sprintf("/zones/%d/dayReport?date=%s", req.GetZoneId(), req.GetDate())
	default:
		return nil, status.Error(codes.InvalidArgument, "resource is required")
	}
	homeID, err := s.client.HomeID(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "discover home: %v", err)
	}
	response, err := s.client.doRequest(ctx, http.MethodGet, fmt.Sprintf("/homes/%d%s", homeID, suffix), nil)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "read source: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, status.Errorf(codes.Unavailable, "source HTTP %d; rate limit %s", response.StatusCode, response.Header.Get("ratelimit"))
	}
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "read response: %v", err)
	}
	return &v1.GetSourceDataResponse{Json: payload, RateLimit: response.Header.Get("ratelimit"), RateLimitPolicy: response.Header.Get("ratelimit-policy")}, nil
}
