package roborock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	roborockv1 "github.com/joshp123/gohome/proto/gen/plugins/roborock/v1"
	"github.com/prometheus/client_golang/prometheus"
)

func TestLiveAPIReadOnly(t *testing.T) {
	bootstrap, host := os.Getenv("ROBOROCK_TEST_BOOTSTRAP"), os.Getenv("ROBOROCK_TEST_HOST")
	if bootstrap == "" || host == "" {
		t.Skip("set live read-only credentials")
	}
	client, err := NewClient(Config{BootstrapFile: bootstrap, IPOverrides: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	devices, err := client.Devices(ctx)
	if err != nil || len(devices) != 1 {
		t.Fatalf("discovery: %v", err)
	}
	device := devices[0]
	client.overrides[device.ID] = host
	defer func() {
		for _, ch := range client.channels {
			ch.Close()
		}
		if client.mqtt != nil {
			client.mqtt.client.Disconnect(0)
		}
	}()
	state, err := client.Status(ctx, device.ID)
	if err != nil {
		t.Fatal(err)
	}
	registry := prometheus.NewRegistry()
	registry.MustRegister(NewMetricsCollector(client))
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, family := range families {
		for _, metric := range family.Metric {
			if strings.Contains(family.GetName(), "cleaning_run_") {
				if metric.GetTimestampMs() != *state.LastRecord.End*1000 {
					t.Fatal("history timestamp is scrape time")
				}
				count++
			}
		}
	}
	if count != len(cleaningRecordMetrics(state.LastRecord)) {
		t.Fatalf("missing record fields: %d", count)
	}
	t.Logf("live scrape: %d metric families; %d run fields carry the device's end timestamp", len(families), count)
	records, err := client.CleaningHistory(ctx, device.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 {
		t.Fatal("no history")
	}
	t.Logf("retained history: %d records", len(records))
	svc := &service{client: client}
	maps, err := svc.ListSavedMaps(ctx, &roborockv1.DeviceStatusRequest{DeviceId: device.ID})
	if err != nil {
		t.Fatal(err)
	}
	details, err := svc.GetMapDetails(ctx, &roborockv1.MapDetailsRequest{DeviceId: device.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d saved maps; %d room mappings; %d obstacle markers; %d raw map bytes", len(maps.Maps), len(details.Rooms), len(details.Obstacles), len(details.RawMap))
	for _, o := range details.Obstacles {
		t.Logf("obstacle type=%d photo_id_present=%v", o.GetTypeCode(), o.PhotoId != "")
	}
	mux := http.NewServeMux()
	Plugin{client: client}.RegisterHTTP(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", fmt.Sprintf("/roborock/map.png?record_id=%d", *records[0].Begin), nil))
	if response.Code != 200 {
		t.Fatalf("historical map HTTP %d: %s", response.Code, response.Body.String())
	}
	image, err := png.Decode(bytes.NewReader(response.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("historical map HTTP 200 PNG %dx%d", image.Bounds().Dx(), image.Bounds().Dy())
	if path := os.Getenv("ROBOROCK_AUDIT_MAP"); path != "" {
		if err := os.WriteFile(path, response.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if path := os.Getenv("ROBOROCK_HISTORY_EXPORT"); path != "" {
		var lines strings.Builder
		for _, record := range records {
			for _, r := range cleaningRecordMetrics(&record) {
				if r.value == nil {
					continue
				}
				line := map[string]any{"metric": map[string]string{"__name__": r.name, "device_id": device.ID, "device_name": device.Name, "model": device.Model, "job": "gohome", "instance": "gohome", "source": "roborock"}, "values": []float64{float64(*r.value) * r.scale}, "timestamps": []int64{*record.End * 1000}}
				raw, err := json.Marshal(line)
				if err != nil {
					t.Fatal(err)
				}
				lines.Write(raw)
				lines.WriteByte('\n')
			}
		}
		if err := os.WriteFile(path, []byte(lines.String()), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Compare every newly collected status value with a separate raw device read.
	wireDevice, err := client.deviceByID(device.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := client.readRPC(ctx, wireDevice, "get_status", nil)
	if err != nil {
		t.Fatal(err)
	}
	rawStatus, ok := normalizeMap(raw)
	if !ok {
		t.Fatal("invalid status")
	}
	for _, family := range families {
		if family.GetName() == "gohome_roborock_bumper_status_code" {
			values := rawStatus["adbumper_status"].([]any)
			if len(family.Metric) != len(values) {
				t.Fatal("missing front sensor")
			}
			for _, metric := range family.Metric {
				for _, label := range metric.Label {
					if label.GetName() == "sensor_index" {
						index, err := strconv.Atoi(label.GetValue())
						if err != nil || index >= len(values) {
							t.Fatal("invalid sensor index")
						}
						if metric.Gauge.GetValue() != float64(intFrom(values[index])) {
							t.Fatal("front sensor differs from device")
						}
					}
				}
			}
			t.Logf("%d front sensor states match the raw response", len(values))
		}
	}

	verified := 0
	for _, family := range families {
		if key, ok := auditStatusWireFields[family.GetName()]; ok {
			value, present := rawStatus[key]
			if !present {
				t.Fatalf("exported absent %s", key)
			}
			if len(family.Metric) != 1 || family.Metric[0].Gauge.GetValue() != float64(intFrom(value)) {
				t.Fatalf("metric %s differs from device", family.GetName())
			}
			verified++
		}
	}
	settingsChecks := []struct{ method, field, metric string }{
		{"get_network_info", "rssi", "wifi_signal_dbm"},
		{"get_dust_collection_mode", "mode", "dust_collection_mode_code"},
		{"get_wash_towel_mode", "wash_mode", "mop_wash_mode_code"},
		{"get_smart_wash_params", "smart_wash", "smart_wash_enabled"},
		{"get_smart_wash_params", "wash_interval", "mop_wash_interval_seconds"},
		{"app_get_dryer_setting", "status", "dryer_enabled"},
		{"app_get_dryer_setting", "on.dry_time", "dryer_duration_seconds"},
		{"get_carpet_mode", "enable", "carpet_boost_enabled"},
		{"get_carpet_clean_mode", "carpet_clean_mode", "carpet_mode_code"},
	}
	for _, check := range settingsChecks {
		raw, err := client.readRPC(ctx, wireDevice, check.method, nil)
		if err != nil {
			t.Fatal(err)
		}
		values, ok := normalizeMap(raw)
		if !ok {
			t.Fatal("invalid operational settings")
		}
		var value any
		if check.field == "on.dry_time" {
			value = values["on"].(map[string]any)["dry_time"]
		} else {
			value = values[check.field]
		}
		found := false
		for _, family := range families {
			if family.GetName() == "gohome_roborock_"+check.metric {
				found = true
				if check.metric == "wifi_signal_dbm" {
					continue
				} // Signal strength can change between reads.
				if family.Metric[0].Gauge.GetValue() != float64(intFrom(value)) {
					t.Fatalf("setting %s differs", check.metric)
				}
			}
		}
		if !found {
			t.Fatalf("missing setting metric %s", check.metric)
		}
	}
	t.Log("9 operational settings exported; stable settings match separate reads")

	t.Logf("%d status measurements match a separate raw read", verified)
}

// Keep the wire-to-metric audit explicit so units and field selection are reviewable.
var auditStatusWireFields = map[string]string{
	"gohome_roborock_dock_error_code":              "dock_error_status",
	"gohome_roborock_dock_status_bits":             "dss",
	"gohome_roborock_robot_status_bits":            "rss",
	"gohome_roborock_common_status_bits":           "common_status",
	"gohome_roborock_charge_status_code":           "charge_status",
	"gohome_roborock_dry_status_code":              "dry_status",
	"gohome_roborock_dry_remaining_seconds":        "rdt",
	"gohome_roborock_cleaning_progress_percent":    "clean_percent",
	"gohome_roborock_obstacles_avoided":            "avoid_count",
	"gohome_roborock_dust_collection_status_code":  "dust_collection_status",
	"gohome_roborock_auto_dust_collection_enabled": "auto_dust_collection",
	"gohome_roborock_wash_phase_code":              "wash_phase",
	"gohome_roborock_wash_ready":                   "wash_ready",
	"gohome_roborock_wash_status_code":             "wash_status",
	"gohome_roborock_return_type_code":             "back_type",
	"gohome_roborock_cleaning_mode_code":           "in_cleaning",
	"gohome_roborock_returning":                    "in_returning",
	"gohome_roborock_warming_up":                   "in_warmup",
	"gohome_roborock_locating":                     "is_locating",
	"gohome_roborock_exploring":                    "is_exploring",
	"gohome_roborock_dnd_enabled":                  "dnd_enabled",
	"gohome_roborock_child_lock_enabled":           "lock_status",
	"gohome_roborock_map_present":                  "map_present",
	"gohome_roborock_map_status_code":              "map_status",
	"gohome_roborock_unsaved_map":                  "unsave_map_flag",
	"gohome_roborock_unsaved_map_reason_code":      "unsave_map_reason",
	"gohome_roborock_collision_avoidance_enabled":  "collision_avoid_status",
	"gohome_roborock_mop_forbidden_enabled":        "mop_forbidden_enable",
	"gohome_roborock_replenish_mode_code":          "replenish_mode",
	"gohome_roborock_corner_clean_mode_code":       "corner_clean_mode",
	"gohome_roborock_cleaning_repeats":             "repeat",
}
