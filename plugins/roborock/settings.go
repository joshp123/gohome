package roborock

import (
	"context"
	"fmt"
)

// Only operational state is polled; network identities and firmware tuning are omitted.
func (c *Client) operationalSettings(ctx context.Context, device HomeDataDevice) (OperationalSettings, error) {
	s := OperationalSettings{}
	requests := []struct {
		method string
		fields map[string]**int64
	}{
		{"get_network_info", map[string]**int64{"rssi": &s.WifiSignalDbm}},
		{"get_dust_collection_mode", map[string]**int64{"mode": &s.DustCollectionModeCode}},
		{"get_wash_towel_mode", map[string]**int64{"wash_mode": &s.MopWashModeCode}},
		{"get_smart_wash_params", map[string]**int64{"smart_wash": &s.SmartWashEnabled, "wash_interval": &s.MopWashIntervalSeconds}},
		{"app_get_dryer_setting", map[string]**int64{"status": &s.DryerEnabled, "on.dry_time": &s.DryerDurationSeconds}},
		{"get_carpet_mode", map[string]**int64{"enable": &s.CarpetBoostEnabled}},
		{"get_carpet_clean_mode", map[string]**int64{"carpet_clean_mode": &s.CarpetModeCode}},
	}
	for _, request := range requests {
		raw, err := c.readRPC(ctx, device, request.method, nil)
		if err != nil {
			return s, fmt.Errorf("%s: %w", request.method, err)
		}
		if value, ok := raw.(string); ok && value == "unknown_method" {
			continue
		}
		values, ok := normalizeMap(raw)
		if !ok {
			return s, fmt.Errorf("invalid %s response", request.method)
		}
		for key, target := range request.fields {
			var value any
			if key == "on.dry_time" {
				if nested, ok := values["on"].(map[string]any); ok {
					value = nested["dry_time"]
				}
			} else {
				value = values[key]
			}
			if value == nil {
				continue
			}
			number, ok := value.(float64)
			if !ok || number != float64(int64(number)) {
				return s, fmt.Errorf("invalid %s %s", request.method, key)
			}
			n := int64(number)
			*target = &n
		}
	}
	return s, nil
}
