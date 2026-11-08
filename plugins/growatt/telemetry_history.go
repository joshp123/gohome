package growatt

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"
)

// WriteInverterHistory converts a captured queryHistoricalData response to
// Prometheus import format. It makes no cloud requests or database writes.
func WriteInverterHistory(w io.Writer, payload []byte, plant Plant, serial, timezone string) (int, error) {
	var response struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Rows          []map[string]any `json:"datas"`
			HaveNext      bool             `json:"haveNext"`
			HaveNextSnake bool             `json:"have_next"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return 0, err
	}
	if response.Code != 0 {
		return 0, APIError{Code: response.Code, Msg: response.Message}
	}
	if response.Data.HaveNext || response.Data.HaveNextSnake {
		return 0, fmt.Errorf("incomplete inverter history response")
	}
	count := 0
	for _, row := range response.Data.Rows {
		telemetry, err := decodeInverterTelemetry(row, serial, timezone)
		if err != nil {
			return count, err
		}
		at, err := time.Parse(time.RFC3339, telemetry.ObservedAt)
		if err != nil {
			return count, err
		}
		for _, reading := range telemetry.Measurements {
			_, err = fmt.Fprintf(w, "%s{plant_id=%q,plant_name=%q,serial=%q,channel=%q} %s %d\n", inverterMetrics[reading.Quantity].name, strconv.FormatInt(plant.ID, 10), plant.Name, serial, reading.Channel, strconv.FormatFloat(reading.Value, 'g', -1, 64), at.UnixMilli())
			if err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}
