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
	Settings                 OperationalSettings
	Readings                 StatusReadings
	LastRecord               *CleaningRecord
	TotalDustCollections     *int64
	Consumables              Consumables
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
	// LastCleanFinished is false when the last run stopped before covering
	// everything it set out to clean, for example when it was interrupted.
	LastCleanFinished bool
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

// Consumables contains device counters since their last maintenance reset.
type Consumables struct {
	MainBrush      *int64 `json:"main_brush_work_time"`
	SideBrush      *int64 `json:"side_brush_work_time"`
	Filter         *int64 `json:"filter_work_time"`
	FilterElement  *int64 `json:"filter_element_work_time"`
	Sensors        *int64 `json:"sensor_dirty_time"`
	MopRoller      *int64 `json:"moproller_work_time"`
	Strainer       *int64 `json:"strainer_work_times"`
	DustCollection *int64 `json:"dust_collection_work_times"`
	CleaningBrush  *int64 `json:"cleaning_brush_work_times"`
}

type StatusReadings struct {
	BumperStatus             []int64 `json:"adbumper_status"`
	DockErrorCode            *int64  `json:"dock_error_status"`
	DockStatusBits           *int64  `json:"dss"`
	RobotStatusBits          *int64  `json:"rss"`
	CommonStatusBits         *int64  `json:"common_status"`
	ChargeStatusCode         *int64  `json:"charge_status"`
	DryStatusCode            *int64  `json:"dry_status"`
	DryRemainingSeconds      *int64  `json:"rdt"`
	CleanPercent             *int64  `json:"clean_percent"`
	AvoidCount               *int64  `json:"avoid_count"`
	DustCollectionStatusCode *int64  `json:"dust_collection_status"`
	AutoDustCollection       *int64  `json:"auto_dust_collection"`
	WashPhaseCode            *int64  `json:"wash_phase"`
	WashReady                *int64  `json:"wash_ready"`
	WashStatusCode           *int64  `json:"wash_status"`
	ReturnTypeCode           *int64  `json:"back_type"`
	InCleaningCode           *int64  `json:"in_cleaning"`
	InReturning              *int64  `json:"in_returning"`
	InWarmup                 *int64  `json:"in_warmup"`
	Locating                 *int64  `json:"is_locating"`
	Exploring                *int64  `json:"is_exploring"`
	DndEnabled               *int64  `json:"dnd_enabled"`
	LockStatus               *int64  `json:"lock_status"`
	MapPresent               *int64  `json:"map_present"`
	MapStatusCode            *int64  `json:"map_status"`
	UnsaveMapFlag            *int64  `json:"unsave_map_flag"`
	UnsaveMapReason          *int64  `json:"unsave_map_reason"`
	CollisionAvoidStatus     *int64  `json:"collision_avoid_status"`
	MopForbiddenEnable       *int64  `json:"mop_forbidden_enable"`
	ReplenishMode            *int64  `json:"replenish_mode"`
	CornerCleanMode          *int64  `json:"corner_clean_mode"`
	Repeat                   *int64  `json:"repeat"`
}

type CleaningRecord struct {
	Begin                *int64 `json:"begin"`
	End                  *int64 `json:"end"`
	Duration             *int64 `json:"duration"`
	Area                 *int64 `json:"area"`
	CleanedArea          *int64 `json:"cleaned_area"`
	Error                *int64 `json:"error"`
	Complete             *int64 `json:"complete"`
	FinishReason         *int64 `json:"finish_reason"`
	StartType            *int64 `json:"start_type"`
	CleanType            *int64 `json:"clean_type"`
	DustCollectionStatus *int64 `json:"dust_collection_status"`
	AvoidCount           *int64 `json:"avoid_count"`
	WashCount            *int64 `json:"wash_count"`
	MapFlag              *int64 `json:"map_flag"`
	CleanTimes           *int64 `json:"clean_times"`
	DirtyReplenish       *int64 `json:"dirty_replenish"`
	ManualReplenish      *int64 `json:"manual_replenish"`
}

type OperationalSettings struct {
	WifiSignalDbm          *int64
	DustCollectionModeCode *int64
	MopWashModeCode        *int64
	SmartWashEnabled       *int64
	MopWashIntervalSeconds *int64
	DryerEnabled           *int64
	DryerDurationSeconds   *int64
	CarpetBoostEnabled     *int64
	CarpetModeCode         *int64
}
