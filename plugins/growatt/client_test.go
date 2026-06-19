package growatt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPlantLivePowerSumsV4DevicePac(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v4/new-api/queryDeviceList":
			if r.Method != http.MethodPost || r.URL.Query().Get("page") != "1" {
				t.Fatalf("bad device list request: %s %s", r.Method, r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"data":[{"deviceSn":"MIN456","deviceType":"min"},{"deviceSn":"MIN123","deviceType":"min"}]}}`))
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

	watts, lastUpdate, err := client.plantLivePower(context.Background(), "GMT+2")
	if err != nil {
		t.Fatal(err)
	}
	if watts != 3000 {
		t.Fatalf("watts = %v, want 3000", watts)
	}

	want := time.Date(2026, 6, 19, 18, 24, 41, 0, time.FixedZone("GMT+2", 2*60*60))
	if lastUpdate == nil || lastUpdate.Unix() != want.Unix() {
		t.Fatalf("lastUpdate = %v, want %s", lastUpdate, want)
	}
}

func TestResolvePlantUsesCachedConfiguredPlant(t *testing.T) {
	id := int64(42)
	client := &Client{
		plantID:    &id,
		plantCache: []Plant{{ID: id, Name: "Josh", Status: 1}},
	}

	plant, err := client.ResolvePlant(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if plant.ID != id || plant.Name != "Josh" {
		t.Fatalf("plant = %+v", plant)
	}
}
