package hikvision

import (
	"context"
	"os"
	"testing"
	"time"

	hikvisionv1 "github.com/joshp123/gohome/proto/gen/plugins/hikvision/v1"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func readStatusMetrics(client *Client) map[string]*dto.Metric {
	ch := make(chan prometheus.Metric, 100)
	collectStatusMetrics(context.Background(), client, ch)
	close(ch)
	result := map[string]*dto.Metric{}
	for metric := range ch {
		value := &dto.Metric{}
		_ = metric.Write(value)
		result[metric.Desc().String()+value.String()] = value
	}
	return result
}

func TestDeviceStatus(t *testing.T) {
	camera := newFakeCamera(t)
	client := camera.client(t, testPassword)
	response, err := (&service{client: client}).GetDeviceStatus(context.Background(), &hikvisionv1.GetDeviceStatusRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if response.Device.GetUptimeSeconds() != 525836 || response.Device.Cpus[0].GetUsagePercent() != 67 || response.Device.Memory[0].GetUsageRaw() != 95 || response.Device.Memory[0].GetAvailableRaw() != 11324 {
		t.Fatalf("device: %v", response.Device)
	}
	if len(response.Storage) != 1 || response.Storage[0].GetCapacityMegabytes() != 119008 || response.Storage[0].GetFreeMegabytes() != 27648 || response.Storage[0].Status != "ok" {
		t.Fatalf("storage: %v", response.Storage)
	}
	if response.GetStreamingSessions() != 1 || len(response.RecordingTracks) != 2 || !response.RecordingTracks[0].GetLoopEnabled() || response.RecordingTracks[1].GetScheduleEnabled() {
		t.Fatalf("recording/streams: %v", response)
	}
	if len(readStatusMetrics(client)) != 21 {
		t.Fatal("expected 21 status readings including four resource health results")
	}
	camera.unavailable.Store(true)
	failed := readStatusMetrics(client)
	if len(failed) != 4 {
		t.Fatal("failed resources exported readings")
	}
	for _, metric := range failed {
		if metric.GetGauge().GetValue() != 0 {
			t.Fatal("failed resource reports success")
		}
	}
	camera.unavailable.Store(false)
	if len(readStatusMetrics(client)) != 21 {
		t.Fatal("resources did not recover")
	}
}

func TestStatusMissingFields(t *testing.T) {
	var status deviceStatusXML
	if err := decodeXML("status", []byte(`<DeviceStatus><CPUList><CPU/></CPUList><MemoryList><Memory/></MemoryList></DeviceStatus>`), &status); err != nil {
		t.Fatal(err)
	}
	if status.Uptime != nil || status.Reboots != nil || status.CPUs[0].Usage != nil || status.Memory[0].Usage != nil {
		t.Fatal("absent reading became zero")
	}
	if err := decodeXML("status", []byte(`<ResponseStatus><statusString>OK</statusString></ResponseStatus>`), &status); err == nil {
		t.Fatal("wrong response accepted")
	}
}

func TestEventMetadata(t *testing.T) {
	event, err := parseEventNotification([]byte(`<EventNotificationAlert><eventType>VMD</eventType><eventState>active</eventState><activePostCount>3</activePostCount><eventDescription>Motion alarm</eventDescription><channelName>Outside</channelName></EventNotificationAlert>`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	response := cameraEventProto(event)
	if response.GetActivePostCount() != 3 || response.EventDescription != "Motion alarm" || response.ChannelName != "Outside" {
		t.Fatalf("metadata lost: %v", response)
	}
}

func TestLiveDeviceStatus(t *testing.T) {
	if os.Getenv("HIKVISION_BASE_URL") == "" {
		t.Skip("set HIKVISION_BASE_URL and HIKVISION_CREDENTIALS_FILE")
	}
	credentials, err := os.ReadFile(os.Getenv("HIKVISION_CREDENTIALS_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	username, password, err := parseCredentials(string(credentials))
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(Config{BaseURL: os.Getenv("HIKVISION_BASE_URL"), Username: username, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	for desc, metric := range readStatusMetrics(client) {
		t.Log(desc)
		for _, label := range metric.Label {
			if label.GetName() == "resource" && metric.GetGauge().GetValue() != 1 {
				t.Errorf("resource %s failed", label.GetValue())
			}
		}
	}
}
