package roborock

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/joshp123/gohome/internal/core"
)

const mapEndpoint = "/roborock/map.png"

var _ core.HTTPRegistrant = (*Plugin)(nil)

func (p Plugin) RegisterHTTP(mux *http.ServeMux) {
	mux.HandleFunc(mapEndpoint, func(w http.ResponseWriter, r *http.Request) {
		if p.client == nil {
			http.Error(w, "roborock unavailable", http.StatusServiceUnavailable)
			return
		}
		deviceID := r.URL.Query().Get("device_id")
		deviceName := r.URL.Query().Get("device_name")
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()

		if deviceID == "" && deviceName != "" {
			devices, err := p.client.Devices(ctx)
			if err == nil {
				for _, dev := range devices {
					if dev.Name == deviceName {
						deviceID = dev.ID
						break
					}
				}
			}
		}

		labelMode := labelModeFromQuery(r.URL.Query().Get("labels"))
		var img mapImage
		var err error
		if labelMode == "" {
			img, err = p.client.MapSnapshot(ctx, deviceID)
		} else {
			img, err = p.client.MapSnapshotWithLabels(ctx, deviceID, labelMode)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(img.png)
	})
}

func labelModeFromQuery(raw string) string {
	if raw == "" {
		return ""
	}
	for _, part := range strings.Split(raw, ",") {
		switch strings.TrimSpace(part) {
		case "names":
			return "names"
		case "segments":
			return "segments"
		}
	}
	return ""
}
