package roborock

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type readingMetric struct {
	name, help string
	value      *int64
	scale      float64
}

func statusMetrics(status Status) []readingMetric {
	r := status.Readings
	s := status.Settings
	return []readingMetric{
		{"gohome_roborock_wifi_signal_dbm", "Wi-Fi signal strength in dBm", s.WifiSignalDbm, 1},
		{"gohome_roborock_dust_collection_mode_code", "Dust collection mode code", s.DustCollectionModeCode, 1},
		{"gohome_roborock_mop_wash_mode_code", "Mop washing mode code", s.MopWashModeCode, 1},
		{"gohome_roborock_smart_wash_enabled", "Smart mop washing enabled", s.SmartWashEnabled, 1},
		{"gohome_roborock_mop_wash_interval_seconds", "Configured seconds between mop washes", s.MopWashIntervalSeconds, 1},
		{"gohome_roborock_dryer_enabled", "Mop dryer enabled", s.DryerEnabled, 1},
		{"gohome_roborock_dryer_duration_seconds", "Configured mop drying seconds", s.DryerDurationSeconds, 1},
		{"gohome_roborock_carpet_boost_enabled", "Carpet suction boost enabled", s.CarpetBoostEnabled, 1},
		{"gohome_roborock_carpet_mode_code", "Carpet cleaning mode code", s.CarpetModeCode, 1},

		{"gohome_roborock_dock_error_code", "Dock fault code", r.DockErrorCode, 1},
		{"gohome_roborock_dock_status_bits", "Raw dock sensor bitfield", r.DockStatusBits, 1},
		{"gohome_roborock_robot_status_bits", "Raw robot status bitfield", r.RobotStatusBits, 1},
		{"gohome_roborock_common_status_bits", "Raw common status bitfield", r.CommonStatusBits, 1},
		{"gohome_roborock_charge_status_code", "Charging state code", r.ChargeStatusCode, 1},
		{"gohome_roborock_dry_status_code", "Mop drying state code", r.DryStatusCode, 1},
		{"gohome_roborock_dry_remaining_seconds", "Remaining drying time in seconds", r.DryRemainingSeconds, 1},
		{"gohome_roborock_cleaning_progress_percent", "Device cleaning progress percentage", r.CleanPercent, 1},
		{"gohome_roborock_obstacles_avoided", "Obstacles avoided during this run", r.AvoidCount, 1},
		{"gohome_roborock_dust_collection_status_code", "Dust collection state code", r.DustCollectionStatusCode, 1},
		{"gohome_roborock_auto_dust_collection_enabled", "Automatic dust collection enabled", r.AutoDustCollection, 1},
		{"gohome_roborock_wash_phase_code", "Mop washing phase code", r.WashPhaseCode, 1},
		{"gohome_roborock_wash_ready", "Mop washing readiness code", r.WashReady, 1},
		{"gohome_roborock_wash_status_code", "Mop washing state code", r.WashStatusCode, 1},
		{"gohome_roborock_return_type_code", "Reason for returning to dock, device code", r.ReturnTypeCode, 1},
		{"gohome_roborock_cleaning_mode_code", "Cleaning operation code", r.InCleaningCode, 1},
		{"gohome_roborock_returning", "Returning to dock", r.InReturning, 1},
		{"gohome_roborock_warming_up", "Warming up", r.InWarmup, 1},
		{"gohome_roborock_locating", "Locating on the map", r.Locating, 1},
		{"gohome_roborock_exploring", "Exploring", r.Exploring, 1},
		{"gohome_roborock_dnd_enabled", "Do not disturb enabled", r.DndEnabled, 1},
		{"gohome_roborock_child_lock_enabled", "Child lock enabled", r.LockStatus, 1},
		{"gohome_roborock_map_present", "Map present", r.MapPresent, 1},
		{"gohome_roborock_map_status_code", "Raw map state and map ID bitfield", r.MapStatusCode, 1},
		{"gohome_roborock_unsaved_map", "Unsaved map flag", r.UnsaveMapFlag, 1},
		{"gohome_roborock_unsaved_map_reason_code", "Unsaved map reason code", r.UnsaveMapReason, 1},
		{"gohome_roborock_collision_avoidance_enabled", "Less collision mode enabled", r.CollisionAvoidStatus, 1},
		{"gohome_roborock_mop_forbidden_enabled", "Mop forbidden flag", r.MopForbiddenEnable, 1},
		{"gohome_roborock_replenish_mode_code", "Water replenishment mode code", r.ReplenishMode, 1},
		{"gohome_roborock_corner_clean_mode_code", "Corner cleaning mode code", r.CornerCleanMode, 1},
		{"gohome_roborock_cleaning_repeats", "Requested cleaning repeat count", r.Repeat, 1},
		{"gohome_roborock_total_dust_collections", "Lifetime dust collection count", status.TotalDustCollections, 1},
	}
}
func cleaningRecordMetrics(r *CleaningRecord) []readingMetric {
	if r == nil {
		return nil
	}
	return []readingMetric{
		{"gohome_roborock_cleaning_run_start_timestamp_seconds", "Start Unix timestamp", r.Begin, 1},
		{"gohome_roborock_cleaning_run_end_timestamp_seconds", "End Unix timestamp", r.End, 1},
		{"gohome_roborock_cleaning_run_duration_seconds", "Active cleaning seconds", r.Duration, 1},
		{"gohome_roborock_cleaning_run_area_square_meters", "Cleaned area in square metres", r.Area, 1e-6},
		{"gohome_roborock_cleaning_run_reported_cleaned_area_square_meters", "Device cleaned_area in square metres", r.CleanedArea, 1e-6},
		{"gohome_roborock_cleaning_run_error_code", "Run error code", r.Error, 1},
		{"gohome_roborock_cleaning_run_finished", "Whether the run finished", r.Complete, 1},
		{"gohome_roborock_cleaning_run_finish_reason_code", "Run finish reason code", r.FinishReason, 1},
		{"gohome_roborock_cleaning_run_start_type_code", "Run start type code", r.StartType, 1},
		{"gohome_roborock_cleaning_run_clean_type_code", "Run cleaning type code", r.CleanType, 1},
		{"gohome_roborock_cleaning_run_dust_collection_status_code", "Run dust collection state", r.DustCollectionStatus, 1},
		{"gohome_roborock_cleaning_run_obstacles_avoided", "Number of obstacles avoided", r.AvoidCount, 1},
		{"gohome_roborock_cleaning_run_mop_washes", "Number of mop washes", r.WashCount, 1},
		{"gohome_roborock_cleaning_run_map_id", "Map ID", r.MapFlag, 1},
		{"gohome_roborock_cleaning_run_cleaning_repeats", "Cleaning repeat count", r.CleanTimes, 1},
		{"gohome_roborock_cleaning_run_dirty_replenish_code", "Device dirty replenishment code", r.DirtyReplenish, 1},
		{"gohome_roborock_cleaning_run_manual_replenish_code", "Device manual replenishment code", r.ManualReplenish, 1},
	}
}
func emitReadings(ch chan<- prometheus.Metric, device Device, readings []readingMetric, timestamp time.Time) {
	for _, r := range readings {
		if r.value == nil {
			continue
		}
		m := prometheus.MustNewConstMetric(prometheus.NewDesc(r.name, r.help, []string{"device_id", "device_name", "model"}, nil), prometheus.GaugeValue, float64(*r.value)*r.scale, device.ID, device.Name, device.Model)
		if !timestamp.IsZero() {
			m = prometheus.NewMetricWithTimestamp(timestamp, m)
		}
		ch <- m
	}
}
