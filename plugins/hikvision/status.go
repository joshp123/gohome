package hikvision

import (
	"context"
	"encoding/xml"
)

const (
	pathDeviceStatus    = "/ISAPI/System/status"
	pathStorage         = "/ISAPI/ContentMgmt/Storage"
	pathStreamingStatus = "/ISAPI/Streaming/status"
	pathRecordingTracks = "/ISAPI/ContentMgmt/record/tracks"
)

type deviceStatusXML struct {
	XMLName     xml.Name `xml:"DeviceStatus"`
	CurrentTime string   `xml:"currentDeviceTime"`
	Uptime      *float64 `xml:"deviceUpTime"`
	Reboots     *float64 `xml:"totalRebootCount"`
	CPUs        []struct {
		Usage *float64 `xml:"cpuUtilization"`
	} `xml:"CPUList>CPU"`
	Memory []struct {
		Usage     *float64 `xml:"memoryUsage"`
		Available *float64 `xml:"memoryAvailable"`
	} `xml:"MemoryList>Memory"`
}

type storageDeviceXML struct {
	ID       string   `xml:"id"`
	Status   string   `xml:"status"`
	Capacity *float64 `xml:"capacity"`
	Free     *float64 `xml:"freeSpace"`
	Property string   `xml:"property"`
}

type storageXML struct {
	XMLName xml.Name           `xml:"storage"`
	Disks   []storageDeviceXML `xml:"hddList>hdd"`
	NAS     []storageDeviceXML `xml:"nasList>nas"`
}

type streamingStatusXML struct {
	XMLName  xml.Name `xml:"StreamingStatus"`
	Sessions *float64 `xml:"totalStreamingSessions"`
}

type recordingTrackXML struct {
	ID              string `xml:"id"`
	Enabled         *bool  `xml:"Enable"`
	Loop            *bool  `xml:"LoopEnable"`
	ScheduleEnabled *bool  `xml:"CustomExtensionList>CustomExtension>enableSchedule"`
}

type recordingTracksXML struct {
	XMLName xml.Name            `xml:"TrackList"`
	Tracks  []recordingTrackXML `xml:"Track"`
}

func (c *Client) DeviceStatus(ctx context.Context) (deviceStatusXML, error) {
	var value deviceStatusXML
	err := c.getXML(ctx, pathDeviceStatus, &value)
	return value, err
}
func (c *Client) Storage(ctx context.Context) (storageXML, error) {
	var value storageXML
	err := c.getXML(ctx, pathStorage, &value)
	return value, err
}
func (c *Client) StreamingStatus(ctx context.Context) (streamingStatusXML, error) {
	var value streamingStatusXML
	err := c.getXML(ctx, pathStreamingStatus, &value)
	return value, err
}
func (c *Client) RecordingTracks(ctx context.Context) (recordingTracksXML, error) {
	var value recordingTracksXML
	err := c.getXML(ctx, pathRecordingTracks, &value)
	return value, err
}
