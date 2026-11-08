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

// alertStream keeps one connection to the camera's event stream open, and
// passes each event's start and end to the metrics and to every WatchEvents
// subscriber.
type alertStream struct {
	client       *Client
	onTransition func(cameraEvent)
	episodes     *episodeTracker

	mu          sync.Mutex
	status      AlertStreamStatus
	subscribers map[chan cameraEvent]struct{}
}

func newAlertStream(client *Client, onTransition func(cameraEvent)) *alertStream {
	s := &alertStream{client: client, onTransition: onTransition, subscribers: map[chan cameraEvent]struct{}{}}
	s.episodes = newEpisodeTracker(eventEndsAfterQuiet, s.publish)
	return s
}

func (s *alertStream) Status() AlertStreamStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Subscribe returns a channel of events' starts and ends, beginning with the
// starts of events already under way. The stream closes the channel if the
// subscriber falls more than subscriberBuffer events behind.
func (s *alertStream) Subscribe() (<-chan cameraEvent, func()) {
	ch := make(chan cameraEvent, subscriberBuffer)
	s.episodes.whileHeld(func(underWay []cameraEvent) {
		for _, start := range underWay {
			ch <- start
		}
		s.mu.Lock()
		s.subscribers[ch] = struct{}{}
		s.mu.Unlock()
	})
	return ch, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.subscribers[ch]; ok {
			delete(s.subscribers, ch)
			close(ch)
		}
	}
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
	s.onTransition(event)
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subscribers {
		select {
		case ch <- event:
		default:
			delete(s.subscribers, ch)
			close(ch)
		}
	}
}
