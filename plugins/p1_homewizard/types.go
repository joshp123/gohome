package p1_homewizard

import (
	"regexp"
	"strconv"
	"time"
)

type Info struct {
	ProductName    string  `json:"product_name"`
	ProductType    string  `json:"product_type"`
	Serial         string  `json:"serial"`
	Firmware       string  `json:"firmware_version"`
	APIVersion     string  `json:"api_version"`
	MeterModel     string  `json:"meter_model"`
	UniqueID       string  `json:"unique_id"`
	WifiSSID       string  `json:"wifi_ssid"`
	WifiStrength   int     `json:"wifi_strength"`
	SMRVersion     int     `json:"smr_version"`
	ActiveTariff   int     `json:"active_tariff"`
	TotalImportKWh float64 `json:"total_power_import_kwh"`
	TotalExportKWh float64 `json:"total_power_export_kwh"`
}

type Data struct {
	SMRVersion          *int     `json:"smr_version"`
	MeterModel          *string  `json:"meter_model"`
	WifiStrengthPercent *float64 `json:"wifi_strength"`
	ActiveTariff        *int     `json:"active_tariff"`
	TotalImportKWh      *float64 `json:"total_power_import_kwh"`
	TotalImportT1KWh    *float64 `json:"total_power_import_t1_kwh"`
	TotalImportT2KWh    *float64 `json:"total_power_import_t2_kwh"`
	TotalExportKWh      *float64 `json:"total_power_export_kwh"`
	TotalExportT1KWh    *float64 `json:"total_power_export_t1_kwh"`
	TotalExportT2KWh    *float64 `json:"total_power_export_t2_kwh"`
	ActivePowerW        *float64 `json:"active_power_w"`
	ActivePowerL1W      *float64 `json:"active_power_l1_w"`
	ActiveCurrentA      *float64 `json:"active_current_a"`
	ActiveCurrentL1A    *float64 `json:"active_current_l1_a"`
	VoltageSagL1Count   *float64 `json:"voltage_sag_l1_count"`
	VoltageSwellL1Count *float64 `json:"voltage_swell_l1_count"`
	AnyPowerFailCount   *float64 `json:"any_power_fail_count"`
	LongPowerFailCount  *float64 `json:"long_power_fail_count"`
}

type PowerFailureEvent struct {
	RestoredAt      time.Time
	DurationSeconds float64
}

type TelegramMetrics struct {
	ImportPowerW       *float64
	ExportPowerW       *float64
	Timestamp          *time.Time
	PowerFailureEvents []PowerFailureEvent
}

var (
	telegramImportRe            = regexp.MustCompile(`1-0:1\.7\.0\(([-0-9.]+)\*kW\)`)
	telegramExportRe            = regexp.MustCompile(`1-0:2\.7\.0\(([-0-9.]+)\*kW\)`)
	telegramTimeRe              = regexp.MustCompile(`0-0:1\.0\.0\((\d{12})([SW])\)`)
	telegramPowerFailureLogRe   = regexp.MustCompile(`1-0:99\.97\.0\([^\r\n]+`)
	telegramPowerFailureEntryRe = regexp.MustCompile(`\((\d{12})([SW])\)\((\d+)\*s\)`)
)

func p1TelegramLocation() *time.Location {
	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		return time.Local
	}
	return loc
}

func ParseTelegram(raw string) (TelegramMetrics, bool) {
	metrics := TelegramMetrics{}
	ok := false
	loc := p1TelegramLocation()

	if match := telegramImportRe.FindStringSubmatch(raw); len(match) == 2 {
		if value, err := strconv.ParseFloat(match[1], 64); err == nil {
			watts := value * 1000
			metrics.ImportPowerW = &watts
			ok = true
		}
	}

	if match := telegramExportRe.FindStringSubmatch(raw); len(match) == 2 {
		if value, err := strconv.ParseFloat(match[1], 64); err == nil {
			watts := value * 1000
			metrics.ExportPowerW = &watts
			ok = true
		}
	}

	if match := telegramTimeRe.FindStringSubmatch(raw); len(match) == 3 {
		if parsed, err := time.ParseInLocation("060102150405", match[1], loc); err == nil {
			metrics.Timestamp = &parsed
			ok = true
		}
	}

	if line := telegramPowerFailureLogRe.FindString(raw); line != "" {
		matches := telegramPowerFailureEntryRe.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			if len(m) != 4 {
				continue
			}
			restoredAt, err := time.ParseInLocation("060102150405", m[1], loc)
			if err != nil {
				continue
			}
			durationSeconds, err := strconv.ParseFloat(m[3], 64)
			if err != nil {
				continue
			}
			metrics.PowerFailureEvents = append(metrics.PowerFailureEvents, PowerFailureEvent{
				RestoredAt:      restoredAt,
				DurationSeconds: durationSeconds,
			})
			ok = true
		}
	}

	return metrics, ok
}
