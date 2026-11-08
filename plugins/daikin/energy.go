package daikin

import (
	"encoding/json"
	"fmt"
	"math"
	"time"
	_ "time/tzdata"
)

type consumptionData struct {
	Value struct {
		Electrical *struct {
			Unit    string        `json:"unit"`
			Heating energyPeriods `json:"heating"`
			Cooling energyPeriods `json:"cooling"`
		} `json:"electrical"`
	} `json:"value"`
}

type energyPeriods struct {
	Day   []*float64 `json:"d"`
	Week  []*float64 `json:"w"`
	Month []*float64 `json:"m"`
}

// EnergyReading is one unmodified API bucket, not a counter or estimated power.
type EnergyReading struct {
	UnitID     string
	UnitName   string
	EmbeddedID string
	Mode       string
	Resolution string
	Slot       int
	KWh        float64
	Start      time.Time
	End        time.Time
}

// DecodeEnergyReadings accepts the payload returned by GetUnitState.
func DecodeEnergyReadings(payload []byte) ([]EnergyReading, time.Time, error) {
	var entry deviceStateEntry
	if err := json.Unmarshal(payload, &entry); err != nil {
		return nil, time.Time{}, err
	}
	readings, err := entry.toDeviceState().energyReadings()
	return readings, entry.Timestamp, err
}

func (s DeviceState) energyReadings() ([]EnergyReading, error) {
	if s.Timestamp.IsZero() {
		return nil, fmt.Errorf("energy has no gateway timestamp")
	}
	zone := ""
	for _, mp := range s.ManagementPoints {
		if mp.TimeZone != nil {
			zone = mp.TimeZone.Value
		}
	}
	if zone == "" {
		return nil, fmt.Errorf("energy has no gateway time zone")
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return nil, fmt.Errorf("energy time zone: %w", err)
	}
	at := s.Timestamp.In(location)
	today := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, location)
	monday := today.AddDate(0, 0, -(int(today.Weekday())+6)%7)
	var readings []EnergyReading
	for _, mp := range s.ManagementPoints {
		if mp.ConsumptionData == nil || mp.ConsumptionData.Value.Electrical == nil {
			continue
		}
		electrical := mp.ConsumptionData.Value.Electrical
		if electrical.Unit != "kWh" {
			return nil, fmt.Errorf("unsupported electrical unit %q", electrical.Unit)
		}
		for _, mode := range []struct {
			name    string
			periods energyPeriods
		}{{"heating", electrical.Heating}, {"cooling", electrical.Cooling}} {
			for _, series := range []struct {
				resolution string
				values     []*float64
				count      int
			}{{"2h", mode.periods.Day, 24}, {"day", mode.periods.Week, 14}, {"month", mode.periods.Month, 24}} {
				if len(series.values) == 0 {
					continue
				}
				if len(series.values) != series.count {
					return nil, fmt.Errorf("unexpected %s energy array length %d", series.resolution, len(series.values))
				}
				for slot, value := range series.values {
					if value == nil {
						continue
					}
					if *value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0) {
						return nil, fmt.Errorf("invalid energy value")
					}
					var start, end time.Time
					switch series.resolution {
					case "2h":
						day := today.AddDate(0, 0, slot/12-1)
						// The fixed 12-slot format cannot identify the repeated/missing DST hour.
						if day.AddDate(0, 0, 1).Sub(day) != 24*time.Hour {
							continue
						}
						start = day.Add(time.Duration(slot%12) * 2 * time.Hour)
						end = start.Add(2 * time.Hour)
					case "day":
						start = monday.AddDate(0, 0, slot-7)
						end = start.AddDate(0, 0, 1)
					case "month":
						start = time.Date(at.Year()-1, time.Month(slot+1), 1, 0, 0, 0, 0, location)
						end = start.AddDate(0, 1, 0)
					}
					if start.After(at) {
						continue
					}
					readings = append(readings, EnergyReading{s.Device.ID, s.Device.Name, mp.EmbeddedID, mode.name, series.resolution, slot, *value, start, end})
				}
			}
		}
	}
	return readings, nil
}
