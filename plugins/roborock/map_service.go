package roborock

import (
	"context"
	"fmt"
	"log"
	"time"
)

const mapRefreshInterval = 5 * time.Second

type mapSnapshot struct {
	vacuumMap vacuumMap
	fetchedAt time.Time
}

// MapPNG renders the device's current map. labelMode is "names", "segments" or "".
func (c *Client) MapPNG(ctx context.Context, deviceID string, labelMode string, showLastRunPath bool) ([]byte, error) {
	vacuumMapData, err := c.currentMap(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	return renderVacuumMap(vacuumMapData, labelMode, showLastRunPath)
}

func (c *Client) SegmentsSnapshot(ctx context.Context, deviceID string) ([]segmentSummary, error) {
	vacuumMapData, err := c.currentMap(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	return vacuumMapData.rooms, nil
}

func (c *Client) currentMap(ctx context.Context, deviceID string) (vacuumMap, error) {
	if err := c.ensureHomeData(ctx); err != nil {
		return vacuumMap{}, err
	}
	if deviceID == "" {
		devices, err := c.Devices(ctx)
		if err != nil {
			return vacuumMap{}, err
		}
		if len(devices) == 0 {
			return vacuumMap{}, fmt.Errorf("no devices available")
		}
		deviceID = devices[0].ID
	}
	if snapshot, ok := c.cachedMap(deviceID); ok {
		return snapshot.vacuumMap, nil
	}
	device, err := c.deviceByID(deviceID)
	if err != nil {
		return vacuumMap{}, err
	}
	raw, err := c.fetchMapViaMQTT(ctx, device)
	if err != nil {
		return vacuumMap{}, err
	}
	vacuumMapData, err := parseVacuumMap(raw, c.cfg.SegmentNames)
	if err != nil {
		return vacuumMap{}, err
	}
	if len(vacuumMapData.rooms) == 0 {
		log.Printf("roborock map has no rooms (device=%s data_bytes=%d)", device.Name, len(raw))
	}
	c.mu.Lock()
	c.mapCache[deviceID] = mapSnapshot{vacuumMap: vacuumMapData, fetchedAt: time.Now()}
	c.mu.Unlock()
	return vacuumMapData, nil
}

func (c *Client) cachedMap(deviceID string) (mapSnapshot, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	snapshot, ok := c.mapCache[deviceID]
	if !ok || time.Since(snapshot.fetchedAt) >= mapRefreshInterval {
		return mapSnapshot{}, false
	}
	return snapshot, true
}
