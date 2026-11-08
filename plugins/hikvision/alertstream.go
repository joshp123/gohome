package hikvision

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"time"

	hikvisionv1 "github.com/joshp123/gohome/proto/gen/plugins/hikvision/v1"
)

const (
	// The camera posts a keep-alive every ten seconds; six missed means the connection is dead.
	alertStreamIdleTimeout = time.Minute
	alertStreamMaxBackoff  = time.Minute
	maxPartBytes           = 8 << 20 // event XML is under 1 KiB; pictures can be larger
	subscriberBuffer       = 256
)

// AlertStreamStatus is the state of the connection to the camera's event stream.
type AlertStreamStatus struct {
	Connected      bool
	ConnectedSince time.Time
	LastMessage    time.Time
	LastError      string
}

// Why a subscription ended.
var (
	errStreamNotConnected = errors.New("gohome is not connected to the camera's event stream")
	errStreamLost         = errors.New("gohome lost the camera's event stream; events under way may not have ended")
	errSubscriberBehind   = errors.New("subscriber fell behind the camera's events")
)

// subscription carries events' starts and ends to one WatchEvents caller.
type subscription struct {
	events chan cameraEvent
	// ended says why events was closed; set before the close.
	ended error
}

// alertStream keeps one connection to the camera's event stream open, and
// passes each event's start and end to the metrics and to every WatchEvents
// subscriber.
type alertStream struct {
	client         *Client
	onEventStarted func(cameraEvent)
	// onEventStopped is called when an event ends, and when the stream is lost while it is under way.
	onEventStopped func(cameraEvent)
	episodes       *episodeTracker

	mu          sync.Mutex
	status      AlertStreamStatus
	subscribers map[*subscription]struct{}
}

func newAlertStream(client *Client, onEventStarted, onEventStopped func(cameraEvent)) *alertStream {
	s := &alertStream{client: client, onEventStarted: onEventStarted, onEventStopped: onEventStopped,
		subscribers: map[*subscription]struct{}{}}
	s.episodes = newEpisodeTracker(eventEndsAfterQuiet, s.publish)
	return s
}

func (s *alertStream) Status() AlertStreamStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Subscribe returns events' starts and ends, beginning with the starts of
// events already under way. It fails while the event stream is disconnected,
// because silence would then look like nothing happening. The stream ends the
// subscription if it is lost or the subscriber falls more than
// subscriberBuffer events behind.
func (s *alertStream) Subscribe() (*subscription, func(), error) {
	sub := &subscription{events: make(chan cameraEvent, subscriberBuffer)}
	var err error
	s.episodes.whileHeld(func(underWay []cameraEvent) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.status.Connected {
			err = errStreamNotConnected
			return
		}
		for _, start := range underWay {
			sub.events <- start
		}
		s.subscribers[sub] = struct{}{}
	})
	if err != nil {
		return nil, nil, err
	}
	return sub, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.endLocked(sub, nil)
	}, nil
}

// endLocked closes a subscription that is still open. s.mu must be held.
func (s *alertStream) endLocked(sub *subscription, why error) {
	if _, ok := s.subscribers[sub]; ok {
		delete(s.subscribers, sub)
		sub.ended = why
		close(sub.events)
	}
}

// lost forgets events under way and ends every subscription: once the stream
// is down nobody can see those events end.
func (s *alertStream) lost() {
	s.episodes.abandon(func(underWay []cameraEvent) {
		for _, start := range underWay {
			s.onEventStopped(start)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		for sub := range s.subscribers {
			s.endLocked(sub, errStreamLost)
		}
	})
}

// Run reconnects until ctx ends.
func (s *alertStream) Run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		started := time.Now()
		err := s.readOnce(ctx)
		s.mu.Lock()
		s.status.Connected = false
		s.status.LastError = err.Error()
		s.mu.Unlock()
		s.lost()
		if ctx.Err() != nil {
			return
		}
		if time.Since(started) > alertStreamIdleTimeout {
			backoff = time.Second
		}
		log.Printf("hikvision alert stream: %v; reconnecting in %s", err, backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, alertStreamMaxBackoff)
	}
}

// readOnce reads one connection until it fails, and always returns why.
func (s *alertStream) readOnce(ctx context.Context) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	resp, err := s.client.OpenAlertStream(ctx)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	mediaType, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") || params["boundary"] == "" {
		return fmt.Errorf("alert stream: expected a multipart response, got %q", resp.Header.Get("Content-Type"))
	}

	now := time.Now()
	s.mu.Lock()
	s.status.Connected, s.status.ConnectedSince, s.status.LastError = true, now, ""
	s.mu.Unlock()

	errIdle := fmt.Errorf("alert stream: no message from the camera for %s", alertStreamIdleTimeout)
	idle := time.AfterFunc(alertStreamIdleTimeout, func() { cancel(errIdle) })
	defer idle.Stop()

	parts := newPartReader(resp.Body, params["boundary"])
	for {
		header, body, err := parts.next()
		if err != nil {
			if cause := context.Cause(ctx); cause != nil {
				return cause
			}
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("alert stream: camera closed the connection")
			}
			return fmt.Errorf("alert stream: %w", err)
		}
		idle.Reset(alertStreamIdleTimeout)
		if err := s.readPart(header, body); err != nil {
			log.Printf("hikvision alert stream: skipped a part: %v", err)
		}
	}
}

func (s *alertStream) readPart(header textproto.MIMEHeader, body []byte) error {
	receivedAt := time.Now()
	s.mu.Lock()
	s.status.LastMessage = receivedAt
	s.mu.Unlock()

	// Smart events can carry pictures as further parts; this plugin reads only the event.
	partType, _, _ := mime.ParseMediaType(header.Get("Content-Type"))
	if partType != "application/xml" && partType != "text/xml" {
		return nil
	}
	event, err := parseEventNotification(body, receivedAt)
	if err != nil {
		return err
	}
	if !event.isKeepAlive() {
		s.episodes.notify(event)
	}
	return nil
}

// partReader reads the camera's multipart stream. The camera ends some parts
// with the closing delimiter ("--boundary--") and then carries on with the
// next part, so mime/multipart, which stops at that delimiter and reads
// ahead past it, cannot be used. Only an opening delimiter starts a part.
type partReader struct {
	r                  *bufio.Reader
	open, close        string
	atOpeningDelimiter bool
}

func newPartReader(r io.Reader, boundary string) *partReader {
	return &partReader{r: bufio.NewReaderSize(r, 64<<10), open: "--" + boundary, close: "--" + boundary + "--"}
}

func (p *partReader) next() (textproto.MIMEHeader, []byte, error) {
	for !p.atOpeningDelimiter {
		line, err := p.r.ReadString('\n')
		if err != nil {
			return nil, nil, err
		}
		p.atOpeningDelimiter = strings.TrimSpace(line) == p.open
	}
	p.atOpeningDelimiter = false
	header, err := textproto.NewReader(p.r).ReadMIMEHeader()
	if err != nil {
		return nil, nil, fmt.Errorf("part header: %w", err)
	}
	if length := header.Get("Content-Length"); length != "" {
		n, err := strconv.Atoi(length)
		if err != nil || n < 0 || n > maxPartBytes {
			return nil, nil, fmt.Errorf("part Content-Length %q", length)
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(p.r, body); err != nil {
			return nil, nil, err
		}
		return header, body, nil
	}
	// No length: the body runs to the next delimiter line.
	var body []byte
	for {
		line, err := p.r.ReadString('\n')
		if err != nil {
			return nil, nil, err
		}
		if trimmed := strings.TrimSpace(line); trimmed == p.open || trimmed == p.close {
			p.atOpeningDelimiter = trimmed == p.open
			return header, body, nil
		}
		if body = append(body, line...); len(body) > maxPartBytes {
			return nil, nil, fmt.Errorf("part larger than %d bytes", maxPartBytes)
		}
	}
}

// publish sends an event's start or end to the metrics and every subscriber.
func (s *alertStream) publish(event cameraEvent) {
	if event.State == hikvisionv1.EventState_EVENT_STATE_ACTIVE {
		s.onEventStarted(event)
	} else {
		s.onEventStopped(event)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for sub := range s.subscribers {
		select {
		case sub.events <- event:
		default:
			s.endLocked(sub, errSubscriberBehind)
		}
	}
}
