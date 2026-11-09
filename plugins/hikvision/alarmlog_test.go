package hikvision

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	hikvisionv1 "github.com/joshp123/gohome/proto/gen/plugins/hikvision/v1"
)

// fakeSearches answers the two searches like the camera: newest first, in
// pages, with responseStatusStrg MORE while more remain.
type fakeSearches struct {
	mu         sync.Mutex
	logLines   []fakeLogLine
	recordings [][2]time.Time
	logReads   int
}

type fakeLogLine struct {
	metaID string
	at     time.Time
}

func (f *fakeSearches) log(metaID string, at time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLines = append(f.logLines, fakeLogLine{metaID, at})
}

// alarm logs a motion alarm the way the camera does: a start and, five seconds later, a stop.
func (f *fakeSearches) alarm(start time.Time, stopped bool) {
	f.log("log.hikvision.com/Alarm/motionStart/1", start)
	if stopped {
		f.log("log.hikvision.com/Alarm/motionStop/1", start.Add(5*time.Second))
	}
}

func (f *fakeSearches) serve(t *testing.T, w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var terms struct {
		MetaID     string   `xml:"metaId"`
		TrackIDs   []string `xml:"trackIDList>trackID"`
		Start      string   `xml:"timeSpanList>timeSpan>startTime"`
		End        string   `xml:"timeSpanList>timeSpan>endTime"`
		MaxResults int      `xml:"maxResults"`
		Position   int      `xml:"searchResultPostion"`
	}
	if err := xml.Unmarshal(body, &terms); err != nil {
		t.Errorf("search terms: %v", err)
		return
	}
	start, _ := time.Parse(isapiTimeLayout, terms.Start)
	end, _ := time.Parse(isapiTimeLayout, terms.End)
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/xml")
	if r.URL.Path == pathRecordingSearch {
		var items string
		for _, rec := range f.recordings {
			if rec[0].Before(end) && rec[1].After(start) {
				items += fmt.Sprintf("<searchMatchItem><trackID>101</trackID><timeSpan><startTime>%s</startTime><endTime>%s</endTime></timeSpan></searchMatchItem>",
					rec[0].Format(isapiTimeLayout), rec[1].Format(isapiTimeLayout))
			}
		}
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><CMSearchResult><responseStatus>true</responseStatus><matchList>%s</matchList></CMSearchResult>`, items)
		return
	}
	f.logReads++
	var lines []fakeLogLine
	for _, line := range f.logLines {
		if !line.at.Before(start) && !line.at.After(end) && strings.HasPrefix(line.metaID, "log.hikvision.com/Alarm") {
			lines = append(lines, line)
		}
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].at.After(lines[j].at) })
	page := lines[min(terms.Position, len(lines)):]
	more := len(page) > terms.MaxResults
	page = page[:min(len(page), terms.MaxResults)]
	status := "OK"
	switch {
	case len(lines) == 0:
		status = "NO MATCHES"
	case more:
		status = "MORE"
	}
	var items string
	for _, line := range page {
		items += fmt.Sprintf("<searchMatchItem><logDescriptor><metaId>%s</metaId><StartDateTime>%s</StartDateTime></logDescriptor></searchMatchItem>",
			line.metaID, line.at.UTC().Format(isapiTimeLayout))
	}
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><CMSearchResult><responseStatus>true</responseStatus><responseStatusStrg>%s</responseStatusStrg><totalMatches>%d</totalMatches><matchList>%s</matchList></CMSearchResult>`,
		status, len(lines), items)
}

func TestParseAlarmLogEntry(t *testing.T) {
	at := "2000-01-01T12:00:00Z"
	start, ok := parseAlarmLogEntry("log.hikvision.com/Alarm/motionStart/1", at)
	if !ok || !start.starts || start.alarmType != "motion" || start.channel != 1 || start.logType != "motionStart" {
		t.Fatalf("start: %+v %v", start, ok)
	}
	stop, ok := parseAlarmLogEntry("log.hikvision.com/Alarm/motionStop/1", at)
	if !ok || stop.starts || stop.alarmType != "motion" {
		t.Fatalf("stop: %+v %v", stop, ok)
	}
	for _, metaID := range []string{"log.hikvision.com/Operation/remoteGetPara", "log.hikvision.com/Alarm/somethingElse/1", "log.hikvision.com/Alarm/motionStart/x"} {
		if _, ok := parseAlarmLogEntry(metaID, at); ok {
			t.Errorf("%s parsed as an alarm", metaID)
		}
	}
	if _, ok := parseAlarmLogEntry("log.hikvision.com/Alarm/motionStart/1", "yesterday"); ok {
		t.Error("bad time parsed")
	}
}

func TestPairAlarms(t *testing.T) {
	base := time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)
	line := func(word string, offset time.Duration) alarmLogEntry {
		entry, _ := parseAlarmLogEntry("log.hikvision.com/Alarm/"+word+"/1", base.Add(offset).Format(isapiTimeLayout))
		return entry
	}
	alarms := pairAlarms([]alarmLogEntry{
		line("motionStop", 0), // its start is before the search: left out
		line("motionStart", 10*time.Second), line("motionStop", 15*time.Second),
		line("motionStart", 20*time.Second), // under way
	})
	if len(alarms) != 2 {
		t.Fatalf("want 2 alarms, got %d", len(alarms))
	}
	if alarms[0].Start != base.Add(10*time.Second) || alarms[0].End != base.Add(15*time.Second) || alarms[0].Kind != hikvisionv1.EventKind_EVENT_KIND_MOTION {
		t.Fatalf("first: %+v", alarms[0])
	}
	if !alarms[1].End.IsZero() {
		t.Fatal("open alarm has an end")
	}
}

// testAlarmLog is an alarmLog on a fake camera with a controllable clock.
type testAlarmLog struct {
	camera  *fakeCamera
	log     *alarmLog
	now     time.Time
	started []alarm
}

func newTestAlarmLog(t *testing.T) *testAlarmLog {
	camera := newFakeCamera(t)
	test := &testAlarmLog{camera: camera, now: time.Now().UTC().Truncate(time.Second)}
	test.log = newAlarmLog(camera.client(t, testPassword), func(a alarm) { test.started = append(test.started, a) })
	test.log.now = func() time.Time { return test.now }
	return test
}

func (a *testAlarmLog) poll(after time.Duration) {
	a.now = a.now.Add(after)
	a.log.poll(context.Background())
}

func nextAlarm(t *testing.T, sub *alarmSubscription) alarm {
	t.Helper()
	select {
	case a, open := <-sub.alarms:
		if !open {
			t.Fatalf("subscription ended: %v", sub.ended)
		}
		return a
	case <-time.After(time.Second):
		t.Fatal("no alarm")
	}
	return alarm{}
}

func noAlarm(t *testing.T, sub *alarmSubscription) {
	t.Helper()
	select {
	case a := <-sub.alarms:
		t.Fatalf("unexpected alarm %+v", a)
	default:
	}
}

func TestAlarmLogDoesNotReplayHistoryAndSendsEachAlarmOnce(t *testing.T) {
	test := newTestAlarmLog(t)
	old := test.now.Add(-2 * time.Hour)
	test.camera.searches.alarm(old, true)
	test.camera.searches.alarm(test.now.Add(-time.Minute), true)

	early := test.log.Subscribe()
	defer test.log.Unsubscribe(early)
	test.poll(0)
	noAlarm(t, early)
	if len(test.started) != 0 {
		t.Fatalf("history sent as new: %+v", test.started)
	}
	if latest, ok := test.log.LatestAlarmStart(); !ok || !latest.Equal(test.now.Add(-time.Minute)) {
		t.Fatalf("latest alarm from history: %v %v", latest, ok)
	}
	sub := test.log.Subscribe()
	defer test.log.Unsubscribe(sub)

	start := test.now.Add(2 * time.Second)
	test.camera.searches.alarm(start, false)
	test.poll(5 * time.Second)
	if a := nextAlarm(t, sub); a.state() != hikvisionv1.EventState_EVENT_STATE_ACTIVE || !a.Start.Equal(start) || !a.SeenStart.Equal(test.now) {
		t.Fatalf("start: %+v", a)
	}
	test.poll(5 * time.Second)
	noAlarm(t, sub)

	test.camera.searches.log("log.hikvision.com/Alarm/motionStop/1", start.Add(5*time.Second))
	test.poll(5 * time.Second)
	end := nextAlarm(t, sub)
	if end.state() != hikvisionv1.EventState_EVENT_STATE_INACTIVE || end.End.Sub(end.Start) != 5*time.Second {
		t.Fatalf("end: %+v", end)
	}
	test.poll(5 * time.Second)
	noAlarm(t, sub)
	if len(test.started) != 1 {
		t.Fatalf("counted %d alarms, want 1", len(test.started))
	}

	// A whole alarm logged between two reads arrives as a start and an end.
	quick := test.now.Add(time.Second)
	test.camera.searches.alarm(quick, true)
	test.poll(10 * time.Second)
	if a := nextAlarm(t, sub); a.state() != hikvisionv1.EventState_EVENT_STATE_ACTIVE || !a.Start.Equal(quick) {
		t.Fatalf("quick start: %+v", a)
	}
	if a := nextAlarm(t, sub); a.state() != hikvisionv1.EventState_EVENT_STATE_INACTIVE {
		t.Fatalf("quick end: %+v", a)
	}
}

func TestAlarmLogCatchesUpAfterOutage(t *testing.T) {
	test := newTestAlarmLog(t)
	test.poll(0)
	sub := test.log.Subscribe()
	defer test.log.Unsubscribe(sub)
	test.camera.unavailable.Store(true)
	during := test.now.Add(20 * time.Second)
	test.camera.searches.alarm(during, true)
	for range 6 {
		test.poll(5 * time.Second)
	}
	if !test.log.Status().Readable {
		t.Fatal("30 s of failed reads reported the log unreadable")
	}
	test.poll(40 * time.Second)
	if test.log.Status().Readable {
		t.Fatal("log reported readable after a minute of failures")
	}
	// A watcher that connects during the outage is not refused.
	late := test.log.Subscribe()
	defer test.log.Unsubscribe(late)
	test.camera.unavailable.Store(false)
	test.poll(5 * time.Second)
	if len(test.started) != 1 || !test.started[0].Start.Equal(during) {
		t.Fatalf("alarm during the outage: %+v", test.started)
	}
	for _, watcher := range []*alarmSubscription{sub, late} {
		start := nextAlarm(t, watcher)
		end := nextAlarm(t, watcher)
		if !start.Start.Equal(during) || start.state() != hikvisionv1.EventState_EVENT_STATE_ACTIVE ||
			end.state() != hikvisionv1.EventState_EVENT_STATE_INACTIVE || !start.SeenStart.Equal(test.now) {
			t.Fatalf("watcher missed the alarm during the outage: %+v %+v", start, end)
		}
	}
}

func TestAlarmLogStartingDuringOutageKeepsNewAlarms(t *testing.T) {
	test := newTestAlarmLog(t)
	history := test.now.Add(-time.Hour)
	test.camera.searches.alarm(history, true)
	test.camera.unavailable.Store(true)
	test.poll(0) // gohome starts; the camera does not answer
	sub := test.log.Subscribe()
	defer test.log.Unsubscribe(sub)
	raised := test.now.Add(10 * time.Second)
	test.camera.searches.alarm(raised, true)
	test.poll(75 * time.Second)
	test.camera.unavailable.Store(false)
	test.poll(5 * time.Second)
	if len(test.started) != 1 || !test.started[0].Start.Equal(raised) {
		t.Fatalf("counted %+v, want only the alarm raised after start", test.started)
	}
	if start := nextAlarm(t, sub); !start.Start.Equal(raised) {
		t.Fatalf("watcher got %+v", start)
	}
	if end := nextAlarm(t, sub); end.state() != hikvisionv1.EventState_EVENT_STATE_INACTIVE {
		t.Fatalf("watcher end %+v", end)
	}
	noAlarm(t, sub)
	if latest, _ := test.log.LatestAlarmStart(); !latest.Equal(raised) {
		t.Fatalf("latest alarm %v", latest)
	}
}

func TestAlarmLogReadsEveryPage(t *testing.T) {
	test := newTestAlarmLog(t)
	first := test.now.Add(-6 * 24 * time.Hour)
	for i := range 140 {
		test.camera.searches.alarm(first.Add(time.Duration(i)*time.Hour), true)
	}
	alarms, err := test.camera.client(t, testPassword).ListAlarms(context.Background(), first.Add(-time.Minute), test.now)
	if err != nil {
		t.Fatal(err)
	}
	if len(alarms) != 140 {
		t.Fatalf("listed %d alarms, want 140", len(alarms))
	}
	for i, a := range alarms {
		if !a.Start.Equal(first.Add(time.Duration(i)*time.Hour)) || a.End.Sub(a.Start) != 5*time.Second {
			t.Fatalf("alarm %d: %+v", i, a)
		}
	}
}

func TestAlarmMetrics(t *testing.T) {
	test := newTestAlarmLog(t)
	collector := NewMetricsCollector(test.camera.client(t, testPassword))
	collector.stream = connectedStream()
	collector.alarms = test.log
	test.log.onStarted = collector.alarmStarted
	test.camera.searches.alarm(test.now.Add(-time.Hour), true)
	test.poll(0)
	start := test.now.Add(time.Second)
	test.camera.searches.alarm(start, true)
	test.poll(5 * time.Second)

	families := gather(t, collector)
	if v := metricValue(families, "gohome_hikvision_alarms_total", map[string]string{"kind": "motion"}); v != 1 {
		t.Fatalf("alarms_total %v, want 1 (history is not counted)", v)
	}
	if v := metricValue(families, "gohome_hikvision_last_alarm_start_timestamp_seconds", map[string]string{"kind": "motion"}); v != float64(start.Unix()) {
		t.Fatalf("last alarm start %v, want %d", v, start.Unix())
	}
	if v := metricValue(families, "gohome_hikvision_alarm_log_readable", nil); v != 1 {
		t.Fatalf("alarm log readable %v", v)
	}

	test.camera.unavailable.Store(true)
	test.poll(alarmLogUnreadableAfter + time.Second)
	families = gather(t, collector)
	if _, ok := families["gohome_hikvision_last_alarm_start_timestamp_seconds"]; ok {
		t.Fatal("last alarm start reported while the log is unreadable")
	}
	if v := metricValue(families, "gohome_hikvision_alarm_log_readable", nil); v != 0 {
		t.Fatalf("alarm log readable %v while failing", v)
	}
}

func TestClientOnlyPostsSearches(t *testing.T) {
	camera := newFakeCamera(t)
	client := camera.client(t, testPassword)
	if _, err := client.request(context.Background(), client.readClient, http.MethodPost, pathDeviceInfo, []byte("<x/>")); err == nil {
		t.Fatal("client POSTed to a settings resource")
	}
	if _, err := client.request(context.Background(), client.readClient, http.MethodPut, pathLogSearch, []byte("<x/>")); err == nil {
		t.Fatal("client sent a PUT")
	}
}

func TestPlaybackPlaylistKeepsCredentialsInsideTheURL(t *testing.T) {
	client, err := NewClient(Config{BaseURL: "http://192.0.2.20:85", Username: "agent", Password: "it's@me/' \n:x"})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2000, 1, 1, 12, 0, 7, 0, time.UTC)
	playlist, err := client.playbackPlaylist(at)
	if err != nil {
		t.Fatal(err)
	}
	want := "ffconcat version 1.0\nfile 'rtsp://agent:it%27s%40me%2F%27%20%0A%3Ax@192.0.2.20:554/Streaming/tracks/101/?starttime=20000101T120007Z'\noption rtsp_transport tcp\n"
	if playlist != want {
		t.Fatalf("playlist:\n%q\nwant\n%q", playlist, want)
	}
}

func TestRecordedFrameRoutes(t *testing.T) {
	test := newTestAlarmLog(t)
	client := test.camera.client(t, testPassword)
	plugin := Plugin{client: client, alarms: test.log, frames: newRecordedFrames(client)}
	mux := http.NewServeMux()
	plugin.RegisterHTTP(mux)
	get := func(method, target string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(method, target, nil))
		return response
	}
	if r := get("GET", frameEndpoint+"?at=yesterday"); r.Code != 400 {
		t.Fatalf("bad time: %d", r.Code)
	}
	if r := get("POST", frameEndpoint+"?at=2000-01-01T12:00:00Z"); r.Code != 405 {
		t.Fatalf("POST: %d", r.Code)
	}
	if r := get("GET", frameEndpoint+"?at=2000-01-01T12:00:00Z"); r.Code != 404 || r.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("no recording: %d %v", r.Code, r.Header())
	}
	if r := get("GET", alarmFrameEndpoint); r.Code != 404 {
		t.Fatalf("no alarm known: %d", r.Code)
	}
	test.camera.searches.alarm(test.now.Add(-time.Minute), true)
	test.poll(0)
	// The alarm is known but the camera has no recording of it.
	if r := get("GET", alarmFrameEndpoint); r.Code != 404 {
		t.Fatalf("alarm without recording: %d", r.Code)
	}
}

// Opt-in read-only probe: lists the real camera's alarms through the
// production code. Set HIKVISION_BASE_URL, HIKVISION_CREDENTIALS_FILE and
// HIKVISION_ALARMS_SINCE (a duration such as 216h).
func TestLiveAlarmLog(t *testing.T) {
	client := liveClient(t)
	period, err := time.ParseDuration(os.Getenv("HIKVISION_ALARMS_SINCE"))
	if err != nil {
		t.Skip("set HIKVISION_ALARMS_SINCE")
	}
	alarms, err := client.ListAlarms(context.Background(), time.Now().Add(-period), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range alarms {
		end := "under way"
		if !a.End.IsZero() {
			end = a.End.UTC().Format(time.RFC3339)
		}
		fmt.Printf("%s\t%s\t%s\n", a.Start.UTC().Format(time.RFC3339), end, a.LogType)
	}
	fmt.Printf("alarms: %d\n", len(alarms))
}

func liveClient(t *testing.T) *Client {
	t.Helper()
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
	return client
}

// Opt-in read-only probe: reads the recorded picture at the latest alarm's
// start through the production code, and reports only its size and timing.
// Needs ffmpeg on PATH.
func TestLiveRecordedFrame(t *testing.T) {
	client := liveClient(t)
	alarms, err := client.ListAlarms(context.Background(), time.Now().Add(-7*24*time.Hour), time.Now())
	if err != nil || len(alarms) == 0 {
		t.Fatalf("no alarm to read a picture for: %v", err)
	}
	at := alarms[len(alarms)-1].Start
	started := time.Now()
	jpeg, second, err := newRecordedFrames(client).Frame(context.Background(), at)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("alarm %s: %d-byte JPEG for %s in %s\n", at.Format(time.RFC3339), len(jpeg), second.Format(time.RFC3339), time.Since(started).Round(100*time.Millisecond))
}

// Opt-in read-only probe: runs the production alarm-log poller and watcher
// against the real camera and prints every alarm it sends, until the first
// alarm ends or HIKVISION_WATCH_FOR (e.g. 6h) passes.
func TestLiveWatchAlarms(t *testing.T) {
	client := liveClient(t)
	watchFor, err := time.ParseDuration(os.Getenv("HIKVISION_WATCH_FOR"))
	if err != nil {
		t.Skip("set HIKVISION_WATCH_FOR")
	}
	collector := NewMetricsCollector(client)
	alarms := newAlarmLog(client, collector.alarmStarted)
	ctx, cancel := context.WithTimeout(context.Background(), watchFor)
	defer cancel()
	sub := alarms.Subscribe()
	defer alarms.Unsubscribe(sub)
	go alarms.Run(ctx)
	fmt.Printf("%s watching\n", time.Now().UTC().Format(time.RFC3339))
	for {
		select {
		case <-ctx.Done():
			fmt.Println("no alarm before the deadline")
			return
		case a := <-sub.alarms:
			fmt.Printf("%s sent %s started %s ended %s seen %s/%s\n", time.Now().UTC().Format(time.RFC3339), a.state(),
				a.Start.UTC().Format(time.RFC3339), a.End.UTC().Format(time.RFC3339), a.SeenStart.UTC().Format(time.RFC3339), a.SeenEnd.UTC().Format(time.RFC3339))
			if !a.End.IsZero() {
				return
			}
		}
	}
}

func TestAlarmPollIntervalFromConfig(t *testing.T) {
	credentials := filepath.Join(t.TempDir(), "camera")
	if err := os.WriteFile(credentials, []byte("agent:secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for seconds, want := range map[uint32]time.Duration{0: 5 * time.Second, 1: time.Second, 3: 3 * time.Second} {
		cfg, err := ConfigFromProto(&hikvisionv1.HikvisionConfig{BaseUrl: "http://192.0.2.20", CredentialsFile: credentials, AlarmPollSeconds: seconds})
		if err != nil || cfg.AlarmPollInterval != want {
			t.Fatalf("alarm_poll_seconds %d: %v %v, want %v", seconds, cfg.AlarmPollInterval, err, want)
		}
	}
}
