package shelly

import (
	"fmt"
	"strings"

	shellyv1 "github.com/joshp123/gohome/proto/gen/plugins/shelly/v1"
)

// CoverConfig names one shutter and the relay that drives it.
type CoverConfig struct {
	DeviceID string
	Address  string
	Name     string
}

func coversFromProto(cfg *shellyv1.ShellyConfig) ([]CoverConfig, error) {
	if len(cfg.GetCovers()) == 0 {
		return nil, fmt.Errorf("shelly: no covers configured")
	}
	seenDeviceIDs := map[string]bool{}
	seenNames := map[string]bool{}
	covers := make([]CoverConfig, 0, len(cfg.GetCovers()))
	for _, c := range cfg.GetCovers() {
		cover := CoverConfig{
			DeviceID: strings.TrimSpace(c.GetDeviceId()),
			Address:  strings.TrimSpace(c.GetAddress()),
			Name:     strings.TrimSpace(c.GetName()),
		}
		if cover.DeviceID == "" || cover.Address == "" || cover.Name == "" {
			return nil, fmt.Errorf("shelly: every cover needs device_id, address and name (got %+v)", cover)
		}
		if seenDeviceIDs[cover.DeviceID] {
			return nil, fmt.Errorf("shelly: device_id %q configured twice", cover.DeviceID)
		}
		if seenNames[cover.Name] {
			return nil, fmt.Errorf("shelly: name %q configured twice", cover.Name)
		}
		seenDeviceIDs[cover.DeviceID], seenNames[cover.Name] = true, true
		covers = append(covers, cover)
	}
	return covers, nil
}
