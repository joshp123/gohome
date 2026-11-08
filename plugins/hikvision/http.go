package hikvision

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/joshp123/gohome/internal/core"
)

const snapshotEndpoint = "/hikvision/snapshot.jpg"
const snapshotCacheDuration = 5 * time.Second

var _ core.HTTPRegistrant = Plugin{}

// RegisterHTTP serves an on-demand picture; no picture is written to disk.
func (p Plugin) RegisterHTTP(mux *http.ServeMux) {
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
