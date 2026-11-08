package hikvision

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	hikvisionv1 "github.com/joshp123/gohome/proto/gen/plugins/hikvision/v1"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// testdata/ holds responses in the shape a Hikvision AcuSense camera sends,
// with fictional identity, addresses, times and detection settings.

const testUsername, testPassword = "agent", "correct horse"

var recordedPaths = map[string]string{
	pathDeviceInfo:         "System_deviceInfo.xml",
	pathSystemCapabilities: "System_capabilities.xml",
	pathEventCapabilities:  "Event_capabilities.xml",
	pathMotionDetection:    "System_Video_inputs_channels_1_motionDetection.xml",
	pathLineDetection:      "Smart_LineDetection_1.xml",
	pathLineDetectionCaps:  "Smart_LineDetection_1_capabilities.xml",
	pathFieldDetection:     "Smart_FieldDetection_1.xml",
	pathFieldDetectionCaps: "Smart_FieldDetection_1_capabilities.xml",
}

// fakeCamera answers like the real one: a Digest challenge, then the recorded
// response. Any request that is not a GET fails the test.
type fakeCamera struct {
	t                  *testing.T
	server             *httptest.Server
	unavailable        atomic.Bool
	authorisedRequests atomic.Int32
	stream             []byte
	streamOpened       chan struct{}
	once               sync.Once
}

func newFakeCamera(t *testing.T) *fakeCamera {
	t.Helper()
	camera := &fakeCamera{t: t, streamOpened: make(chan struct{})}
	stream, err := os.ReadFile(filepath.Join("testdata", "alertstream.multipart"))
	if err != nil {
		t.Fatal(err)
	}
	camera.stream = stream
	camera.server = httptest.NewServer(http.HandlerFunc(camera.serve))
	t.Cleanup(camera.server.Close)
	return camera
}

func (c *fakeCamera) client(t *testing.T, password string) *Client {
	t.Helper()
	client, err := NewClient(Config{BaseURL: c.server.URL, Username: testUsername, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func (c *fakeCamera) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.t.Errorf("plugin sent %s %s; it must only read", r.Method, r.URL.Path)
		http.Error(w, "read-only", http.StatusMethodNotAllowed)
		return
	}
	const nonce = "4e6f6e63652d66726f6d2d746865"
	challenge := `Digest qop="auth", realm="IP Camera(K0000)", nonce="` + nonce + `", stale="FALSE"`
	authorization := r.Header.Get("Authorization")
	if authorization == "" {
		w.Header().Set("WWW-Authenticate", challenge)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	c.authorisedRequests.Add(1)
	if !validDigest(authorization, r.URL.Path, nonce) {
		w.Header().Set("WWW-Authenticate", challenge)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if c.unavailable.Load() {
		http.Error(w, "busy", http.StatusServiceUnavailable)
		return
	}
	switch r.URL.Path {
	case pathMainStreamPicture:
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0xFF, 0xD9})
	case pathAlertStream:
		w.Header().Set("Content-Type", "multipart/mixed; boundary=boundary")
		_, _ = w.Write(c.stream)
		w.(http.Flusher).Flush()
		c.once.Do(func() { close(c.streamOpened) })
		<-r.Context().Done()
	default:
		name, ok := recordedPaths[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		body, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			c.t.Error(err)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write(body)
	}
}

// validDigest checks the response the way the camera does (RFC 7616, MD5, qop=auth).
func validDigest(header, uri, nonce string) bool {
	params := map[string]string{}
	for _, part := range splitDigestParams(strings.TrimPrefix(header, "Digest ")) {
		key, value, _ := strings.Cut(part, "=")
		params[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	ha1 := md5Hex(testUsername + ":" + params["realm"] + ":" + testPassword)
	ha2 := md5Hex("GET:" + uri)
	want := md5Hex(ha1 + ":" + nonce + ":" + params["nc"] + ":" + params["cnonce"] + ":auth:" + ha2)
	return params["username"] == testUsername && params["uri"] == uri && params["nonce"] == nonce && params["response"] == want
}

func TestGetCameraFromRecordedResponses(t *testing.T) {
	camera := newFakeCamera(t)
	plugin := newPlugin(camera.client(t, testPassword))
	svc := &service{client: plugin.client, stream: plugin.stream}

	resp, err := svc.GetCamera(context.Background(), &hikvisionv1.GetCameraRequest{})
	if err != nil {
		t.Fatal(err)
	}
	got := resp.GetCamera()
	if got.GetIdentity().GetModel() != "DS-2CD0000G0-I" || got.GetIdentity().GetFirmwareBuild() != "build 200101" {
		t.Fatalf("identity: %v", got.GetIdentity())
	}
	human, vehicle := hikvisionv1.DetectionTarget_DETECTION_TARGET_HUMAN, hikvisionv1.DetectionTarget_DETECTION_TARGET_VEHICLE
	if m := got.GetMotion(); !m.GetEnabled() || m.GetSensitivity() != 40 || len(m.GetTargets()) != 2 || m.GetTargets()[0] != human || m.GetTargets()[1] != vehicle {
		t.Fatalf("motion: %v", m)
	}
	for name, want := range map[string]struct {
		smart   *hikvisionv1.SmartDetection
		enabled bool
	}{"line crossing": {got.GetLineCrossing(), true}, "intrusion": {got.GetIntrusion(), false}} {
		smart := want.smart
		if smart.GetEnabled() != want.enabled || len(smart.GetRules()) != 4 || len(smart.GetSupportedTargets()) != 3 || smart.GetRules()[0].GetTarget() != human {
			t.Fatalf("%s: %v", name, smart)
		}
	}
	supported := strings.Join(got.GetSupportedEvents(), ",")
	for _, want := range []string{"MotionDetection", "LineDetection", "FieldDetection"} {
		if !strings.Contains(supported, want) {
			t.Fatalf("supported events %q lacks %s", supported, want)
		}
	}
	if strings.Contains(supported, "VideoLoss") {
		t.Fatalf("camera reports no video-loss support, got %q", supported)
	}

	snapshot, err := svc.GetSnapshot(context.Background(), &hikvisionv1.GetSnapshotRequest{})
	if err != nil || len(snapshot.GetJpeg()) == 0 {
		t.Fatalf("snapshot: %v", err)
	}
}

func TestRejectedCredentialsAreNotRetried(t *testing.T) {
	camera := newFakeCamera(t)
	client := camera.client(t, "wrong password")
	for range 3 {
		if _, err := client.DeviceInfo(context.Background()); err == nil {
			t.Fatal("wrong password was accepted")
		}
	}
	if n := camera.authorisedRequests.Load(); n != 1 {
		t.Fatalf("camera saw %d login attempts; want 1, so it does not lock the account", n)
	}
}

// testQuiet stands in for eventEndsAfterQuiet so tests do not wait 10 s.
const testQuiet = 300 * time.Millisecond

func nextEvent(t *testing.T, events <-chan cameraEvent, what string) cameraEvent {
	t.Helper()
	select {
	case event, open := <-events:
		if !open {
			t.Fatalf("%s: subscription closed", what)
		}
		return event
	case <-time.After(5 * time.Second):
		t.Fatalf("%s: nothing received", what)
	}
	return cameraEvent{}
}

func expectNoEvent(t *testing.T, events <-chan cameraEvent, within time.Duration) {
	t.Helper()
	select {
	case event := <-events:
		t.Fatalf("unexpected event: %+v", event)
	case <-time.After(within):
	}
}

func TestRecordedMotionIsOneEvent(t *testing.T) {
	camera := newFakeCamera(t)
	plugin := newPlugin(camera.client(t, testPassword))
	plugin.stream.episodes = newEpisodeTracker(testQuiet, plugin.stream.publish)
	events, unsubscribe := plugin.stream.Subscribe()
	defer unsubscribe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go plugin.stream.Run(ctx)

	// The recording has 12 motion notifications, one a second by the camera's
	// clock, and 2 keep-alives, with the camera's closing delimiter after each
	// motion part. They arrive together, so they are one event.
	start := nextEvent(t, events, "start")
	if start.Kind != hikvisionv1.EventKind_EVENT_KIND_MOTION || start.State != hikvisionv1.EventState_EVENT_STATE_ACTIVE ||
		start.Target != hikvisionv1.DetectionTarget_DETECTION_TARGET_UNSPECIFIED || start.CameraTime.IsZero() {
		t.Fatalf("start: %+v", start)
	}
	<-camera.streamOpened
	motion := map[string]string{"kind": "motion", "target": "unspecified"}
	families := gather(t, plugin.metrics)
	if v := metricValue(families, "gohome_hikvision_events_total", motion); v != 1 {
		t.Fatalf("motion events = %v, want 1", v)
	}
	if v := metricValue(families, "gohome_hikvision_event_active", motion); v != 1 {
		t.Fatalf("motion active during the event = %v, want 1", v)
	}

	end := nextEvent(t, events, "end")
	if end.Kind != start.Kind || end.State != hikvisionv1.EventState_EVENT_STATE_INACTIVE || !end.CameraTime.After(start.CameraTime) {
		t.Fatalf("end: %+v; want the last notification of the event that started %v", end, start.CameraTime)
	}
	expectNoEvent(t, events, 2*testQuiet)
	families = gather(t, plugin.metrics)
	if v := metricValue(families, "gohome_hikvision_event_active", motion); v != 0 {
		t.Fatalf("motion active after the event = %v, want 0", v)
	}
	if v := metricValue(families, "gohome_hikvision_events_total", motion); v != 1 {
		t.Fatalf("motion events = %v, want 1", v)
	}
	if v := metricValue(families, "gohome_hikvision_alert_stream_connected", nil); v != 1 {
		t.Fatalf("alert stream connected = %v", v)
	}
}

func motionNotification() cameraEvent {
	return cameraEvent{Kind: hikvisionv1.EventKind_EVENT_KIND_MOTION, State: hikvisionv1.EventState_EVENT_STATE_ACTIVE, ReceivedAt: time.Now()}
}

// Repeats closer together than the quiet period are one event; a longer gap starts another.
func TestRepeatsAreOneEventUntilQuiet(t *testing.T) {
	stream := newAlertStream(nil, func(cameraEvent) {})
	stream.episodes = newEpisodeTracker(testQuiet, stream.publish)
	events, unsubscribe := stream.Subscribe()
	defer unsubscribe()

	for range 5 {
		stream.episodes.notify(motionNotification())
		time.Sleep(testQuiet / 3)
	}
	if first := nextEvent(t, events, "first start"); first.State != hikvisionv1.EventState_EVENT_STATE_ACTIVE {
		t.Fatalf("first: %+v", first)
	}
	if end := nextEvent(t, events, "first end"); end.State != hikvisionv1.EventState_EVENT_STATE_INACTIVE {
		t.Fatalf("want the end after a quiet period, got %+v", end)
	}
	stream.episodes.notify(motionNotification())
	if second := nextEvent(t, events, "second start"); second.State != hikvisionv1.EventState_EVENT_STATE_ACTIVE {
		t.Fatalf("second: %+v", second)
	}
}

func TestLateSubscriberGetsEventsUnderWay(t *testing.T) {
	stream := newAlertStream(nil, func(cameraEvent) {})
	stream.episodes = newEpisodeTracker(testQuiet, stream.publish)
	stream.episodes.notify(motionNotification())

	events, unsubscribe := stream.Subscribe()
	defer unsubscribe()
	if start := nextEvent(t, events, "start"); start.State != hikvisionv1.EventState_EVENT_STATE_ACTIVE {
		t.Fatalf("want the start of the motion under way, got %+v", start)
	}
	if end := nextEvent(t, events, "end"); end.State != hikvisionv1.EventState_EVENT_STATE_INACTIVE {
		t.Fatalf("end: %+v", end)
	}
}

// Whether something is happening now is unknown while the event stream is down.
func TestEventActiveAbsentWhileDisconnected(t *testing.T) {
	camera := newFakeCamera(t)
	plugin := newPlugin(camera.client(t, testPassword))
	plugin.metrics.observeTransition(motionNotification())
	families := gather(t, plugin.metrics)
	if _, ok := families["gohome_hikvision_event_active"]; ok {
		t.Fatal("event_active reported while the event stream is disconnected")
	}
	if v := metricValue(families, "gohome_hikvision_events_total", map[string]string{"kind": "motion"}); v != 1 {
		t.Fatalf("motion events = %v, want 1", v)
	}
}

// The recorded stream has only motion events. These follow Hikvision's
// documented alert fields.
func TestSmartEventTargets(t *testing.T) {
	regionTarget := `<EventNotificationAlert version="2.0" xmlns="http://www.hikvision.com/ver20/XMLSchema">
<channelID>1</channelID><dateTime>2000-01-01T12:00:06+00:00</dateTime>
<eventType>linedetection</eventType><eventState>active</eventState>
<DetectionRegionList><DetectionRegionEntry><regionID>2</regionID><detectionTarget>human</detectionTarget></DetectionRegionEntry></DetectionRegionList>
</EventNotificationAlert>`
	topLevelTarget := `<EventNotificationAlert><eventType>fielddetection</eventType><eventState>active</eventState>
<detectionTarget>vehicle</detectionTarget></EventNotificationAlert>`

	line, err := parseEventNotification([]byte(regionTarget), time.Now())
	if err != nil || line.Kind != hikvisionv1.EventKind_EVENT_KIND_LINE_CROSSING ||
		line.Target != hikvisionv1.DetectionTarget_DETECTION_TARGET_HUMAN || line.RuleID != 2 {
		t.Fatalf("line crossing: %+v, %v", line, err)
	}
	intrusion, err := parseEventNotification([]byte(topLevelTarget), time.Now())
	if err != nil || intrusion.Kind != hikvisionv1.EventKind_EVENT_KIND_INTRUSION ||
		intrusion.Target != hikvisionv1.DetectionTarget_DETECTION_TARGET_VEHICLE {
		t.Fatalf("intrusion: %+v, %v", intrusion, err)
	}
}

func TestFailedReadRemovesSettings(t *testing.T) {
	camera := newFakeCamera(t)
	plugin := newPlugin(camera.client(t, testPassword))

	for _, available := range []bool{true, false, true} {
		camera.unavailable.Store(!available)
		families := gather(t, plugin.metrics)
		if got := metricValue(families, "gohome_hikvision_scrape_success", nil); got != boolValue(available) {
			t.Fatalf("available=%v: scrape_success=%v", available, got)
		}
		_, hasInfo := families["gohome_hikvision_info"]
		_, hasDetection := families["gohome_hikvision_detection_enabled"]
		if hasInfo != available || hasDetection != available {
			t.Fatalf("available=%v: info present=%v, detection present=%v", available, hasInfo, hasDetection)
		}
	}
}

func gather(t *testing.T, collector prometheus.Collector) map[string]*dto.MetricFamily {
	t.Helper()
	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*dto.MetricFamily{}
	for _, family := range families {
		byName[family.GetName()] = family
	}
	return byName
}

func metricValue(families map[string]*dto.MetricFamily, name string, labels map[string]string) float64 {
	family, ok := families[name]
	if !ok {
		return -1
	}
	for _, metric := range family.GetMetric() {
		matched := 0
		for _, pair := range metric.GetLabel() {
			if labels[pair.GetName()] == pair.GetValue() {
				matched++
			}
		}
		if matched != len(labels) {
			continue
		}
		switch {
		case metric.GetCounter() != nil:
			return metric.GetCounter().GetValue()
		case metric.GetGauge() != nil:
			return metric.GetGauge().GetValue()
		}
	}
	return -1
}
