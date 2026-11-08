package roborock

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

func decodeDeviceObject(value any, target any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

// CleaningHistory returns every record retained in the device's summary, oldest first.
func (c *Client) CleaningHistory(ctx context.Context, id string) ([]CleaningRecord, error) {
	if err := c.ensureHomeData(ctx); err != nil {
		return nil, err
	}
	device, err := c.deviceByID(id)
	if err != nil {
		return nil, err
	}
	raw, err := c.readRPC(ctx, device, "get_clean_summary", nil)
	if err != nil {
		return nil, err
	}
	summary, ok := normalizeMap(raw)
	if !ok {
		return nil, fmt.Errorf("invalid cleaning summary")
	}
	ids, ok := summary["records"].([]any)
	if !ok {
		return nil, fmt.Errorf("missing cleaning record IDs")
	}
	records := []CleaningRecord{}
	for _, id := range ids {
		raw, err := c.readRPC(ctx, device, "get_clean_record", []any{id})
		if err != nil {
			return nil, err
		}
		parts, ok := raw.([]any)
		if !ok {
			parts = []any{raw}
		}
		for _, part := range parts {
			var record CleaningRecord
			if err := decodeDeviceObject(part, &record); err != nil {
				return nil, err
			}
			if record.Begin == nil || record.End == nil || *record.End < *record.Begin {
				return nil, fmt.Errorf("invalid cleaning record timestamps")
			}
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool { return *records[i].Begin < *records[j].Begin })
	return records, nil
}
