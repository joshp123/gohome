package hikvision

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var statusMetrics = map[string]*prometheus.Desc{
	"read_success":               statusDesc("read_success", "Whether this resource was read successfully; failed resources export no readings", "resource"),
	"uptime_seconds":             statusDesc("uptime_seconds", "Camera uptime in seconds"),
	"device_time_seconds":        statusDesc("device_time_seconds", "Camera's own clock, Unix seconds"),
	"reboot_count":               statusDesc("reboot_count", "Reboot count reported by the camera"),
	"cpu_usage_percent":          statusDesc("cpu_usage_percent", "CPU utilisation in percent", "cpu"),
	"memory_usage_raw":           statusDesc("memory_usage_raw", "ISAPI memoryUsage, unconverted; firmware-dependent units (see plugin documentation)", "memory"),
	"memory_available_raw":       statusDesc("memory_available_raw", "ISAPI memoryAvailable, unconverted; firmware-dependent units (see plugin documentation)", "memory"),
	"storage_capacity_megabytes": statusDesc("storage_capacity_megabytes", "Storage capacity in ISAPI MB", "storage", "id"),
	"storage_free_megabytes":     statusDesc("storage_free_megabytes", "Free storage in ISAPI MB", "storage", "id"),
	"storage_status":             statusDesc("storage_status", "Current storage status, always 1 for the reported state", "storage", "id", "status"),
	"storage_property":           statusDesc("storage_property", "Storage access property (RW, RO or Redund), always 1", "storage", "id", "property"),
	"streaming_sessions":         statusDesc("streaming_sessions", "Current video streaming sessions"),
	"recording_enabled":          statusDesc("recording_enabled", "Whether a recording track is enabled; not proof it is recording now", "track"),
	"recording_loop_enabled":     statusDesc("recording_loop_enabled", "Whether a recording track can overwrite old recordings", "track"),
	"recording_schedule_enabled": statusDesc("recording_schedule_enabled", "Whether a recording track's schedule is enabled", "track"),
}

func statusDesc(name, help string, labels ...string) *prometheus.Desc {
	return prometheus.NewDesc("gohome_hikvision_"+name, help, labels, nil)
}

func collectStatusMetrics(ctx context.Context, client *Client, ch chan<- prometheus.Metric) {
	emit := func(name string, value float64, labels ...string) {
		ch <- prometheus.MustNewConstMetric(statusMetrics[name], prometheus.GaugeValue, value, labels...)
	}
	optional := func(name string, value *float64, labels ...string) {
		if value != nil {
			emit(name, *value, labels...)
		}
	}
	success := func(resource string, err error) bool {
		emit("read_success", boolValue(err == nil), resource)
		if err != nil {
			log.Printf("hikvision %s read failed: %v", resource, err)
		}
		return err == nil
	}
	status, err := client.DeviceStatus(ctx)
	if success("status", err) {
		optional("uptime_seconds", status.Uptime)
		optional("reboot_count", status.Reboots)
		if clock, err := time.Parse(time.RFC3339, status.CurrentTime); err == nil {
			emit("device_time_seconds", float64(clock.Unix()))
		}
		for i, cpu := range status.CPUs {
			optional("cpu_usage_percent", cpu.Usage, strconv.Itoa(i+1))
		}
		for i, memory := range status.Memory {
			optional("memory_usage_raw", memory.Usage, strconv.Itoa(i+1))
			optional("memory_available_raw", memory.Available, strconv.Itoa(i+1))
		}
	}
	storage, err := client.Storage(ctx)
	if success("storage", err) {
		for kind, devices := range map[string][]storageDeviceXML{"hdd": storage.Disks, "nas": storage.NAS} {
			for _, device := range devices {
				optional("storage_capacity_megabytes", device.Capacity, kind, device.ID)
				optional("storage_free_megabytes", device.Free, kind, device.ID)
				if device.Status != "" {
					emit("storage_status", 1, kind, device.ID, device.Status)
				}
				if device.Property != "" {
					emit("storage_property", 1, kind, device.ID, device.Property)
				}
			}
		}
	}
	streams, err := client.StreamingStatus(ctx)
	if success("streaming", err) {
		optional("streaming_sessions", streams.Sessions)
	}
	tracks, err := client.RecordingTracks(ctx)
	if success("recording", err) {
		for _, track := range tracks.Tracks {
			for name, value := range map[string]*bool{"recording_enabled": track.Enabled, "recording_loop_enabled": track.Loop, "recording_schedule_enabled": track.ScheduleEnabled} {
				if value != nil {
					emit(name, boolValue(*value), track.ID)
				}
			}
		}
	}
}
