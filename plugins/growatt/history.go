package growatt

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	victoriaImportURL = "http://127.0.0.1:8428/vm/api/v1/import/prometheus"
	growattHistoryWeeks = 52
)

// EnergyPoint represents a dated energy reading.
type EnergyPoint struct {
	Timestamp time.Time
	EnergyKWh float64
	Period    string
}

func (c *Client) EnergyHistory(ctx context.Context, plantID int64, start, end time.Time, unit string) ([]EnergyPoint, error) {
	var data struct {
		Energys []map[string]any `json:"energys"`
	}

	params := map[string]string{
		"plant_id":   strconv.FormatInt(plantID, 10),
		"start_date": start.Format("2006-01-02"),
		"end_date":   end.Format("2006-01-02"),
		"time_unit":  unit,
		"page":       "1",
		"perpage":    "100",
	}

	if err := c.getJSON(ctx, "plant/energy", params, &data); err != nil {
		return nil, err
	}

	points := make([]EnergyPoint, 0, len(data.Energys))
	for _, entry := range data.Energys {
		ts, err := parseEnergyDate(unit, entry["date"])
		if err != nil {
			continue
		}
		points = append(points, EnergyPoint{
			Timestamp: ts,
			EnergyKWh: parseFloat(entry["energy"]),
			Period:    unit,
		})
	}

	return points, nil
}

func parseEnergyDate(unit string, value any) (time.Time, error) {
	switch unit {
	case "day":
		text, ok := value.(string)
		if !ok {
			return time.Time{}, fmt.Errorf("unexpected day date %T", value)
		}
		t, err := time.ParseInLocation("2006-01-02", text, time.Local)
		if err != nil {
			return time.Time{}, err
		}
		return time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, time.Local), nil
	case "month":
		text, ok := value.(string)
		if !ok {
			return time.Time{}, fmt.Errorf("unexpected month date %T", value)
		}
		t, err := time.ParseInLocation("2006-01", text, time.Local)
		if err != nil {
			return time.Time{}, err
		}
		return time.Date(t.Year(), t.Month(), 1, 12, 0, 0, 0, time.Local), nil
	case "year":
		switch v := value.(type) {
		case float64:
			year := int(v)
			return time.Date(year, 1, 1, 12, 0, 0, 0, time.Local), nil
		case int:
			return time.Date(v, 1, 1, 12, 0, 0, 0, time.Local), nil
		case int64:
			return time.Date(int(v), 1, 1, 12, 0, 0, 0, time.Local), nil
		case string:
			year, err := strconv.Atoi(v)
			if err != nil {
				return time.Time{}, err
			}
			return time.Date(year, 1, 1, 12, 0, 0, 0, time.Local), nil
		default:
			return time.Time{}, fmt.Errorf("unexpected year date %T", value)
		}
	default:
		return time.Time{}, fmt.Errorf("unknown unit %s", unit)
	}
}

func (c *Client) ImportEnergyHistory(ctx context.Context, plant Plant) error {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, -11, 0)
	yearStart := time.Date(now.Year()-4, 1, 1, 0, 0, 0, 0, time.Local)

	day, err := c.dailyHistoryChunks(ctx, plant.ID, today, growattHistoryWeeks)
	if err != nil {
		return err
	}
	week := aggregateWeekly(day)
	month, err := c.EnergyHistory(ctx, plant.ID, monthStart, today, "month")
	if err != nil {
		return err
	}
	year, err := c.EnergyHistory(ctx, plant.ID, yearStart, today, "year")
	if err != nil {
		return err
	}

	points := make([]EnergyPoint, 0, len(day)+len(week)+len(month)+len(year))
	points = append(points, day...)
	points = append(points, week...)
	points = append(points, month...)
	points = append(points, year...)
	return importEnergyPoints(ctx, plant, points)
}

func importEnergyPoints(ctx context.Context, plant Plant, points []EnergyPoint) error {
	if len(points) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, point := range points {
		labels := fmt.Sprintf(
			"plant_id=\"%s\",plant_name=\"%s\",period=\"%s\"",
			escapeLabelValue(strconv.FormatInt(plant.ID, 10)),
			escapeLabelValue(plant.Name),
			escapeLabelValue(point.Period),
		)
		tsMillis := point.Timestamp.Unix() * 1000
		buf.WriteString("gohome_growatt_energy_kwh{")
		buf.WriteString(labels)
		buf.WriteString("} ")
		buf.WriteString(strconv.FormatFloat(point.EnergyKWh, 'f', -1, 64))
		buf.WriteString(" ")
		buf.WriteString(strconv.FormatInt(tsMillis, 10))
		buf.WriteString("\n")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, victoriaImportURL, strings.NewReader(buf.String()))
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
		return fmt.Errorf("victoria import http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func escapeLabelValue(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return value
}

func (c *Client) dailyHistoryChunks(ctx context.Context, plantID int64, end time.Time, weeks int) ([]EnergyPoint, error) {
	if weeks <= 0 {
		return nil, nil
	}

	points := make([]EnergyPoint, 0, weeks*7)
	seen := make(map[int64]struct{}, weeks*7)

	for i := 0; i < weeks; i++ {
		chunkEnd := end.AddDate(0, 0, -(i * 7))
		chunkStart := chunkEnd.AddDate(0, 0, -6)

		chunk, err := c.EnergyHistory(ctx, plantID, chunkStart, chunkEnd, "day")
		if err != nil {
			return nil, err
		}
		for _, point := range chunk {
			key := point.Timestamp.Unix()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			points = append(points, point)
		}
		if i < weeks-1 {
			if err := sleepWithContext(ctx, 25*time.Second); err != nil {
				return points, err
			}
		}
	}

	return points, nil
}

func aggregateWeekly(points []EnergyPoint) []EnergyPoint {
	if len(points) == 0 {
		return nil
	}

	type weekKey struct {
		year int
		week int
	}

	weekTotals := make(map[weekKey]float64)
	for _, point := range points {
		year, week := point.Timestamp.ISOWeek()
		weekTotals[weekKey{year: year, week: week}] += point.EnergyKWh
	}

	weekly := make([]EnergyPoint, 0, len(weekTotals))
	for key, energy := range weekTotals {
		weekly = append(weekly, EnergyPoint{
			Timestamp: isoWeekStart(key.year, key.week),
			EnergyKWh: energy,
			Period:    "week",
		})
	}
	return weekly
}

func isoWeekStart(year, week int) time.Time {
	anchor := time.Date(year, time.January, 4, 12, 0, 0, 0, time.Local)
	weekday := int(anchor.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := anchor.AddDate(0, 0, -(weekday - 1))
	return monday.AddDate(0, 0, (week-1)*7)
}
