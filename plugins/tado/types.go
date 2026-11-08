package tado

import "time"

// Zone represents a Tado zone.
type Zone struct {
	ID      int
	Name    string
	Devices []Device
}

// Device is a Tado device in a zone, such as a radiator valve or a wireless sensor.
type Device struct {
	SerialNumber string
	// Tado's model code, e.g. VA02 for a smart radiator thermostat.
	DeviceType string
	// Tado reports a battery only as normal or low, with no level. Nil for devices without a battery.
	BatteryLow *bool
}

// ZoneState holds key measurements for metrics.
type ZoneState struct {
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
	OutsideTemperatureCelsius   *float64
	OutsideTemperatureTimestamp *time.Time
	SolarIntensityPercent       *float64
	SolarIntensityTimestamp     *time.Time
}
