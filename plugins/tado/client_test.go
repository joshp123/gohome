package tado

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientFlow(t *testing.T) {
	var tokenRequests int
	var overlayBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			tokenRequests++
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST to /token, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"test-token","expires_in":3600}`)
			return
		case "/me":
			assertAuth(t, r)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"homes":[{"id":1}]}`)
			return
		case "/homes/1/zones":
			assertAuth(t, r)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[{"id":2,"name":"Living"}]`)
			return
		case "/homes/1/zoneStates":
			assertAuth(t, r)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"zoneStates":{"2":{"sensorDataPoints":{"insideTemperature":{"celsius":21.5},"humidity":{"percentage":40.2}}}}}`)
			return
		case "/homes/1/zones/2/overlay":
			assertAuth(t, r)
			body, _ := io.ReadAll(r.Body)
			overlayBody = string(body)
			w.WriteHeader(http.StatusOK)
			return
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := Config{
		BaseURL:      server.URL,
		AuthURL:      server.URL + "/token",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RefreshToken: "refresh-token",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx := context.Background()

	homeID, err := client.HomeID(ctx)
	if err != nil {
		t.Fatalf("HomeID: %v", err)
	}
	if homeID != 1 {
		t.Fatalf("expected homeID 1, got %d", homeID)
	}

	zones, err := client.Zones(ctx)
	if err != nil {
		t.Fatalf("Zones: %v", err)
	}
	if len(zones) != 1 || zones[0].ID != 2 {
		t.Fatalf("unexpected zones: %+v", zones)
	}

	states, err := client.ZoneStates(ctx)
	if err != nil {
		t.Fatalf("ZoneStates: %v", err)
	}
	state, ok := states[2]
	if !ok {
		t.Fatalf("expected state for zone 2")
	}
	if state.InsideTemperatureCelsius != 21.5 {
		t.Fatalf("unexpected temperature: %v", state.InsideTemperatureCelsius)
	}
	if state.HumidityPercent != 40.2 {
		t.Fatalf("unexpected humidity: %v", state.HumidityPercent)
	}

	if err := client.SetZoneTemperature(ctx, 2, 20.0); err != nil {
		t.Fatalf("SetZoneTemperature: %v", err)
	}
	if overlayBody == "" || !strings.Contains(overlayBody, "\"celsius\":20") {
		t.Fatalf("unexpected overlay payload: %s", overlayBody)
	}

	if tokenRequests == 0 {
		t.Fatalf("expected token refresh request")
	}
}

func assertAuth(t *testing.T, r *http.Request) {
	t.Helper()
	auth := r.Header.Get("Authorization")
	if auth != "Bearer test-token" {
		t.Fatalf("unexpected auth header: %s", auth)
	}
}
