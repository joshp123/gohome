package tado

import "time"

// Zone represents a Tado zone.
type Zone struct {
	ID   int
	Name string
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
