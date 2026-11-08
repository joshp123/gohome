package roborock

import (
	"context"
	"net/http"
	"strconv"
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
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			for _, dev := range devices {
				if dev.Name == deviceName {
					deviceID = dev.ID
					break
				}
			}
			if deviceID == "" {
				http.Error(w, "device not found", http.StatusNotFound)
				return
			}
		}

		labelMode := labelModeFromQuery(r.URL.Query().Get("labels"))
		withTrace := boolFromQueryDefault(r.URL.Query().Get("path"), true)
		if r.URL.Query().Get("trace") != "" {
			withTrace = boolFromQueryDefault(r.URL.Query().Get("trace"), withTrace)
		}
		var png []byte
		var err error
		if recordID := r.URL.Query().Get("record_id"); recordID != "" {
			startTime, parseErr := strconv.ParseInt(recordID, 10, 64)
			if parseErr != nil || startTime <= 0 {
				http.Error(w, "invalid record_id", http.StatusBadRequest)
				return
			}
			if deviceID == "" {
				devices, discoveryErr := p.client.Devices(ctx)
				if discoveryErr != nil || len(devices) != 1 {
					http.Error(w, "select a device", http.StatusBadRequest)
					return
				}
				deviceID = devices[0].ID
			}
			var raw []byte
			raw, err = p.client.RecordMap(ctx, deviceID, startTime)
			if err == nil {
				var parsed vacuumMap
				parsed, err = parseVacuumMap(raw, p.client.cfg.SegmentNames)
				if err == nil {
					png, err = renderVacuumMap(parsed, labelMode, withTrace)
				}
			}
		} else {
			png, err = p.client.MapPNG(ctx, deviceID, labelMode, withTrace)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(png)
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

func boolFromQueryDefault(raw string, fallback bool) bool {
	if raw == "" {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
