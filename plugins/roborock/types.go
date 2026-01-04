package roborock

import "time"

// Device represents a Roborock device.
type Device struct {
	ID          string
	Name        string
	Model       string
	Firmware    string
	SupportsMop bool
}

// Status captures the current device status.
type Status struct {
	State                    string
	BatteryPercent           int
	ErrorCode                string
	ErrorMessage             string
	CleaningAreaSquareMeters float64
	CleaningTimeSeconds      int
	TotalCleaningTimeSeconds int
	TotalCleaningAreaSquareM float64
	TotalCleaningCount       int
	FanSpeed                 string
	MopMode                  string
	MopIntensity             string
	WaterTankAttached        bool
	MopAttached              bool
	WaterShortage            bool
	Charging                 bool
	LastCleanStart           time.Time
	LastCleanEnd             time.Time
}

// DeviceState ties device metadata with live status.
type DeviceState struct {
	Device Device
	Status Status
}

// Zone defines a cleaning zone bounding box.
type Zone struct {
	X1 int
	Y1 int
	X2 int
	Y2 int
}
