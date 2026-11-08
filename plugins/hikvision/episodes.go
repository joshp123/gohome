package hikvision

import (
	"sync"
	"time"

	hikvisionv1 "github.com/joshp123/gohome/proto/gen/plugins/hikvision/v1"
)

// eventEndsAfterQuiet is how long an event must go without a repeat before it
// has ended. While an event lasts the camera repeats an active notification
// about once a second, its detector drops out for a second or two now and
// then, and it never sends an inactive notification for motion.
const eventEndsAfterQuiet = 10 * time.Second

type episodeKey struct {
	kind   hikvisionv1.EventKind
	target hikvisionv1.DetectionTarget
}

type episode struct {
	start, latest cameraEvent
	end           *time.Timer
}

// episodeTracker turns the camera's repeated active notifications into one
// active event when an event starts and one inactive event when it ends.
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

func (t *episodeTracker) notify(notification cameraEvent) {
	if notification.State != hikvisionv1.EventState_EVENT_STATE_ACTIVE {
		return
	}
	key := episodeKey{notification.Kind, notification.Target}
	t.mu.Lock()
	defer t.mu.Unlock()
	if current, ok := t.open[key]; ok {
		current.latest = notification
		current.end.Reset(t.quiet)
		return
	}
	started := &episode{start: notification, latest: notification}
	t.open[key] = started
	started.end = time.AfterFunc(t.quiet, func() { t.finish(key, started) })
	t.emit(notification)
}

// finish ends the episode, unless a repeat arrived after its timer fired; that
// repeat's Reset has scheduled finish again.
func (t *episodeTracker) finish(key episodeKey, ending *episode) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.open[key] != ending || time.Since(ending.latest.ReceivedAt) < t.quiet {
		return
	}
	delete(t.open, key)
	ended := ending.latest
	ended.State = hikvisionv1.EventState_EVENT_STATE_INACTIVE
	t.emit(ended)
}

// whileHeld runs fn with the starts of events still under way. No start or end
// is emitted until fn returns.
func (t *episodeTracker) whileHeld(fn func(underWay []cameraEvent)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	underWay := make([]cameraEvent, 0, len(t.open))
	for _, current := range t.open {
		underWay = append(underWay, current.start)
	}
	fn(underWay)
}
