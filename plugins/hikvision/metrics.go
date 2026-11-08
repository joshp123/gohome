package hikvision

import (
	"context"
	"log"
	"strings"
	"time"

	hikvisionv1 "github.com/joshp123/gohome/proto/gen/plugins/hikvision/v1"
	"github.com/prometheus/client_golang/prometheus"
)

// Label values come only from the fixed EventKind and DetectionTarget
// vocabularies, never from camera text.
var eventLabels = []string{"kind", "target"}

// MetricsCollector reads the camera on each scrape and counts alert-stream events as they arrive.
type MetricsCollector struct {
	client *Client
	stream *alertStream

	scrapeSuccess     *prometheus.Desc
	lastSuccess       prometheus.Gauge
	info              *prometheus.Desc
	detectionEnabled  *prometheus.Desc
	streamConnected   *prometheus.Desc
	streamLastMessage *prometheus.Desc
	events            *prometheus.CounterVec
	eventsActive      *prometheus.GaugeVec
	lastEvent         *prometheus.GaugeVec
}

func NewMetricsCollector(client *Client) *MetricsCollector {
	return &MetricsCollector{
		client: client,
		scrapeSuccess: prometheus.NewDesc("gohome_hikvision_scrape_success",
			"Whether the last read of the camera's identity and detection settings succeeded (1=ok, 0=error)", nil, nil),
		lastSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_hikvision_last_success_timestamp_seconds",
			Help: "Last successful read of the camera (epoch seconds)",
		}),
		info: prometheus.NewDesc("gohome_hikvision_info",
			"Camera identity; always 1. Absent when the last read failed.",
			[]string{"model", "firmware_version", "firmware_build"}, nil),
		detectionEnabled: prometheus.NewDesc("gohome_hikvision_detection_enabled",
			"Whether the camera's own detection is switched on (1) or off (0). Absent when the last read failed.",
			[]string{"detection"}, nil),
		streamConnected: prometheus.NewDesc("gohome_hikvision_alert_stream_connected",
			"Whether gohome holds an open connection to the camera's event stream (1) or not (0)", nil, nil),
		streamLastMessage: prometheus.NewDesc("gohome_hikvision_alert_stream_last_message_timestamp_seconds",
			"When the event stream last carried anything, including the ten-second keep-alive (epoch seconds)", nil, nil),
		events: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gohome_hikvision_events_total",
			Help: "Events that started, however many notifications the camera repeated while each lasted. An event ends after 10 s without a repeat.",
		}, eventLabels),
		eventsActive: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_hikvision_events_active",
			Help: "Events of this kind and target happening now. Absent while the event stream is disconnected.",
		}, eventLabels),
		lastEvent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_hikvision_last_event_timestamp_seconds",
			Help: "When the latest event of this kind and target started (epoch seconds)",
		}, eventLabels),
	}
}

func (c *MetricsCollector) eventStarted(event cameraEvent) {
	labels := eventLabelValues(event)
	c.events.With(labels).Inc()
	c.eventsActive.With(labels).Inc()
	c.lastEvent.With(labels).Set(float64(event.ReceivedAt.Unix()))
}

func (c *MetricsCollector) eventStopped(event cameraEvent) {
	c.eventsActive.With(eventLabelValues(event)).Dec()
}

func eventLabelValues(event cameraEvent) prometheus.Labels {
	return prometheus.Labels{"kind": kindLabel(event.Kind), "target": targetLabel(event.Target)}
}

func kindLabel(kind hikvisionv1.EventKind) string {
	return strings.ToLower(strings.TrimPrefix(kind.String(), "EVENT_KIND_"))
}

func targetLabel(target hikvisionv1.DetectionTarget) string {
	return strings.ToLower(strings.TrimPrefix(target.String(), "DETECTION_TARGET_"))
}

func (c *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.scrapeSuccess
	c.lastSuccess.Describe(ch)
	ch <- c.info
	ch <- c.detectionEnabled
	ch <- c.streamConnected
	ch <- c.streamLastMessage
	c.events.Describe(ch)
	c.eventsActive.Describe(ch)
	c.lastEvent.Describe(ch)
}

// settingsRead is what each scrape reads from the camera.
type settingsRead struct {
	device          deviceInfoXML
	motion          motionDetectionXML
	line, intrusion smartDetectionXML
}

func (c *MetricsCollector) readSettings(ctx context.Context) (settingsRead, error) {
	var read settingsRead
	var err error
	if read.device, err = c.client.DeviceInfo(ctx); err != nil {
		return read, err
	}
	if read.motion, err = c.client.MotionDetection(ctx); err != nil {
		return read, err
	}
	if read.line, err = c.client.LineDetection(ctx); err != nil {
		return read, err
	}
	read.intrusion, err = c.client.FieldDetection(ctx)
	return read, err
}

func (c *MetricsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	read, err := c.readSettings(ctx)
	if err != nil {
		log.Printf("hikvision source read failed: %v", err)
		ch <- prometheus.MustNewConstMetric(c.scrapeSuccess, prometheus.GaugeValue, 0)
	} else {
		c.lastSuccess.Set(float64(time.Now().Unix()))
		ch <- prometheus.MustNewConstMetric(c.scrapeSuccess, prometheus.GaugeValue, 1)
		ch <- prometheus.MustNewConstMetric(c.info, prometheus.GaugeValue, 1,
			read.device.Model, read.device.FirmwareVersion, read.device.FirmwareBuild)
		for detection, enabled := range map[string]bool{
			"motion":        read.motion.Enabled,
			"line_crossing": read.line.Enabled,
			"intrusion":     read.intrusion.Enabled,
		} {
			ch <- prometheus.MustNewConstMetric(c.detectionEnabled, prometheus.GaugeValue, boolValue(enabled), detection)
		}
	}
	c.lastSuccess.Collect(ch)

	status := c.stream.Status()
	ch <- prometheus.MustNewConstMetric(c.streamConnected, prometheus.GaugeValue, boolValue(status.Connected))
	if !status.LastMessage.IsZero() {
		ch <- prometheus.MustNewConstMetric(c.streamLastMessage, prometheus.GaugeValue, float64(status.LastMessage.Unix()))
	}
	if status.Connected {
		c.eventsActive.Collect(ch)
	}
	c.events.Collect(ch)
	c.lastEvent.Collect(ch)
}

func boolValue(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
