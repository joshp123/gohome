package growatt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPlantLivePowerUsesPlantDevice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/device/list":
			if r.Method != http.MethodGet || r.URL.Query().Get("plant_id") != "42" {
				t.Fatalf("bad device list request: %s %s", r.Method, r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"error_code":0,"data":{"count":1,"devices":[{"device_sn":"MIN123","type":7}]}}`))
		case "/v4/new-api/queryLastData":
			if r.Method != http.MethodPost || r.URL.Query().Get("deviceType") != "min" {
				t.Fatalf("bad last-data request: %s %s", r.Method, r.URL.RawQuery)
			}
			switch r.URL.Query().Get("deviceSn") {
			case "MIN123":
				_, _ = w.Write([]byte(`{"code":0,"data":{"min":[{"pac":"2545","time":"2026-06-19 18:23:41"}]}}`))
			case "MIN456":
				_, _ = w.Write([]byte(`{"code":0,"data":{"min":[{"pac":"455","time":"2026-06-19 18:24:41"}]}}`))
			default:
				t.Fatalf("bad deviceSn %q", r.URL.Query().Get("deviceSn"))
			}
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL + "/", token: "test-token", http: server.Client()}

	watts, lastUpdate, err := client.plantLivePower(context.Background(), 42, "GMT+2")
	if err != nil {
		t.Fatal(err)
	}
	if watts != 2545 {
		t.Fatalf("watts = %v, want 2545", watts)
	}

	want := time.Date(2026, 6, 19, 18, 23, 41, 0, time.FixedZone("GMT+2", 2*60*60))
	if lastUpdate == nil || lastUpdate.Unix() != want.Unix() {
		t.Fatalf("lastUpdate = %v, want %s", lastUpdate, want)
	}
}

func TestResolvePlantUsesCachedConfiguredPlant(t *testing.T) {
	id := int64(42)
	client := &Client{
		plantID:    &id,
		plantCache: []Plant{{ID: id, Name: "Plant", Status: 1}},
	}

	plant, err := client.ResolvePlant(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if plant.ID != id || plant.Name != "Plant" {
		t.Fatalf("plant = %+v", plant)
	}
}
