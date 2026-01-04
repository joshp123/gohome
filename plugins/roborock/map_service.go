package roborock

import (
	"context"
	"fmt"
	"time"
)

const mapRefreshInterval = 5 * time.Second

type mapSnapshot struct {
	image     mapImage
	fetchedAt time.Time
}

func (c *Client) MapSnapshot(ctx context.Context, deviceID string) (mapImage, error) {
	if err := c.ensureHomeData(ctx); err != nil {
		return mapImage{}, err
	}
	if deviceID == "" {
		devs, err := c.Devices(ctx)
		if err != nil {
			return mapImage{}, err
		}
		if len(devs) == 0 {
			return mapImage{}, fmt.Errorf("no devices available")
		}
		deviceID = devs[0].ID
	}
	if img, ok := c.cachedMap(deviceID); ok {
		return img, nil
	}
	device, err := c.deviceByID(deviceID)
	if err != nil {
		return mapImage{}, err
	}
	data, err := c.fetchMapViaMQTT(ctx, device)
	if err != nil {
		return mapImage{}, err
	}
	parsed, err := parseMapImage(data)
	if err != nil {
		return mapImage{}, err
	}
	c.storeMap(deviceID, parsed)
	return parsed, nil
}

func (c *Client) cachedMap(deviceID string) (mapImage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.mapCache[deviceID]; ok {
		if time.Since(entry.fetchedAt) < mapRefreshInterval {
			return entry.image, true
		}
	}
	return mapImage{}, false
}

func (c *Client) storeMap(deviceID string, img mapImage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mapCache[deviceID] = mapSnapshot{image: img, fetchedAt: time.Now()}
}
