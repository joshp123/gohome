package sonos

import (
	"context"

	sonosv1 "github.com/joshp123/gohome/proto/gen/plugins/sonos/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type service struct {
	sonosv1.UnimplementedSonosServiceServer
	client *Client
}

func (s *service) GetZones(ctx context.Context, _ *sonosv1.GetZonesRequest) (*sonosv1.GetZonesResponse, error) {
	zones, err := s.zones(ctx)
	if err != nil {
		return nil, err
	}
	response := &sonosv1.GetZonesResponse{}
	for _, zone := range zones {
		out := &sonosv1.Zone{Name: zone.Name, CoordinatorUuid: zone.CoordinatorUUID}
		for _, p := range zone.Players {
			out.Players = append(out.Players, &sonosv1.Player{
				Uuid: p.UUID, Name: p.Name, Host: p.Host, SoftwareVersion: p.SoftwareVersion,
				Channels: p.Channels, Satellite: p.Satellite,
			})
		}
		response.Zones = append(response.Zones, out)
	}
	return response, nil
}

func (s *service) GetPlayback(ctx context.Context, _ *sonosv1.GetPlaybackRequest) (*sonosv1.GetPlaybackResponse, error) {
	zones, err := s.zones(ctx)
	if err != nil {
		return nil, err
	}
	response := &sonosv1.GetPlaybackResponse{}
	for _, zone := range zones {
		zoneStatus, err := s.client.Status(ctx, zone)
		if err != nil {
			return nil, status.Errorf(codes.Unavailable, "zone %q: %v", zone.Name, err)
		}
		track, err := s.client.Track(ctx, zone)
		if err != nil {
			return nil, status.Errorf(codes.Unavailable, "zone %q: %v", zone.Name, err)
		}
		response.Zones = append(response.Zones, &sonosv1.ZonePlayback{
			ZoneName: zone.Name,
			State:    transportStates[zoneStatus.TransportState],
			Volume:   zoneStatus.Volume,
			Muted:    zoneStatus.Muted,
			Track:    toTrack(track),
		})
	}
	return response, nil
}

func (s *service) zones(ctx context.Context) ([]Zone, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "sonos client not configured")
	}
	zones, err := s.client.Zones(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "read topology: %v", err)
	}
	return zones, nil
}

var transportStates = map[string]sonosv1.TransportState{
	"STOPPED":          sonosv1.TransportState_TRANSPORT_STATE_STOPPED,
	"PLAYING":          sonosv1.TransportState_TRANSPORT_STATE_PLAYING,
	"PAUSED_PLAYBACK":  sonosv1.TransportState_TRANSPORT_STATE_PAUSED,
	"TRANSITIONING":    sonosv1.TransportState_TRANSPORT_STATE_TRANSITIONING,
	"NO_MEDIA_PRESENT": sonosv1.TransportState_TRANSPORT_STATE_NO_MEDIA,
}

func toTrack(track *Track) *sonosv1.Track {
	if track == nil {
		return nil
	}
	return &sonosv1.Track{
		Title:           optionalString(track.Title),
		Artist:          optionalString(track.Artist),
		Album:           optionalString(track.Album),
		Uri:             track.URI,
		DurationSeconds: track.DurationSeconds,
		PositionSeconds: track.PositionSeconds,
	}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (s *service) GetDiagnostics(ctx context.Context, _ *sonosv1.GetDiagnosticsRequest) (*sonosv1.GetDiagnosticsResponse, error) {
	zones, err := s.zones(ctx)
	if err != nil {
		return nil, err
	}
	out := &sonosv1.GetDiagnosticsResponse{}
	for _, zone := range zones {
		out.Zones = append(out.Zones, s.client.Diagnostics(ctx, zone))
	}
	return out, nil
}
