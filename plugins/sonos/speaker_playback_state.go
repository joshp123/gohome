package sonos

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type Zone struct {
	Name            string
	CoordinatorUUID string
	CoordinatorHost string
	Players         []Player
}

type Player struct {
	UUID            string
	Name            string
	Host            string
	SoftwareVersion string
	Channels        []string
	Satellite       bool
}

type ZoneStatus struct {
	TransportState string
	Volume         uint32
	Muted          bool
}

type Track struct {
	Title, Artist, Album string
	URI                  string
	DurationSeconds      *uint32
	PositionSeconds      *uint32
}

type zoneGroupState struct {
	Groups []struct {
		Coordinator string `xml:"Coordinator,attr"`
		Members     []struct {
			topologyPlayer
			Satellites []topologyPlayer `xml:"Satellite"`
		} `xml:"ZoneGroupMember"`
	} `xml:"ZoneGroups>ZoneGroup"`
}

type topologyPlayer struct {
	UUID            string `xml:"UUID,attr"`
	Location        string `xml:"Location,attr"`
	ZoneName        string `xml:"ZoneName,attr"`
	SoftwareVersion string `xml:"SoftwareVersion,attr"`
	HTSatChanMapSet string `xml:"HTSatChanMapSet,attr"`
	Invisible       string `xml:"Invisible,attr"`
}

func parseZoneGroupState(document string) ([]Zone, error) {
	var state zoneGroupState
	if err := xml.Unmarshal([]byte(document), &state); err != nil {
		return nil, fmt.Errorf("decode zone group state: %w", err)
	}
	zones := make([]Zone, 0, len(state.Groups))
	for _, group := range state.Groups {
		zone := Zone{CoordinatorUUID: group.Coordinator}
		var channelMap string
		for _, member := range group.Members {
			if member.UUID == group.Coordinator {
				zone.Name = member.ZoneName
				channelMap = member.HTSatChanMapSet
				host, err := hostOf(member.Location)
				if err != nil {
					return nil, err
				}
				zone.CoordinatorHost = host
			}
		}
		if zone.CoordinatorHost == "" {
			return nil, fmt.Errorf("zone group coordinator %s is not among its members", group.Coordinator)
		}
		channels := parseChannelMap(channelMap)
		for _, member := range group.Members {
			players := append([]topologyPlayer{member.topologyPlayer}, member.Satellites...)
			for i, p := range players {
				host, err := hostOf(p.Location)
				if err != nil {
					return nil, err
				}
				zone.Players = append(zone.Players, Player{
					UUID:            p.UUID,
					Name:            p.ZoneName,
					Host:            host,
					SoftwareVersion: p.SoftwareVersion,
					Channels:        channels[p.UUID],
					Satellite:       i > 0 || p.Invisible == "1",
				})
			}
		}
		zones = append(zones, zone)
	}
	return zones, nil
}

// parseChannelMap reads "UUID:LF,RF;UUID:SW" into channels per player.
func parseChannelMap(channelMap string) map[string][]string {
	channels := make(map[string][]string)
	for _, entry := range strings.Split(channelMap, ";") {
		uuid, list, ok := strings.Cut(entry, ":")
		if ok {
			channels[uuid] = strings.Split(list, ",")
		}
	}
	return channels
}

func hostOf(location string) (string, error) {
	parsed, err := url.Parse(location)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("player location %q has no host", location)
	}
	return parsed.Host, nil
}

type didlLite struct {
	Items []struct {
		Title   string `xml:"http://purl.org/dc/elements/1.1/ title"`
		Creator string `xml:"http://purl.org/dc/elements/1.1/ creator"`
		Album   string `xml:"urn:schemas-upnp-org:metadata-1-0/upnp/ album"`
	} `xml:"item"`
}

func parsePositionInfo(values map[string]string) (*Track, error) {
	uri := values["TrackURI"]
	if uri == "" {
		return nil, nil
	}
	track := &Track{
		URI:             uri,
		DurationSeconds: parseDuration(values["TrackDuration"]),
		PositionSeconds: parseClock(values["RelTime"]),
	}
	if metadata := values["TrackMetaData"]; metadata != "" && metadata != "NOT_IMPLEMENTED" {
		var didl didlLite
		if err := xml.Unmarshal([]byte(metadata), &didl); err != nil {
			return nil, fmt.Errorf("decode track metadata: %w", err)
		}
		if len(didl.Items) > 0 {
			track.Title, track.Artist, track.Album = didl.Items[0].Title, didl.Items[0].Creator, didl.Items[0].Album
		}
	}
	return track, nil
}

// parseClock reads "H:MM:SS"; anything else ("NOT_IMPLEMENTED") is absent.
func parseClock(clock string) *uint32 {
	parts := strings.Split(clock, ":")
	if len(parts) != 3 {
		return nil
	}
	var seconds uint32
	for _, part := range parts {
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return nil
		}
		seconds = seconds*60 + uint32(n)
	}
	return &seconds
}

// parseDuration treats "0:00:00" as absent: streams and TV audio have no length.
func parseDuration(clock string) *uint32 {
	seconds := parseClock(clock)
	if seconds == nil || *seconds == 0 {
		return nil
	}
	return seconds
}
