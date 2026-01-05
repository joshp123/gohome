package roborock

import (
	"context"
	"fmt"
	"log"
	"time"
)

const mapRefreshInterval = 5 * time.Second

type mapSnapshot struct {
	data      []byte
	image     mapImage
	segments  []segmentSummary
	trace     []mapPoint
	fetchedAt time.Time
}

func (c *Client) MapSnapshot(ctx context.Context, deviceID string) (mapImage, error) {
	img, _, err := c.mapSnapshot(ctx, deviceID, "", false)
	return img, err
}

func (c *Client) MapSnapshotWithLabels(ctx context.Context, deviceID string, labelMode string) (mapImage, error) {
	img, _, err := c.mapSnapshot(ctx, deviceID, labelMode, false)
	return img, err
}

func (c *Client) MapSnapshotWithOptions(ctx context.Context, deviceID string, labelMode string, withTrace bool) (mapImage, error) {
	img, _, err := c.mapSnapshot(ctx, deviceID, labelMode, withTrace)
	return img, err
}

func (c *Client) SegmentsSnapshot(ctx context.Context, deviceID string) ([]segmentSummary, error) {
	_, segments, err := c.mapSnapshot(ctx, deviceID, "", false)
	return segments, err
}

func (c *Client) mapSnapshot(ctx context.Context, deviceID string, labelMode string, withTrace bool) (mapImage, []segmentSummary, error) {
	if err := c.ensureHomeData(ctx); err != nil {
		return mapImage{}, nil, err
	}
	if deviceID == "" {
		devs, err := c.Devices(ctx)
		if err != nil {
			return mapImage{}, nil, err
		}
		if len(devs) == 0 {
			return mapImage{}, nil, fmt.Errorf("no devices available")
		}
		deviceID = devs[0].ID
	}
	if snap, ok := c.cachedMap(deviceID); ok && labelMode == "" && !withTrace {
		return snap.image, snap.segments, nil
	}
	device, err := c.deviceByID(deviceID)
	if err != nil {
		return mapImage{}, nil, err
	}
	var data []byte
	var trace []mapPoint
	if snap, ok := c.cachedMap(deviceID); ok {
		data = snap.data
		trace = snap.trace
	}
	if len(data) == 0 {
		data, err = c.fetchMapViaMQTT(ctx, device)
		if err != nil {
			return mapImage{}, nil, err
		}
	}
	if withTrace && len(trace) == 0 {
		trace, err = extractTrace(data)
		if err != nil {
			log.Printf("roborock trace parse failed: %v", err)
			trace = nil
		}
	}
	parsed, segments, err := parseMapData(data, device.Name, labelMode, c.cfg.SegmentNames, trace)
	if err != nil {
		return mapImage{}, nil, err
	}
	if len(segments) == 0 {
		log.Printf("roborock map snapshot has 0 segments (device=%s data_bytes=%d)", device.Name, len(data))
	}
	c.storeMap(deviceID, data, parsed, segments, trace)
	return parsed, segments, nil
}

func (c *Client) cachedMap(deviceID string) (mapSnapshot, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.mapCache[deviceID]; ok {
		if time.Since(entry.fetchedAt) < mapRefreshInterval {
			return entry, true
		}
	}
	return mapSnapshot{}, false
}

func (c *Client) storeMap(deviceID string, data []byte, img mapImage, segments []segmentSummary, trace []mapPoint) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mapCache[deviceID] = mapSnapshot{
		data:      data,
		image:     img,
		segments:  segments,
		trace:     trace,
		fetchedAt: time.Now(),
	}
}

func applySegmentLabels(segments []segmentSummary, labels map[uint32]string) {
	if len(labels) == 0 {
		return
	}
	for i := range segments {
		if label, ok := labels[uint32(segments[i].id)]; ok {
			segments[i].label = label
		}
	}
}
