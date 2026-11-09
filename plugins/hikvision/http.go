package hikvision

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/joshp123/gohome/internal/core"
)

const snapshotEndpoint = "/hikvision/snapshot.jpg"

// Pictures from the camera's own recording: at a moment given as ?at=RFC 3339,
// or at the start of the latest alarm.
const frameEndpoint = "/hikvision/frame.jpg"
const alarmFrameEndpoint = "/hikvision/alarm.jpg"
const snapshotCacheDuration = 5 * time.Second

var _ core.HTTPRegistrant = Plugin{}

// RegisterHTTP serves on-demand pictures; no picture is written to disk.
func (p Plugin) RegisterHTTP(mux *http.ServeMux) {
	mux.HandleFunc(frameEndpoint, func(w http.ResponseWriter, r *http.Request) {
		if !readRequest(w, r) {
			return
		}
		at, err := time.Parse(time.RFC3339Nano, r.URL.Query().Get("at"))
		if err != nil {
			http.Error(w, "at must be an RFC 3339 time", http.StatusBadRequest)
			return
		}
		p.serveRecordedFrame(w, r, at, "")
	})
	mux.HandleFunc(alarmFrameEndpoint, func(w http.ResponseWriter, r *http.Request) {
		if !readRequest(w, r) {
			return
		}
		if p.alarms == nil {
			http.Error(w, "hikvision unavailable", http.StatusServiceUnavailable)
			return
		}
		start, ok := p.alarms.LatestAlarmStart()
		if !ok {
			http.Error(w, "no alarm known yet", http.StatusNotFound)
			return
		}
		p.serveRecordedFrame(w, r, start, start.UTC().Format(time.RFC3339))
	})
	var mu sync.Mutex
	var jpeg []byte
	var capturedAt time.Time
	mux.HandleFunc(snapshotEndpoint, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if p.client == nil {
			http.Error(w, "hikvision unavailable", http.StatusServiceUnavailable)
			return
		}
		// Serialize refreshes so simultaneous dashboard loads share one read.
		mu.Lock()
		if time.Since(capturedAt) >= snapshotCacheDuration {
			jpeg = nil
			capturedAt = time.Time{}
			ctx, cancel := context.WithTimeout(r.Context(), readTimeout)
			picture, err := p.client.Snapshot(ctx)
			cancel()
			if err != nil {
				mu.Unlock()
				slog.Warn("hikvision snapshot failed", "error", err)
				http.Error(w, "camera snapshot unavailable", http.StatusBadGateway)
				return
			}
			jpeg = picture
			capturedAt = time.Now().UTC()
			expiresPicture := capturedAt
			time.AfterFunc(snapshotCacheDuration, func() {
				mu.Lock()
				defer mu.Unlock()
				if capturedAt.Equal(expiresPicture) {
					jpeg = nil
					capturedAt = time.Time{}
				}
			})
		}
		picture, taken := jpeg, capturedAt
		mu.Unlock()
		w.Header().Set("Content-Type", "image/jpeg")
		// ISAPI supplies no capture timestamp: this is when the read completed,
		// matching GetSnapshot's captured_at, not the camera's clock.
		w.Header().Set("X-Snapshot-Captured-At", taken.Format(time.RFC3339Nano))
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(picture)
		}
	})
}

// readRequest sets the no-store header and accepts only GET and HEAD.
func readRequest(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// serveRecordedFrame answers with the recording's picture at the second at.
func (p Plugin) serveRecordedFrame(w http.ResponseWriter, r *http.Request, at time.Time, alarmStartedAt string) {
	if p.frames == nil {
		http.Error(w, "hikvision unavailable", http.StatusServiceUnavailable)
		return
	}
	picture, second, err := p.frames.Frame(r.Context(), at)
	switch {
	case errors.Is(err, errNoRecording):
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	case errors.Is(err, errNoFFmpeg):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	case err != nil:
		slog.Warn("hikvision recorded picture failed", "error", err)
		http.Error(w, "recorded picture unavailable", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("X-Frame-At", second.Format(time.RFC3339))
	if alarmStartedAt != "" {
		w.Header().Set("X-Alarm-Started-At", alarmStartedAt)
	}
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(picture)
	}
}
