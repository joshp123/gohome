package tado

import "time"
import "encoding/json"

// Zone represents a Tado zone.
type Zone struct {
	ID      int
	Name    string
	Devices []Device
}

// Device is a Tado device in a zone, such as a radiator valve or a wireless sensor.
type Device struct {
	DeviceObservations
	SerialNumber string
	// Tado's model code, e.g. VA02 for a smart radiator thermostat.
	DeviceType string
	// Tado reports a battery only as normal or low, with no level. Nil for devices without a battery.
	BatteryLow *bool
}

// ZoneState holds key measurements for metrics.
type ZoneState struct {
	ZoneObservations
	HeatingPowerTimestamp      *time.Time
	InsideTemperatureCelsius   *float64
	InsideTemperatureTimestamp *time.Time
	HumidityPercent            *float64
	HumidityTimestamp          *time.Time
	SetpointCelsius            *float64
	HeatingPowerPercent        *float64
	PowerOn                    *bool
	OverrideActive             *bool
}

// Weather holds home-level weather measurements.
type Weather struct {
	WeatherState                string
	WeatherTimestamp            *time.Time
	OutsideTemperatureCelsius   *float64
	OutsideTemperatureTimestamp *time.Time
	SolarIntensityPercent       *float64
	SolarIntensityTimestamp     *time.Time
}

// Optional objects keep absent readings distinct from reported false values.
type DeviceObservations struct {
	ConnectionState *struct {
		Value     *bool  `json:"value"`
		Timestamp string `json:"timestamp"`
	} `json:"connectionState"`
	MountingState *struct {
		Value     string `json:"value"`
		Timestamp string `json:"timestamp"`
	} `json:"mountingState"`
	MountingStateWithError string `json:"mountingStateWithError"`
	ChildLockEnabled       *bool  `json:"childLockEnabled"`
	InPairingMode          *bool  `json:"inPairingMode"`
}

type ZoneObservations struct {
	TadoMode                       string `json:"tadoMode"`
	GeolocationOverride            *bool  `json:"geolocationOverride"`
	GeolocationOverrideDisableTime string `json:"geolocationOverrideDisableTime"`
	Preparation                    string `json:"preparation"`
	RunningOfflineSchedule         *bool  `json:"runningOfflineSchedule"`
	Link                           struct {
		State string `json:"state"`
	} `json:"link"`
	// null means no detection/override; an omitted field is unknown.
	OpenWindow  json.RawMessage `json:"openWindow"`
	OverlayType json.RawMessage `json:"overlayType"`
	Overlay     *struct {
		Termination struct {
			Type                   string   `json:"type"`
			RemainingTimeInSeconds *float64 `json:"remainingTimeInSeconds"`
			DurationInSeconds      *float64 `json:"durationInSeconds"`
			ProjectedExpiry        string   `json:"projectedExpiry"`
		} `json:"termination"`
	} `json:"overlay"`
}
