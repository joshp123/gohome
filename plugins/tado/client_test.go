package tado

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joshp123/gohome/internal/oauth"
)

type memoryBlobStore struct {
	data map[string][]byte
}

func (m *memoryBlobStore) Load(_ context.Context, provider string) ([]byte, error) {
	if m.data != nil {
		if data, ok := m.data[provider]; ok {
			return data, nil
		}
	}
	return nil, oauth.ErrBlobNotFound
}

func (m *memoryBlobStore) Save(_ context.Context, provider string, data []byte) error {
	if m.data == nil {
		m.data = make(map[string][]byte)
	}
	m.data[provider] = data
	return nil
}

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
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "refresh_token=refresh-token") {
				t.Fatalf("expected refresh_token in request, got %s", string(body))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"test-token","refresh_token":"new-refresh","expires_in":3600,"token_type":"Bearer"}`)
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
			_, _ = io.WriteString(w, `{"zoneStates":{"2":{"sensorDataPoints":{"insideTemperature":{"celsius":21.5,"timestamp":"2024-08-04T09:20:08.370Z"},"humidity":{"percentage":40.2,"timestamp":"2024-08-04T09:20:08.370Z"}},"activityDataPoints":{"heatingPower":{"percentage":12.5}},"setting":{"power":"ON","temperature":{"celsius":20.0}},"overlayType":"MANUAL"}}}`)
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

	tempDir := t.TempDir()
	bootstrapPath := filepath.Join(tempDir, "bootstrap.json")
	statePath := filepath.Join(tempDir, "state.json")

	bootstrap := oauth.State{
		SchemaVersion: oauth.SchemaVersion,
		ClientID:      "client-id",
		ClientSecret:  "client-secret",
		RefreshToken:  "refresh-token",
		Scope:         "offline_access",
	}
	if err := oauth.WriteState(bootstrapPath, bootstrap); err != nil {
		t.Fatalf("write bootstrap: %v", err)
	}

	decl := oauth.Declaration{
		Provider:  "tado",
		TokenURL:  server.URL + "/token",
		Scope:     "offline_access",
		StatePath: statePath,
	}

	cfg := Config{
		BaseURL:       server.URL,
		BootstrapFile: bootstrapPath,
	}

	client, err := NewClientWithStore(cfg, decl, &memoryBlobStore{})
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
	if state.InsideTemperatureCelsius == nil || *state.InsideTemperatureCelsius != 21.5 {
		t.Fatalf("unexpected temperature: %v", state.InsideTemperatureCelsius)
	}
	if state.HumidityPercent == nil || *state.HumidityPercent != 40.2 {
		t.Fatalf("unexpected humidity: %v", state.HumidityPercent)
	}
	if state.SetpointCelsius == nil || *state.SetpointCelsius != 20.0 {
		t.Fatalf("unexpected setpoint: %v", state.SetpointCelsius)
	}
	if state.HeatingPowerPercent == nil || *state.HeatingPowerPercent != 12.5 {
		t.Fatalf("unexpected heating power: %v", state.HeatingPowerPercent)
	}
	if state.PowerOn == nil || *state.PowerOn != true {
		t.Fatalf("unexpected power state: %v", state.PowerOn)
	}
	if state.OverrideActive == nil || *state.OverrideActive != true {
		t.Fatalf("unexpected override state: %v", state.OverrideActive)
	}
	if state.InsideTemperatureTimestamp == nil {
		t.Fatalf("expected inside temperature timestamp")
	}
	expectedTime, err := time.Parse(time.RFC3339Nano, "2024-08-04T09:20:08.370Z")
	if err != nil {
		t.Fatalf("parse expected timestamp: %v", err)
	}
	if !state.InsideTemperatureTimestamp.Equal(expectedTime) {
		t.Fatalf("unexpected timestamp: %s", state.InsideTemperatureTimestamp.UTC().Format(time.RFC3339Nano))
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
