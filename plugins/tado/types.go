package tado

// Zone represents a Tado zone.
type Zone struct {
	ID   int
	Name string
}

// ZoneState holds key measurements for metrics.
type ZoneState struct {
	InsideTemperatureCelsius float64
	HumidityPercent          float64
}
