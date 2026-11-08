package hikvision

import (
	"strings"
	"sync"
	"time"

	hikvisionv1 "github.com/joshp123/gohome/proto/gen/plugins/hikvision/v1"
)

// eventEndsAfterQuiet is how long an event must go without a repeat before it
// has ended. While an event lasts the camera repeats an active notification
// about once a second, its detector drops out for a second or two now and
// then, and it never sends an inactive notification for motion.
const eventEndsAfterQuiet = 10 * time.Second

// episodeKey is one event as the camera names it. It uses the camera's own
// words, because the fixed vocabularies fold different events together (all
// unknown types are EVENT_KIND_OTHER), and the rule, because two line-crossing
// rules can fire at once.
type episodeKey struct {
	eventType, cameraTarget string
	channelID, ruleID       uint32
}

func episodeKeyOf(event cameraEvent) episodeKey {
	return episodeKey{strings.ToLower(event.EventType), strings.ToLower(event.CameraWords), event.ChannelID, event.RuleID}
}

type episode struct {
	start, latest cameraEvent
}

// episodeTracker turns the camera's repeated active notifications into one
// active event when an event starts and one inactive event when it ends.
//
// An event ends only when a later message shows the stream is still alive:
// silence alone could be a dead connection, and then nobody knows whether the
// event ended. The camera sends a keep-alive every ten seconds, so an end is
// emitted 10 to 20 s after the event's last notification.
type episodeTracker struct {
	quiet time.Duration
	// emit is called with the tracker's lock held, so transitions arrive in order.
	emit func(cameraEvent)

	mu   sync.Mutex
	open map[episodeKey]*episode
}

func newEpisodeTracker(quiet time.Duration, emit func(cameraEvent)) *episodeTracker {
	return &episodeTracker{quiet: quiet, emit: emit, open: map[episodeKey]*episode{}}
}

// observe takes every message from the event stream, keep-alives included, in
// the order they arrive.
func (t *episodeTracker) observe(message cameraEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for key, current := range t.open {
		if message.ReceivedAt.Sub(current.latest.ReceivedAt) >= t.quiet {
			delete(t.open, key)
			ended := current.latest
			ended.State = hikvisionv1.EventState_EVENT_STATE_INACTIVE
			t.emit(ended)
		}
	}
	if message.isKeepAlive() || message.State != hikvisionv1.EventState_EVENT_STATE_ACTIVE {
		return
	}
	key := episodeKeyOf(message)
	if current, ok := t.open[key]; ok {
		current.latest = message
		return
	}
	t.open[key] = &episode{start: message, latest: message}
	t.emit(message)
}

// abandon forgets every event under way without saying it ended: with the
// event stream lost, nobody can tell. fn runs with the starts of those events
// before any new event is emitted.
func (t *episodeTracker) abandon(fn func(underWay []cameraEvent)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fn(t.startsLocked())
	clear(t.open)
}

// whileHeld runs fn with the starts of events still under way. No start or end
// is emitted until fn returns.
func (t *episodeTracker) whileHeld(fn func(underWay []cameraEvent)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fn(t.startsLocked())
}

func (t *episodeTracker) startsLocked() []cameraEvent {
	starts := make([]cameraEvent, 0, len(t.open))
	for _, current := range t.open {
		starts = append(starts, current.start)
	}
	return starts
}
