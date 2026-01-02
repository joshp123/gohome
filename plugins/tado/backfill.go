package tado

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const DefaultImportURL = "http://127.0.0.1:8428/vm/api/v1/import/prometheus"

var ErrDayReportNotFound = errors.New("tado day report not found")

type BackfillOptions struct {
	StartDate time.Time
	EndDate   time.Time
	Zones     []string

	ImportURL string
	BatchSize int
	Throttle  time.Duration
}

type sample struct {
	Name      string
	Labels    map[string]string
	Value     float64
	Timestamp time.Time
}

func Backfill(ctx context.Context, client *Client, opts BackfillOptions) error {
	if client == nil {
		return fmt.Errorf("tado client is required")
	}
	if opts.ImportURL == "" {
		opts.ImportURL = DefaultImportURL
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 5000
	}
	if opts.Throttle < 0 {
		opts.Throttle = 0
	}

	homeID, err := client.HomeID(ctx)
	if err != nil {
		return err
	}
	zones, err := client.Zones(ctx)
	if err != nil {
		return err
	}
	filtered := filterZones(zones, opts.Zones)
	if len(filtered) == 0 {
		return fmt.Errorf("no zones matched filter")
	}

	start := time.Date(opts.StartDate.Year(), opts.StartDate.Month(), opts.StartDate.Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(opts.EndDate.Year(), opts.EndDate.Month(), opts.EndDate.Day(), 0, 0, 0, 0, time.UTC)

	var buf []sample
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		for _, zone := range filtered {
			report, err := client.DayReport(ctx, homeID, zone.ID, day)
			if err != nil {
				if errors.Is(err, ErrDayReportNotFound) {
					continue
				}
				return err
			}
			buf = append(buf, dayReportSamples(report, zone)...)
			if len(buf) >= opts.BatchSize {
				if err := importSamples(ctx, opts.ImportURL, buf); err != nil {
					return err
				}
				buf = buf[:0]
			}
			if opts.Throttle > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(opts.Throttle):
				}
			}
		}
	}

	if len(buf) > 0 {
		if err := importSamples(ctx, opts.ImportURL, buf); err != nil {
			return err
		}
	}
	return nil
}

func filterZones(zones []Zone, names []string) []Zone {
	if len(names) == 0 {
		return zones
	}
	lookup := map[string]struct{}{}
	for _, name := range names {
		lookup[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	filtered := make([]Zone, 0, len(zones))
	for _, zone := range zones {
		if _, ok := lookup[strings.ToLower(zone.Name)]; ok {
			filtered = append(filtered, zone)
		}
	}
	return filtered
}

func dayReportSamples(report dayReport, zone Zone) []sample {
	var samples []sample
	labels := map[string]string{
		"zone_id":   strconv.Itoa(zone.ID),
		"zone_name": zone.Name,
	}

	for _, pt := range report.MeasuredData.InsideTemperature.DataPoints {
		ts, err := parseDayReportTime(pt.Timestamp)
		if err != nil {
			continue
		}
		samples = append(samples, sample{
			Name:      "gohome_tado_inside_temperature_celsius",
			Labels:    labels,
			Value:     pt.Value.Celsius,
			Timestamp: ts,
		})
	}

	for _, pt := range report.MeasuredData.Humidity.DataPoints {
		ts, err := parseDayReportTime(pt.Timestamp)
		if err != nil {
			continue
		}
		value := pt.Value.Percentage
		if value <= 1 {
			value *= 100
		}
		samples = append(samples, sample{
			Name:      "gohome_tado_humidity_percent",
			Labels:    labels,
			Value:     value,
			Timestamp: ts,
		})
	}

	for _, iv := range report.CallForHeat.DataIntervals {
		ts, err := parseDayReportTime(iv.From)
		if err != nil {
			continue
		}
		power := callForHeatToPercent(iv.Value)
		samples = append(samples, sample{
			Name:      "gohome_tado_heating_power_percent",
			Labels:    labels,
			Value:     power,
			Timestamp: ts,
		})
		active := 0.0
		if power > 0 {
			active = 1
		}
		samples = append(samples, sample{
			Name:      "gohome_tado_heating_active_bool",
			Labels:    labels,
			Value:     active,
			Timestamp: ts,
		})
	}

	for _, iv := range report.Settings.DataIntervals {
		ts, err := parseDayReportTime(iv.From)
		if err != nil {
			continue
		}
		if iv.Value.Power != "" {
			value := 0.0
			if strings.EqualFold(iv.Value.Power, "ON") {
				value = 1
			}
			samples = append(samples, sample{
				Name:      "gohome_tado_power_on_bool",
				Labels:    labels,
				Value:     value,
				Timestamp: ts,
			})
		}
		if iv.Value.Temperature.Celsius != nil {
			samples = append(samples, sample{
				Name:      "gohome_tado_setpoint_celsius",
				Labels:    labels,
				Value:     *iv.Value.Temperature.Celsius,
				Timestamp: ts,
			})
		}
	}

	for _, iv := range report.Weather.Condition.DataIntervals {
		ts, err := parseDayReportTime(iv.From)
		if err != nil {
			continue
		}
		if iv.Value.Temperature.Celsius != nil {
			samples = append(samples, sample{
				Name:      "gohome_tado_outside_temperature_celsius",
				Value:     *iv.Value.Temperature.Celsius,
				Timestamp: ts,
			})
		}
	}

	for _, iv := range report.Weather.Sunny.DataIntervals {
		ts, err := parseDayReportTime(iv.From)
		if err != nil {
			continue
		}
		value := 0.0
		if iv.Value {
			value = 100
		}
		samples = append(samples, sample{
			Name:      "gohome_tado_solar_intensity_percent",
			Value:     value,
			Timestamp: ts,
		})
	}

	return samples
}

func callForHeatToPercent(value string) float64 {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "HIGH":
		return 100
	case "LOW":
		return 50
	default:
		return 0
	}
}

func importSamples(ctx context.Context, importURL string, samples []sample) error {
	if len(samples) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, s := range samples {
		buf.WriteString(s.Name)
		if len(s.Labels) > 0 {
			buf.WriteString("{")
			first := true
			for k, v := range s.Labels {
				if !first {
					buf.WriteString(",")
				}
				first = false
				buf.WriteString(k)
				buf.WriteString("=\"")
				buf.WriteString(escapeLabelValue(v))
				buf.WriteString("\"")
			}
			buf.WriteString("}")
		}
		buf.WriteString(" ")
		buf.WriteString(strconv.FormatFloat(s.Value, 'f', -1, 64))
		buf.WriteString(" ")
		buf.WriteString(strconv.FormatInt(s.Timestamp.Unix()*1000, 10))
		buf.WriteString("\n")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, importURL, strings.NewReader(buf.String()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain; version=0.0.4")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("tado backfill import http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func escapeLabelValue(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return value
}
