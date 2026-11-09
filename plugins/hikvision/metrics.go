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

// MetricsCollector reads the camera on each scrape, counts raw alert-stream
// events as they arrive, and counts the alarms the camera logs.
type MetricsCollector struct {
	client *Client
	stream *alertStream
	alarms *alarmLog

	scrapeSuccess     *prometheus.Desc
	lastSuccess       prometheus.Gauge
	info              *prometheus.Desc
	detectionEnabled  *prometheus.Desc
	streamConnected   *prometheus.Desc
	streamLastMessage *prometheus.Desc
	events            *prometheus.CounterVec
	eventsActive      *prometheus.GaugeVec
	lastEvent         *prometheus.GaugeVec
	alarmsTotal       *prometheus.CounterVec
	lastAlarmStart    *prometheus.Desc
	alarmLogReadable  *prometheus.Desc
	alarmLogLastRead  *prometheus.Desc
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
			Help: "Raw alert-stream events that started, before the camera's people-and-vehicle filter, so mostly wind and light. One per event however many notifications it repeated; an event ends after 10 s without a repeat. The filtered alarms are gohome_hikvision_alarms_total.",
		}, eventLabels),
		eventsActive: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_hikvision_events_active",
			Help: "Raw alert-stream events of this kind and target happening now. Absent while the event stream is disconnected.",
		}, eventLabels),
		lastEvent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_hikvision_last_event_timestamp_seconds",
			Help: "When the latest raw alert-stream event of this kind and target started (epoch seconds)",
		}, eventLabels),
		alarmsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gohome_hikvision_alarms_total",
			Help: "Alarms the camera raised and logged since gohome started: the ones its detection filter accepted (a person or vehicle, for motion filtered to them).",
		}, []string{"kind"}),
		lastAlarmStart: prometheus.NewDesc("gohome_hikvision_last_alarm_start_timestamp_seconds",
			"When the latest logged alarm of this kind started, camera clock (epoch seconds). Absent while the alarm log is unreadable.",
			[]string{"kind"}, nil),
		alarmLogReadable: prometheus.NewDesc("gohome_hikvision_alarm_log_readable",
			"Whether gohome read the camera's alarm log in the last minute (1) or not (0)", nil, nil),
		alarmLogLastRead: prometheus.NewDesc("gohome_hikvision_alarm_log_last_read_timestamp_seconds",
			"When gohome last read the camera's alarm log (epoch seconds)", nil, nil),
	}
}

func (c *MetricsCollector) eventStarted(event cameraEvent) {
	labels := eventLabelValues(event)
	c.events.With(labels).Inc()
	c.eventsActive.With(labels).Inc()
	c.lastEvent.With(labels).Set(float64(event.ReceivedAt.Unix()))
}

func (c *MetricsCollector) alarmStarted(a alarm) {
	c.alarmsTotal.With(prometheus.Labels{"kind": kindLabel(a.Kind)}).Inc()
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
	for _, desc := range statusMetrics {
		ch <- desc
	}
	ch <- c.scrapeSuccess
	c.lastSuccess.Describe(ch)
	ch <- c.info
	ch <- c.detectionEnabled
	ch <- c.streamConnected
	ch <- c.streamLastMessage
	c.events.Describe(ch)
	c.eventsActive.Describe(ch)
	c.lastEvent.Describe(ch)
	c.alarmsTotal.Describe(ch)
	ch <- c.lastAlarmStart
	ch <- c.alarmLogReadable
	ch <- c.alarmLogLastRead
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
	collectStatusMetrics(ctx, c.client, ch)

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
	c.collectAlarms(ch)
}

func (c *MetricsCollector) collectAlarms(ch chan<- prometheus.Metric) {
	c.alarmsTotal.Collect(ch)
	if c.alarms == nil {
		return
	}
	status := c.alarms.Status()
	ch <- prometheus.MustNewConstMetric(c.alarmLogReadable, prometheus.GaugeValue, boolValue(status.Readable))
	if !status.LastRead.IsZero() {
		ch <- prometheus.MustNewConstMetric(c.alarmLogLastRead, prometheus.GaugeValue, float64(status.LastRead.Unix()))
	}
	if !status.Readable {
		return
	}
	for kind, start := range status.LatestStart {
		ch <- prometheus.MustNewConstMetric(c.lastAlarmStart, prometheus.GaugeValue, float64(start.Unix()), kindLabel(kind))
	}
}

func boolValue(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
