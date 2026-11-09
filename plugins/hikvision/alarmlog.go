package hikvision

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	hikvisionv1 "github.com/joshp123/gohome/proto/gen/plugins/hikvision/v1"
)

// The camera's alarm log holds the alarms it raised: those its detection
// filter accepted, which for motion filtered to people and vehicles means a
// person or vehicle. The raw alert stream, by contrast, carries every
// reaction of the motion detector before that filter.
const (
	alarmLogPollInterval = 5 * time.Second
	// A poll searches from this long before the previous successful poll, so
	// an alarm logged late, or during an outage, is still found.
	alarmLogOverlap = 2 * time.Minute
	// How far back the first read looks, to learn the latest alarm. Alarms in
	// it are history and never sent as new.
	alarmLogBaseline = 7 * 24 * time.Hour
	// The search ends this far after now, in case the camera clock runs ahead.
	alarmLogClockAhead = time.Hour
	// The log counts as unreadable once no read has succeeded for this long.
	alarmLogUnreadableAfter = time.Minute
	alarmLogPageSize        = 100
	alarmLogMaxEntries      = 5000
	// An alarm whose end never appears is forgotten after this long.
	alarmForgetOpenAfter = 24 * time.Hour
)

var errAlarmSubBehind = errors.New("subscriber fell behind the camera's alarms")

type logSearchXML struct {
	XMLName    xml.Name `xml:"CMSearchDescription"`
	SearchID   string   `xml:"searchID"`
	MetaID     string   `xml:"metaId"`
	Start      string   `xml:"timeSpanList>timeSpan>startTime"`
	End        string   `xml:"timeSpanList>timeSpan>endTime"`
	MaxResults int      `xml:"maxResults"`
	Position   int      `xml:"searchResultPostion"` // sic: ISAPI's spelling
}

type logSearchResultXML struct {
	Status  string `xml:"responseStatusStrg"` // OK, MORE or NO MATCHES
	Entries []struct {
		MetaID string `xml:"logDescriptor>metaId"`
		Time   string `xml:"logDescriptor>StartDateTime"`
	} `xml:"matchList>searchMatchItem"`
}

// alarmLogEntry is one line of the alarm log: an alarm starting or stopping.
type alarmLogEntry struct {
	alarmType string // the alarm, without Start/Stop, e.g. "motion"
	logType   string // the camera's own word, e.g. "motionStart"
	starts    bool
	channel   uint32
	at        time.Time
}

const isapiTimeLayout = "2006-01-02T15:04:05Z"

// parseAlarmLogEntry reads a metaId such as log.hikvision.com/Alarm/motionStart/1.
// It returns false for lines that are not an alarm starting or stopping.
func parseAlarmLogEntry(metaID, at string) (alarmLogEntry, bool) {
	parts := strings.Split(metaID, "/")
	if len(parts) < 3 || parts[1] != "Alarm" {
		return alarmLogEntry{}, false
	}
	entry := alarmLogEntry{logType: parts[2]}
	switch {
	case strings.HasSuffix(entry.logType, "Start"):
		entry.alarmType, entry.starts = strings.TrimSuffix(entry.logType, "Start"), true
	case strings.HasSuffix(entry.logType, "Stop"):
		entry.alarmType = strings.TrimSuffix(entry.logType, "Stop")
	default:
		return alarmLogEntry{}, false
	}
	if len(parts) > 3 {
		channel, err := strconv.ParseUint(parts[3], 10, 32)
		if err != nil {
			return alarmLogEntry{}, false
		}
		entry.channel = uint32(channel)
	}
	t, err := time.Parse(isapiTimeLayout, at)
	if err != nil {
		return alarmLogEntry{}, false
	}
	entry.at = t
	return entry, entry.alarmType != ""
}

var alarmKinds = map[string]hikvisionv1.EventKind{
	"motion": hikvisionv1.EventKind_EVENT_KIND_MOTION,
}

func alarmKind(alarmType string) hikvisionv1.EventKind {
	if kind, ok := alarmKinds[strings.ToLower(alarmType)]; ok {
		return kind
	}
	return hikvisionv1.EventKind_EVENT_KIND_OTHER
}

// readAlarmLog returns the alarm starts and stops logged between since and
// until, oldest first.
func (c *Client) readAlarmLog(ctx context.Context, since, until time.Time) ([]alarmLogEntry, error) {
	search := logSearchXML{
		SearchID:   newSearchID(),
		MetaID:     "log.std-cgi.com/Alarm",
		Start:      since.UTC().Format(isapiTimeLayout),
		End:        until.UTC().Format(isapiTimeLayout),
		MaxResults: alarmLogPageSize,
	}
	seen := map[alarmLogEntry]bool{}
	var entries []alarmLogEntry
	for {
		var result logSearchResultXML
		if err := c.search(ctx, pathLogSearch, search, &result); err != nil {
			return nil, err
		}
		for _, item := range result.Entries {
			// Pages shift when the camera logs something new; a repeat is the same line.
			if entry, ok := parseAlarmLogEntry(item.MetaID, item.Time); ok && !seen[entry] {
				seen[entry] = true
				entries = append(entries, entry)
			}
		}
		search.Position += len(result.Entries)
		if result.Status != "MORE" || len(result.Entries) == 0 {
			break
		}
		if search.Position >= alarmLogMaxEntries {
			return nil, fmt.Errorf("alarm log search: more than %d entries", alarmLogMaxEntries)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if !entries[i].at.Equal(entries[j].at) {
			return entries[i].at.Before(entries[j].at)
		}
		return entries[i].starts && !entries[j].starts
	})
	return entries, nil
}

func newSearchID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	h := hex.EncodeToString(b)
	return strings.ToUpper(h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32])
}

// alarm is one alarm, paired from its start and stop lines.
type alarm struct {
	Kind      hikvisionv1.EventKind
	AlarmType string // e.g. "motion"
	LogType   string // the start line's word, e.g. "motionStart"
	Channel   uint32
	Start     time.Time
	End       time.Time // zero while under way
	SeenStart time.Time // when gohome first read the start; zero in a listing
	SeenEnd   time.Time // when gohome first read the end
	announced bool      // its start was sent as new
}

func (a *alarm) state() hikvisionv1.EventState {
	if a.End.IsZero() {
		return hikvisionv1.EventState_EVENT_STATE_ACTIVE
	}
	return hikvisionv1.EventState_EVENT_STATE_INACTIVE
}

type alarmKey struct {
	alarmType string
	channel   uint32
	start     time.Time
}

func (a *alarm) key() alarmKey { return alarmKey{a.AlarmType, a.Channel, a.Start} }

// pairAlarms turns log lines, oldest first, into alarms, oldest first. A stop
// ends the latest open alarm of its type and channel; a stop whose start is
// not among the lines is left out.
func pairAlarms(entries []alarmLogEntry) []*alarm {
	var alarms []*alarm
	open := map[string]*alarm{}
	for _, entry := range entries {
		slot := entry.alarmType + "/" + strconv.FormatUint(uint64(entry.channel), 10)
		if entry.starts {
			a := &alarm{Kind: alarmKind(entry.alarmType), AlarmType: entry.alarmType, LogType: entry.logType,
				Channel: entry.channel, Start: entry.at}
			alarms = append(alarms, a)
			open[slot] = a
			continue
		}
		if a := open[slot]; a != nil && !entry.at.Before(a.Start) {
			a.End = entry.at
			delete(open, slot)
		}
	}
	return alarms
}

// alarmSubscription carries alarms' starts and ends to one WatchAlarms caller.
type alarmSubscription struct {
	alarms chan alarm
	ended  error // why alarms was closed; set before the close
}

// alarmLog polls the camera's alarm log and passes each new alarm's start
// and end to the metrics and to every WatchAlarms subscriber.
type alarmLog struct {
	client       *Client
	onStarted    func(alarm)
	pollInterval time.Duration
	now          func() time.Time

	mu sync.Mutex
	// startedAt divides history from new alarms: an alarm that starts before
	// it is never sent as new. It is set when the first read is attempted,
	// whether or not the camera answers.
	startedAt    time.Time
	baselined    bool      // a read has succeeded
	lastRead     time.Time // last successful read, gohome clock
	lastSearched time.Time // the until of the search that started the last read
	failingSince time.Time
	lastError    string
	known        map[alarmKey]*alarm
	latestStart  map[hikvisionv1.EventKind]time.Time
	subscribers  map[*alarmSubscription]struct{}
}

func newAlarmLog(client *Client, onStarted func(alarm)) *alarmLog {
	return &alarmLog{client: client, onStarted: onStarted, pollInterval: alarmLogPollInterval, now: time.Now,
		known: map[alarmKey]*alarm{}, latestStart: map[hikvisionv1.EventKind]time.Time{},
		subscribers: map[*alarmSubscription]struct{}{}}
}

// Run polls until ctx ends.
func (l *alarmLog) Run(ctx context.Context) {
	for {
		l.poll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(l.pollInterval):
		}
	}
}

func (l *alarmLog) poll(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	now := l.now()
	l.mu.Lock()
	if l.startedAt.IsZero() {
		l.startedAt = now
	}
	since := now.Add(-alarmLogBaseline)
	if l.baselined {
		since = l.lastSearched.Add(-alarmLogOverlap)
	}
	l.mu.Unlock()
	entries, err := l.client.readAlarmLog(ctx, since, now.Add(alarmLogClockAhead))
	if err != nil {
		l.failed(now, err)
		return
	}
	l.observe(now, since, entries)
}

// failed records a failed read. Subscribers stay connected: the log keeps
// the alarms, and the next successful read sends what they missed.
func (l *alarmLog) failed(now time.Time, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.lastError == "" {
		log.Printf("hikvision alarm log: %v", err)
	}
	l.lastError = err.Error()
	if l.failingSince.IsZero() {
		l.failingSince = now
	}
}

// observe takes one successful read of the log from since onwards.
func (l *alarmLog) observe(now, since time.Time, entries []alarmLogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.lastError != "" {
		log.Printf("hikvision alarm log readable again")
	}
	l.lastRead, l.lastSearched, l.failingSince, l.lastError = now, now, time.Time{}, ""
	l.baselined = true
	for _, read := range pairAlarms(entries) {
		current, ok := l.known[read.key()]
		if !ok {
			announce := !read.Start.Before(l.startedAt)
			// The end, if this read already has it, is applied below.
			started := *read
			started.End, started.SeenStart, started.announced = time.Time{}, now, announce
			current = &started
			l.known[read.key()] = current
			if start := l.latestStart[current.Kind]; current.Start.After(start) {
				l.latestStart[current.Kind] = current.Start
			}
			if announce {
				l.onStarted(*current)
				l.publishLocked(*current)
			}
		}
		if !read.End.IsZero() && current.End.IsZero() {
			current.End, current.SeenEnd = read.End, now
			if current.announced {
				l.publishLocked(*current)
			}
		}
	}
	// Forget alarms the next search no longer covers, and open alarms whose end never came.
	for key, known := range l.known {
		if (!known.End.IsZero() && known.End.Before(since)) || now.Sub(known.Start) > alarmForgetOpenAfter {
			delete(l.known, key)
		}
	}
}

func (l *alarmLog) readableLocked() bool {
	return !l.lastRead.IsZero() && l.now().Sub(l.lastRead) < alarmLogUnreadableAfter
}

// Subscribe returns new alarms' starts and ends, beginning with the starts of
// announced alarms under way. While the log is unreadable the subscription
// stays open, and alarms logged meanwhile arrive once it is readable again.
func (l *alarmLog) Subscribe() *alarmSubscription {
	l.mu.Lock()
	defer l.mu.Unlock()
	sub := &alarmSubscription{alarms: make(chan alarm, subscriberBuffer)}
	var underWay []alarm
	for _, known := range l.known {
		if known.announced && known.End.IsZero() {
			underWay = append(underWay, *known)
		}
	}
	sort.Slice(underWay, func(i, j int) bool { return underWay[i].Start.Before(underWay[j].Start) })
	for _, a := range underWay {
		sub.alarms <- a
	}
	l.subscribers[sub] = struct{}{}
	return sub
}

// Unsubscribe ends a subscription that is still open.
func (l *alarmLog) Unsubscribe(sub *alarmSubscription) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.endLocked(sub, nil)
}

func (l *alarmLog) endLocked(sub *alarmSubscription, why error) {
	if _, ok := l.subscribers[sub]; ok {
		delete(l.subscribers, sub)
		sub.ended = why
		close(sub.alarms)
	}
}

func (l *alarmLog) publishLocked(a alarm) {
	for sub := range l.subscribers {
		select {
		case sub.alarms <- a:
		default:
			l.endLocked(sub, errAlarmSubBehind)
		}
	}
}

// alarmLogStatus is what the metrics report about the alarm log.
type alarmLogStatus struct {
	Readable    bool
	LastRead    time.Time
	LatestStart map[hikvisionv1.EventKind]time.Time
}

func (l *alarmLog) Status() alarmLogStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	status := alarmLogStatus{Readable: l.readableLocked(), LastRead: l.lastRead,
		LatestStart: map[hikvisionv1.EventKind]time.Time{}}
	for kind, start := range l.latestStart {
		status.LatestStart[kind] = start
	}
	return status
}

// LatestAlarmStart is the start of the latest alarm of any kind, if known.
func (l *alarmLog) LatestAlarmStart() (time.Time, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var latest time.Time
	for _, start := range l.latestStart {
		if start.After(latest) {
			latest = start
		}
	}
	return latest, !latest.IsZero()
}

// ListAlarms reads the alarms that started between since and until, oldest first.
func (c *Client) ListAlarms(ctx context.Context, since, until time.Time) ([]alarm, error) {
	// Read a little past until, so an alarm that starts before until has its end.
	entries, err := c.readAlarmLog(ctx, since, until.Add(alarmLogOverlap))
	if err != nil {
		return nil, err
	}
	var alarms []alarm
	for _, a := range pairAlarms(entries) {
		if a.Start.Before(until) {
			alarms = append(alarms, *a)
		}
	}
	return alarms, nil
}
