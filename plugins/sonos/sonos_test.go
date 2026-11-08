package sonos

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	sonosv1 "github.com/joshp123/gohome/proto/gen/plugins/sonos/v1"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// readResponses maps every SOAP action the plugin may send to a response recorded from
// the house's Arc (device IDs replaced). Any other action fails the test.
var readResponses = map[string]string{
	"DeviceProperties#GetZoneInfo":               "DeviceProperties_GetZoneInfo.xml",
	"AVTransport#GetTransportSettings":           "AVTransport_GetTransportSettings.xml",
	"AVTransport#GetRemainingSleepTimerDuration": "AVTransport_GetRemainingSleepTimerDuration.xml",
	"AVTransport#GetMediaInfo":                   "AVTransport_GetMediaInfo.xml",
	"AVTransport#GetCrossfadeMode":               "AVTransport_GetCrossfadeMode.xml",
	"RenderingControl#GetEQ":                     "RenderingControl_GetEQ.xml",
	"RenderingControl#GetRoomCalibrationStatus":  "RenderingControl_GetRoomCalibrationStatus.xml",
	"RenderingControl#GetOutputFixed":            "RenderingControl_GetOutputFixed.xml",
	"RenderingControl#GetLoudness":               "RenderingControl_GetLoudness.xml",
	"RenderingControl#GetTreble":                 "RenderingControl_GetTreble.xml",
	"RenderingControl#GetBass":                   "RenderingControl_GetBass.xml",
	"ZoneGroupTopology#GetZoneGroupState":        "get_zone_group_state.xml",
	"AVTransport#GetTransportInfo":               "get_transport_info.xml",
	"AVTransport#GetPositionInfo":                "get_position_info.xml",
	"RenderingControl#GetVolume":                 "get_volume.xml",
	"RenderingControl#GetMute":                   "get_mute.xml",
}

type fakeArc struct {
	*httptest.Server
	responses    map[string]string
	down         bool
	failedAction string
}

func newFakeArc(t *testing.T) *fakeArc {
	arc := &fakeArc{responses: make(map[string]string)}
	for action, file := range readResponses {
		arc.responses[action] = file
	}
	arc.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := strings.TrimPrefix(strings.Trim(r.Header.Get("SOAPACTION"), `"`), "urn:schemas-upnp-org:service:")
		action = strings.Replace(action, ":1#", "#", 1)
		file, ok := arc.responses[action]
		if !ok {
			t.Errorf("plugin sent %q, which is not a read", action)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		if arc.down || action == arc.failedAction {
			http.Error(w, "", http.StatusServiceUnavailable)
			return
		}
		body, err := os.ReadFile("testdata/" + file)
		if err != nil {
			t.Fatal(err)
		}
		host := strings.TrimPrefix(arc.URL, "http://")
		w.Write([]byte(strings.ReplaceAll(string(body), "192.0.2.10:1400", host)))
	}))
	t.Cleanup(arc.Close)
	return arc
}

func newTestService(t *testing.T, arc *fakeArc) *service {
	client, err := NewClient(Config{BaseURL: arc.URL})
	if err != nil {
		t.Fatal(err)
	}
	return &service{client: client}
}

func TestGetZonesFromRecordedArc(t *testing.T) {
	response, err := newTestService(t, newFakeArc(t)).GetZones(context.Background(), &sonosv1.GetZonesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Zones) != 1 || response.Zones[0].Name != "Living Room" {
		t.Fatalf("zones: %v", response.Zones)
	}
	channels := map[string]string{}
	satellites := 0
	for _, p := range response.Zones[0].Players {
		channels[strings.Join(p.Channels, ",")] = p.Uuid
		if p.Satellite {
			satellites++
		}
	}
	for _, want := range []string{"LF,RF", "LR", "RR", "SW"} {
		if channels[want] == "" {
			t.Errorf("no player plays %s: %v", want, channels)
		}
	}
	if satellites != 3 {
		t.Errorf("satellites = %d, want 3 (two surrounds and the Sub)", satellites)
	}
}

func TestGetPlaybackFromRecordedArc(t *testing.T) {
	response, err := newTestService(t, newFakeArc(t)).GetPlayback(context.Background(), &sonosv1.GetPlaybackRequest{})
	if err != nil {
		t.Fatal(err)
	}
	zone := response.Zones[0]
	if zone.State != sonosv1.TransportState_TRANSPORT_STATE_STOPPED || zone.Volume != 26 || zone.Muted || zone.Track != nil {
		t.Fatalf("playback: %v", zone)
	}
}

// The playing response is constructed from Sonos's DIDL-Lite format: the Arc was not
// made to play anything to record one.
func TestGetPlaybackReadsTrack(t *testing.T) {
	arc := newFakeArc(t)
	arc.responses["AVTransport#GetPositionInfo"] = "get_position_info_playing.xml"
	response, err := newTestService(t, arc).GetPlayback(context.Background(), &sonosv1.GetPlaybackRequest{})
	if err != nil {
		t.Fatal(err)
	}
	track := response.Zones[0].Track
	if track.GetTitle() != "Example Track & Friends" || track.GetArtist() != "Example Artist" || track.GetAlbum() != "Example Album" ||
		track.GetDurationSeconds() != 245 || track.GetPositionSeconds() != 62 || !strings.HasPrefix(track.Uri, "x-sonos-spotify:") {
		t.Fatalf("track: %v", track)
	}
}

func TestMetricsAbsentWhenArcUnreachable(t *testing.T) {
	arc := newFakeArc(t)
	client, err := NewClient(Config{BaseURL: arc.URL})
	if err != nil {
		t.Fatal(err)
	}
	registry := prometheus.NewRegistry()
	registry.MustRegister(NewMetricsCollector(client))

	metrics := gather(t, registry)
	if metrics["gohome_sonos_bass{zone=Living Room}"] != 2 || metrics["gohome_sonos_music_surround_level{zone=Living Room}"] != -6 || metrics["gohome_sonos_scrape_success"] != 1 || metrics["gohome_sonos_volume{zone=Living Room}"] != 26 ||
		metrics["gohome_sonos_transport_state{state=stopped,zone=Living Room}"] != 1 ||
		metrics["gohome_sonos_muted{zone=Living Room}"] != 0 || metrics["gohome_sonos_zone_reachable{zone=Living Room}"] != 1 {
		t.Fatalf("live metrics: %v", metrics)
	}
	lastSuccess := metrics["gohome_sonos_last_success_timestamp_seconds"]

	arc.down = true
	metrics = gather(t, registry)
	if len(metrics) != 2 || metrics["gohome_sonos_scrape_success"] != 0 ||
		metrics["gohome_sonos_last_success_timestamp_seconds"] != lastSuccess {
		t.Fatalf("metrics after failure, want only scrape_success=0 and the unchanged last success: %v", metrics)
	}
}

func gather(t *testing.T, registry *prometheus.Registry) map[string]float64 {
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]float64{}
	for _, family := range families {
		for _, m := range family.Metric {
			out[seriesName(family.GetName(), m)] = m.GetGauge().GetValue()
		}
	}
	return out
}

func seriesName(name string, m *dto.Metric) string {
	if len(m.Label) == 0 {
		return name
	}
	labels := make([]string, 0, len(m.Label))
	for _, l := range m.Label {
		labels = append(labels, l.GetName()+"="+l.GetValue())
	}
	return name + "{" + strings.Join(labels, ",") + "}"
}

func TestDiagnosticsRetainOnlySuccessfulReads(t *testing.T) {
	arc := newFakeArc(t)
	service := newTestService(t, arc)
	first, err := service.GetDiagnostics(context.Background(), &sonosv1.GetDiagnosticsRequest{})
	if err != nil || first.Zones[0].GetBass() != 2 {
		t.Fatalf("first read: %v, %v", first, err)
	}
	arc.failedAction = "RenderingControl#GetBass"
	next, err := service.GetDiagnostics(context.Background(), &sonosv1.GetDiagnosticsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	got := next.Zones[0]
	if got.Bass != nil || got.Treble == nil || *got.Treble != 2 || len(got.ReadErrors) != 1 {
		t.Fatalf("partial read: %v", got)
	}
}
