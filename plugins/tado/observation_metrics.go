package tado

import (
	"encoding/json"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// All these observations come from the three existing collection requests.
var observationDescriptors = map[string]*prometheus.Desc{}

func observationDescriptor(name, help string, labels ...string) *prometheus.Desc {
	desc := prometheus.NewDesc("gohome_tado_"+name, help, labels, nil)
	observationDescriptors[name] = desc
	return desc
}

var (
	zoneLink                  = observationDescriptor("zone_connection_state", "Reported zone connection state", "zone_id", "zone_name", "state")
	zonePresence              = observationDescriptor("zone_presence", "Tado HOME/AWAY mode, not a physical occupancy measurement", "zone_id", "zone_name", "mode")
	zoneOverlay               = observationDescriptor("zone_overlay_type", "Reported override type; NONE for explicit null", "zone_id", "zone_name", "type")
	zonePreparation           = observationDescriptor("zone_preparation", "Reported preparation state", "zone_id", "zone_name", "state")
	zoneGeoOverride           = observationDescriptor("zone_geolocation_override", "Geolocation override enabled (0/1)", "zone_id", "zone_name")
	zoneGeoExpiry             = observationDescriptor("zone_geolocation_override_disable_timestamp_seconds", "Geolocation override disable time (Unix seconds)", "zone_id", "zone_name")
	zoneOfflineSchedule       = observationDescriptor("zone_running_offline_schedule", "Running the offline schedule (0/1)", "zone_id", "zone_name")
	zoneOpenWindow            = observationDescriptor("zone_open_window_detected", "Tado detects an open window (0/1); not a contact sensor", "zone_id", "zone_name")
	zoneOpenWindowDuration    = observationDescriptor("zone_open_window_duration_seconds", "Reported open-window suspension duration (seconds)", "zone_id", "zone_name")
	zoneOpenWindowRemaining   = observationDescriptor("zone_open_window_remaining_seconds", "Remaining open-window suspension (seconds)", "zone_id", "zone_name")
	zoneOpenWindowExpiry      = observationDescriptor("zone_open_window_expiry_timestamp_seconds", "Open-window suspension expiry (Unix seconds)", "zone_id", "zone_name")
	zoneOpenWindowStart       = observationDescriptor("zone_open_window_detected_timestamp_seconds", "Open-window detection time (Unix seconds)", "zone_id", "zone_name")
	zoneOverlayTermination    = observationDescriptor("zone_overlay_termination", "Reported override termination type", "zone_id", "zone_name", "type")
	zoneOverlayDuration       = observationDescriptor("zone_overlay_duration_seconds", "Reported override duration (seconds)", "zone_id", "zone_name")
	zoneOverlayRemaining      = observationDescriptor("zone_overlay_remaining_seconds", "Remaining override duration (seconds)", "zone_id", "zone_name")
	zoneOverlayExpiry         = observationDescriptor("zone_overlay_expiry_timestamp_seconds", "Projected override expiry (Unix seconds)", "zone_id", "zone_name")
	humidityTimestamp         = observationDescriptor("humidity_last_updated_timestamp_seconds", "Humidity observation time (Unix seconds)", "zone_id", "zone_name")
	heatingTimestamp          = observationDescriptor("heating_power_last_updated_timestamp_seconds", "Heating demand observation time (Unix seconds)", "zone_id", "zone_name")
	deviceConnected           = observationDescriptor("device_connected", "Device connection state (0/1)", "zone_name", "serial", "device_type")
	deviceConnectionTimestamp = observationDescriptor("device_connection_timestamp_seconds", "Device connection state observation time (Unix seconds)", "zone_name", "serial", "device_type")
	deviceMounting            = observationDescriptor("device_mounting_state", "Device mounting state", "zone_name", "serial", "device_type", "state")
	deviceMountingError       = observationDescriptor("device_mounting_state_with_error", "Device mounting/error state", "zone_name", "serial", "device_type", "state")
	deviceMountingTimestamp   = observationDescriptor("device_mounting_timestamp_seconds", "Device mounting state observation time (Unix seconds)", "zone_name", "serial", "device_type")
	deviceChildLock           = observationDescriptor("device_child_lock_enabled", "Device child lock enabled (0/1)", "zone_name", "serial", "device_type")
	devicePairing             = observationDescriptor("device_pairing_mode", "Device pairing mode (0/1)", "zone_name", "serial", "device_type")
	weatherState              = observationDescriptor("weather_state", "Reported weather condition", "state")
	weatherTimestamp          = observationDescriptor("weather_last_updated_timestamp_seconds", "Weather condition observation time (Unix seconds)")
	outsideTimestamp          = observationDescriptor("outside_temperature_last_updated_timestamp_seconds", "Outdoor temperature observation time (Unix seconds)")
	solarTimestamp            = observationDescriptor("solar_intensity_last_updated_timestamp_seconds", "Solar intensity observation time (Unix seconds)")
)

func emitObservation(ch chan<- prometheus.Metric, desc *prometheus.Desc, value float64, labels ...string) {
	ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, value, labels...)
}
func emitBoolean(ch chan<- prometheus.Metric, desc *prometheus.Desc, value *bool, labels ...string) {
	if value != nil {
		emitObservation(ch, desc, boolToFloat(*value), labels...)
	}
}
func emitNumber(ch chan<- prometheus.Metric, desc *prometheus.Desc, value *float64, labels ...string) {
	if value != nil {
		emitObservation(ch, desc, *value, labels...)
	}
}
func emitTimestamp(ch chan<- prometheus.Metric, desc *prometheus.Desc, value *time.Time, labels ...string) {
	if value != nil {
		emitObservation(ch, desc, float64(value.Unix()), labels...)
	}
}
func emitState(ch chan<- prometheus.Metric, desc *prometheus.Desc, value string, labels ...string) {
	if value != "" {
		emitObservation(ch, desc, 1, append(labels, value)...)
	}
}

func collectZoneObservations(ch chan<- prometheus.Metric, state ZoneState, labels ...string) {
	emitState(ch, zoneLink, state.Link.State, labels...)
	emitState(ch, zonePresence, state.TadoMode, labels...)
	emitState(ch, zonePreparation, state.Preparation, labels...)
	emitBoolean(ch, zoneGeoOverride, state.GeolocationOverride, labels...)
	emitBoolean(ch, zoneOfflineSchedule, state.RunningOfflineSchedule, labels...)
	emitTimestamp(ch, zoneGeoExpiry, parseTimestamp(state.GeolocationOverrideDisableTime), labels...)
	emitTimestamp(ch, humidityTimestamp, state.HumidityTimestamp, labels...)
	emitTimestamp(ch, heatingTimestamp, state.HeatingPowerTimestamp, labels...)
	if len(state.OverlayType) > 0 {
		kind := "NONE"
		if string(state.OverlayType) != "null" {
			if err := json.Unmarshal(state.OverlayType, &kind); err != nil {
				kind = ""
			}
		}
		emitState(ch, zoneOverlay, kind, labels...)
	}
	if state.Overlay != nil {
		termination := state.Overlay.Termination
		emitState(ch, zoneOverlayTermination, termination.Type, labels...)
		emitNumber(ch, zoneOverlayDuration, termination.DurationInSeconds, labels...)
		emitNumber(ch, zoneOverlayRemaining, termination.RemainingTimeInSeconds, labels...)
		emitTimestamp(ch, zoneOverlayExpiry, parseTimestamp(termination.ProjectedExpiry), labels...)
	}
	if len(state.OpenWindow) > 0 {
		if string(state.OpenWindow) == "null" {
			emitObservation(ch, zoneOpenWindow, 0, labels...)
		} else {
			var window struct {
				RemainingTimeInSeconds *float64 `json:"remainingTimeInSeconds"`
				DurationInSeconds      *float64 `json:"durationInSeconds"`
				DetectedTime           string   `json:"detectedTime"`
				Expiry                 string   `json:"expiry"`
			}
			if json.Unmarshal(state.OpenWindow, &window) == nil {
				emitObservation(ch, zoneOpenWindow, 1, labels...)
				emitNumber(ch, zoneOpenWindowDuration, window.DurationInSeconds, labels...)
				emitNumber(ch, zoneOpenWindowRemaining, window.RemainingTimeInSeconds, labels...)
				emitTimestamp(ch, zoneOpenWindowStart, parseTimestamp(window.DetectedTime), labels...)
				emitTimestamp(ch, zoneOpenWindowExpiry, parseTimestamp(window.Expiry), labels...)
			}
		}
	}
}

func collectDeviceObservations(ch chan<- prometheus.Metric, device Device, zoneName string) {
	labels := []string{zoneName, device.SerialNumber, device.DeviceType}
	if state := device.ConnectionState; state != nil {
		emitBoolean(ch, deviceConnected, state.Value, labels...)
		emitTimestamp(ch, deviceConnectionTimestamp, parseTimestamp(state.Timestamp), labels...)
	}
	if state := device.MountingState; state != nil {
		emitState(ch, deviceMounting, state.Value, labels...)
		emitTimestamp(ch, deviceMountingTimestamp, parseTimestamp(state.Timestamp), labels...)
	}
	emitState(ch, deviceMountingError, device.MountingStateWithError, labels...)
	emitBoolean(ch, deviceChildLock, device.ChildLockEnabled, labels...)
	emitBoolean(ch, devicePairing, device.InPairingMode, labels...)
}
