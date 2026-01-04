package roborock

import (
	"context"
	"net/http"
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

		img, err := p.client.MapSnapshot(ctx, deviceID)
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
