package sonos

import (
	"context"
	"fmt"
	"strconv"

	sonosv1 "github.com/joshp123/gohome/proto/gen/plugins/sonos/v1"
	"github.com/prometheus/client_golang/prometheus"
)

// Diagnostics reads settings and measurements without changing playback. Each
// action fails independently; an unsupported property is absent, never zero.
func (c *Client) Diagnostics(ctx context.Context, zone Zone) *sonosv1.ZoneDiagnostics {
	out := &sonosv1.ZoneDiagnostics{Zone: zone.Name}
	read := func(action readAction) map[string]string {
		values, err := c.read(ctx, "http://"+zone.CoordinatorHost, action)
		if err != nil {
			out.ReadErrors = append(out.ReadErrors, err.Error())
			return nil
		}
		return values
	}
	number := func(values map[string]string, key string) *float64 {
		raw, ok := values[key]
		if !ok || raw == "NOT_IMPLEMENTED" {
			return nil
		}
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			out.ReadErrors = append(out.ReadErrors, fmt.Sprintf("invalid %s: %q", key, raw))
			return nil
		}
		return &value
	}
	seconds := func(values map[string]string, key string, duration bool) *float64 {
		var parsed *uint32
		if duration {
			parsed = parseDuration(values[key])
		} else {
			parsed = parseClock(values[key])
		}
		if parsed == nil {
			return nil
		}
		value := float64(*parsed)
		return &value
	}
	var values map[string]string
	values = read(readAction{"/MediaRenderer/RenderingControl/Control", "RenderingControl", "GetBass", `<InstanceID>0</InstanceID>`})
	out.Bass = number(values, "CurrentBass")
	values = read(readAction{"/MediaRenderer/RenderingControl/Control", "RenderingControl", "GetTreble", `<InstanceID>0</InstanceID>`})
	out.Treble = number(values, "CurrentTreble")
	values = read(readAction{"/MediaRenderer/RenderingControl/Control", "RenderingControl", "GetLoudness", `<InstanceID>0</InstanceID><Channel>Master</Channel>`})
	out.Loudness = number(values, "CurrentLoudness")
	values = read(readAction{"/MediaRenderer/RenderingControl/Control", "RenderingControl", "GetOutputFixed", `<InstanceID>0</InstanceID>`})
	out.OutputFixed = number(values, "CurrentFixed")
	values = read(readAction{"/MediaRenderer/RenderingControl/Control", "RenderingControl", "GetRoomCalibrationStatus", `<InstanceID>0</InstanceID>`})
	out.RoomCalibrationEnabled = number(values, "RoomCalibrationEnabled")
	out.RoomCalibrationAvailable = number(values, "RoomCalibrationAvailable")
	values = read(readAction{"/MediaRenderer/AVTransport/Control", "AVTransport", "GetCrossfadeMode", `<InstanceID>0</InstanceID>`})
	out.Crossfade = number(values, "CrossfadeMode")
	values = read(readAction{"/MediaRenderer/AVTransport/Control", "AVTransport", "GetPositionInfo", `<InstanceID>0</InstanceID>`})
	out.TrackNumber = number(values, "Track")
	out.TrackDurationSeconds = seconds(values, "TrackDuration", true)
	out.TrackPositionSeconds = seconds(values, "RelTime", false)
	values = read(readAction{"/MediaRenderer/AVTransport/Control", "AVTransport", "GetMediaInfo", `<InstanceID>0</InstanceID>`})
	out.QueueTracks = number(values, "NrTracks")
	out.MediaDurationSeconds = seconds(values, "MediaDuration", true)
	values = read(readAction{"/MediaRenderer/AVTransport/Control", "AVTransport", "GetRemainingSleepTimerDuration", `<InstanceID>0</InstanceID>`})
	out.SleepTimerRemainingSeconds = seconds(values, "RemainingSleepTimerDuration", true)
	values = read(readAction{"/DeviceProperties/Control", "DeviceProperties", "GetZoneInfo", ``})
	out.AudioInputFormat = number(values, "HTAudioIn")
	values = read(readAction{"/MediaRenderer/RenderingControl/Control", "RenderingControl", "GetEQ", `<InstanceID>0</InstanceID><EQType>SurroundEnable</EQType>`})
	out.SurroundEnabled = number(values, "CurrentValue")
	values = read(readAction{"/MediaRenderer/RenderingControl/Control", "RenderingControl", "GetEQ", `<InstanceID>0</InstanceID><EQType>SubEnable</EQType>`})
	out.SubEnabled = number(values, "CurrentValue")
	values = read(readAction{"/MediaRenderer/RenderingControl/Control", "RenderingControl", "GetEQ", `<InstanceID>0</InstanceID><EQType>SubGain</EQType>`})
	out.SubGain = number(values, "CurrentValue")
	values = read(readAction{"/MediaRenderer/RenderingControl/Control", "RenderingControl", "GetEQ", `<InstanceID>0</InstanceID><EQType>AudioDelay</EQType>`})
	out.AudioDelay = number(values, "CurrentValue")
	values = read(readAction{"/MediaRenderer/RenderingControl/Control", "RenderingControl", "GetEQ", `<InstanceID>0</InstanceID><EQType>NightMode</EQType>`})
	out.NightMode = number(values, "CurrentValue")
	values = read(readAction{"/MediaRenderer/RenderingControl/Control", "RenderingControl", "GetEQ", `<InstanceID>0</InstanceID><EQType>DialogLevel</EQType>`})
	out.DialogLevel = number(values, "CurrentValue")
	values = read(readAction{"/MediaRenderer/RenderingControl/Control", "RenderingControl", "GetEQ", `<InstanceID>0</InstanceID><EQType>SurroundMode</EQType>`})
	out.SurroundMode = number(values, "CurrentValue")
	values = read(readAction{"/MediaRenderer/RenderingControl/Control", "RenderingControl", "GetEQ", `<InstanceID>0</InstanceID><EQType>SurroundLevel</EQType>`})
	out.SurroundLevel = number(values, "CurrentValue")
	values = read(readAction{"/MediaRenderer/RenderingControl/Control", "RenderingControl", "GetEQ", `<InstanceID>0</InstanceID><EQType>MusicSurroundLevel</EQType>`})
	out.MusicSurroundLevel = number(values, "CurrentValue")
	values = read(readAction{"/MediaRenderer/RenderingControl/Control", "RenderingControl", "GetEQ", `<InstanceID>0</InstanceID><EQType>SpeechEnhanceEnabled</EQType>`})
	out.SpeechEnhanceEnabled = number(values, "CurrentValue")
	values = read(readAction{"/MediaRenderer/RenderingControl/Control", "RenderingControl", "GetEQ", `<InstanceID>0</InstanceID><EQType>HeightChannelLevel</EQType>`})
	out.HeightChannelLevel = number(values, "CurrentValue")
	values = read(readAction{"/MediaRenderer/AVTransport/Control", "AVTransport", "GetTransportSettings", `<InstanceID>0</InstanceID>`})
	if value, ok := values["PlayMode"]; ok {
		out.PlayMode = &value
	}
	values = read(readAction{"/MediaRenderer/AVTransport/Control", "AVTransport", "GetTransportInfo", `<InstanceID>0</InstanceID>`})
	out.TransportSpeed = number(values, "CurrentSpeed")
	if value, ok := values["CurrentTransportStatus"]; ok {
		out.TransportStatus = &value
	}
	return out
}

var diagnosticDescs = []*prometheus.Desc{
	prometheus.NewDesc("gohome_sonos_transport_speed", "Playback speed multiplier", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_media_duration_seconds", "Loaded medium duration in seconds; absent when unknown", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_bass", "Raw bass setting (-10 to 10)", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_treble", "Raw treble setting (-10 to 10)", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_loudness", "Loudness compensation enabled (0 or 1)", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_output_fixed", "Fixed output enabled (0 or 1)", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_room_calibration_enabled", "Room calibration enabled (0 or 1)", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_room_calibration_available", "Room calibration available (0 or 1)", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_crossfade", "Crossfade enabled (0 or 1)", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_track_number", "Current track number", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_track_duration_seconds", "Track duration in seconds, absent for streams", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_track_position_seconds", "Playback position in seconds", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_queue_tracks", "Number of tracks reported by the loaded medium", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_sleep_timer_remaining_seconds", "Remaining sleep timer in seconds; absent when inactive", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_audio_input_format", "Raw Sonos HTAudioIn format code", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_surround_enabled", "Surround speakers enabled", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_sub_enabled", "Subwoofer enabled", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_sub_gain", "Raw subwoofer gain", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_audio_delay", "Raw audio delay setting", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_night_mode", "Night mode enabled", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_dialog_level", "Raw dialogue enhancement setting", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_surround_mode", "Raw surround mode", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_surround_level", "Raw TV surround level", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_music_surround_level", "Raw music surround level", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_speech_enhance_enabled", "Speech enhancement enabled", []string{"zone"}, nil),
	prometheus.NewDesc("gohome_sonos_height_channel_level", "Raw height channel level", []string{"zone"}, nil),
}
var playModeDesc = prometheus.NewDesc("gohome_sonos_play_mode", "Current playback mode", []string{"zone", "mode"}, nil)
var transportStatusDesc = prometheus.NewDesc("gohome_sonos_transport_status", "Current transport status, including faults", []string{"zone", "status"}, nil)

func emitDiagnostics(ch chan<- prometheus.Metric, out *sonosv1.ZoneDiagnostics) {
	values := []*float64{out.TransportSpeed, out.MediaDurationSeconds, out.Bass, out.Treble, out.Loudness, out.OutputFixed, out.RoomCalibrationEnabled, out.RoomCalibrationAvailable, out.Crossfade, out.TrackNumber, out.TrackDurationSeconds, out.TrackPositionSeconds, out.QueueTracks, out.SleepTimerRemainingSeconds, out.AudioInputFormat, out.SurroundEnabled, out.SubEnabled, out.SubGain, out.AudioDelay, out.NightMode, out.DialogLevel, out.SurroundMode, out.SurroundLevel, out.MusicSurroundLevel, out.SpeechEnhanceEnabled, out.HeightChannelLevel}
	for i, value := range values {
		if value != nil {
			ch <- gauge(diagnosticDescs[i], *value, out.Zone)
		}
	}
	if out.PlayMode != nil {
		ch <- gauge(playModeDesc, 1, out.Zone, *out.PlayMode)
	}
	if out.TransportStatus != nil {
		ch <- gauge(transportStatusDesc, 1, out.Zone, *out.TransportStatus)
	}
}
