package daikin

import "github.com/prometheus/client_golang/prometheus"

type stringValue struct {
	Value string `json:"value"`
}
type boolValue struct {
	Value bool `json:"value"`
}
type numberValue struct {
	Value *float64 `json:"value"`
}

type fanControl struct {
	Value struct {
		OperationModes map[string]struct {
			FanSpeed *struct {
				CurrentMode *stringValue           `json:"currentMode"`
				Modes       map[string]numberValue `json:"modes"`
			} `json:"fanSpeed"`
			FanDirection map[string]struct {
				CurrentMode *stringValue `json:"currentMode"`
			} `json:"fanDirection"`
		} `json:"operationModes"`
	} `json:"value"`
}

type scheduleState struct {
	Value struct {
		CurrentMode *stringValue `json:"currentMode"`
		Modes       map[string]struct {
			Enabled         *boolValue   `json:"enabled"`
			CurrentSchedule *stringValue `json:"currentSchedule"`
		} `json:"modes"`
	} `json:"value"`
}

var additionalDescriptors = map[string]*prometheus.Desc{
	"device_timestamp_seconds": extraDesc("device_timestamp_seconds", "Timestamp of the gateway device response"),
	"error_code_info":          extraDesc("error_code_info", "Raw device error code (including the no-error code)", "code"),
	"firmware_info":            extraDesc("firmware_info", "Reported firmware and software versions", "firmware", "software", "model"),
	"powerful_mode":            extraDesc("powerful_mode", "Configured powerful mode (1=on, 0=off)"),
	"powerful_mode_active":     extraDesc("powerful_mode_active", "Whether powerful mode is active"),
	"holiday_mode_enabled":     extraDesc("holiday_mode_enabled", "Whether holiday mode is enabled"),
	"fan_mode":                 extraDesc("fan_mode", "Configured fan speed mode for each operation mode (1=selected)", "operation_mode", "mode"),
	"fan_speed_level":          extraDesc("fan_speed_level", "Configured fan speed level; device scale, not RPM or percent", "operation_mode", "mode"),
	"fan_direction":            extraDesc("fan_direction", "Configured fan direction for each operation mode (1=selected)", "operation_mode", "axis", "direction"),
	"schedule_mode":            extraDesc("schedule_mode", "Selected schedule mode (1=selected)", "mode"),
	"schedule_enabled":         extraDesc("schedule_enabled", "Whether scheduling is enabled for this mode", "mode"),
	"schedule_selected":        extraDesc("schedule_selected", "Selected schedule identifier (1=selected)", "mode", "schedule"),
}

func extraDesc(name, help string, labels ...string) *prometheus.Desc {
	return prometheus.NewDesc("gohome_daikin_"+name, help, append([]string{"unit_id", "unit_name", "embedded_id"}, labels...), nil)
}

func collectAdditional(ch chan<- prometheus.Metric, state DeviceState) {
	for _, mp := range state.ManagementPoints {
		emit := func(name string, value float64, labels ...string) {
			ch <- prometheus.MustNewConstMetric(additionalDescriptors[name], prometheus.GaugeValue, value, append([]string{state.Device.ID, state.Device.Name, mp.EmbeddedID}, labels...)...)
		}
		if !state.Timestamp.IsZero() {
			emit("device_timestamp_seconds", float64(state.Timestamp.Unix()))
		}
		if mp.ErrorCode != nil {
			emit("error_code_info", 1, mp.ErrorCode.Value)
		}
		if mp.FirmwareVersion != nil || mp.SoftwareVersion != nil || mp.ModelInfo != nil {
			value := func(v *stringValue) string {
				if v == nil {
					return ""
				}
				return v.Value
			}
			emit("firmware_info", 1, value(mp.FirmwareVersion), value(mp.SoftwareVersion), value(mp.ModelInfo))
		}
		if mp.PowerfulMode != nil {
			if mp.PowerfulMode.Value == "on" {
				emit("powerful_mode", 1)
			} else if mp.PowerfulMode.Value == "off" {
				emit("powerful_mode", 0)
			}
		}
		if mp.IsPowerfulModeActive != nil {
			emit("powerful_mode_active", boolToFloat(mp.IsPowerfulModeActive.Value))
		}
		if mp.HolidayMode != nil && mp.HolidayMode.Value.Enabled != nil {
			emit("holiday_mode_enabled", boolToFloat(*mp.HolidayMode.Value.Enabled))
		}
		if mp.FanControl != nil {
			for mode, fan := range mp.FanControl.Value.OperationModes {
				if fan.FanSpeed != nil {
					if fan.FanSpeed.CurrentMode != nil {
						emit("fan_mode", 1, mode, fan.FanSpeed.CurrentMode.Value)
					}
					for speed, value := range fan.FanSpeed.Modes {
						if value.Value != nil {
							emit("fan_speed_level", *value.Value, mode, speed)
						}
					}
				}
				for axis, direction := range fan.FanDirection {
					if direction.CurrentMode != nil {
						emit("fan_direction", 1, mode, axis, direction.CurrentMode.Value)
					}
				}
			}
		}
		if mp.Schedule != nil {
			if mp.Schedule.Value.CurrentMode != nil {
				emit("schedule_mode", 1, mp.Schedule.Value.CurrentMode.Value)
			}
			for mode, schedule := range mp.Schedule.Value.Modes {
				if schedule.Enabled != nil {
					emit("schedule_enabled", boolToFloat(schedule.Enabled.Value), mode)
				}
				if schedule.CurrentSchedule != nil {
					emit("schedule_selected", 1, mode, schedule.CurrentSchedule.Value)
				}
			}
		}
	}
}
