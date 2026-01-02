package tado

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/joshp123/gohome/internal/oauth"
	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
)

// Client talks to the Tado REST API.
type Client struct {
	baseURL string
	oauth   *oauth.Manager

	httpClient *http.Client
	homeID     *int
}

type HTTPStatusError struct {
	Status int
	Body   string
}

func (e HTTPStatusError) Error() string {
	return fmt.Sprintf("tado api error %d: %s", e.Status, strings.TrimSpace(e.Body))
}

func NewClient(cfg Config, decl oauth.Declaration, oauthCfg *configv1.OAuthConfig) (*Client, error) {
	blobStore, err := oauth.NewS3Store(oauthCfg)
	if err != nil {
		return nil, err
	}
	return NewClientWithStore(cfg, decl, blobStore)
}

func NewClientWithStore(cfg Config, decl oauth.Declaration, blobStore oauth.BlobStore) (*Client, error) {
	if blobStore == nil {
		return nil, fmt.Errorf("blob store is required")
	}

	manager, err := oauth.NewManager(decl, cfg.BootstrapFile, blobStore)
	if err != nil {
		return nil, err
	}
	manager.Start(context.Background())

	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	return &Client{
		baseURL:    baseURL,
		oauth:      manager,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		homeID:     cfg.HomeID,
	}, nil
}

func (c *Client) HomeID(ctx context.Context) (int, error) {
	if c.homeID != nil {
		return *c.homeID, nil
	}

	var resp struct {
		Homes []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"homes"`
	}

	if err := c.getJSON(ctx, "/me", &resp); err != nil {
		return 0, err
	}
	if len(resp.Homes) == 0 {
		return 0, fmt.Errorf("no homes found in /me response")
	}
	if len(resp.Homes) > 1 {
		labels := make([]string, 0, len(resp.Homes))
		for _, home := range resp.Homes {
			if home.Name != "" {
				labels = append(labels, fmt.Sprintf("%d (%s)", home.ID, home.Name))
				continue
			}
			labels = append(labels, fmt.Sprintf("%d", home.ID))
		}
		return 0, fmt.Errorf("multiple homes found: %s (set home_id override)", strings.Join(labels, ", "))
	}

	c.homeID = &resp.Homes[0].ID
	return *c.homeID, nil
}

func (c *Client) Zones(ctx context.Context) ([]Zone, error) {
	homeID, err := c.HomeID(ctx)
	if err != nil {
		return nil, err
	}

	var resp []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	if err := c.getJSON(ctx, fmt.Sprintf("/homes/%d/zones", homeID), &resp); err != nil {
		return nil, err
	}

	zones := make([]Zone, 0, len(resp))
	for _, zone := range resp {
		zones = append(zones, Zone{ID: zone.ID, Name: zone.Name})
	}
	return zones, nil
}

func (c *Client) ZoneStates(ctx context.Context) (map[int]ZoneState, error) {
	homeID, err := c.HomeID(ctx)
	if err != nil {
		return nil, err
	}

	var resp struct {
		ZoneStates map[string]struct {
			SensorDataPoints struct {
				InsideTemperature struct {
					Celsius   *float64 `json:"celsius"`
					Timestamp string   `json:"timestamp"`
				} `json:"insideTemperature"`
				Humidity struct {
					Percentage *float64 `json:"percentage"`
					Timestamp  string   `json:"timestamp"`
				} `json:"humidity"`
			} `json:"sensorDataPoints"`
			ActivityDataPoints struct {
				HeatingPower struct {
					Percentage *float64 `json:"percentage"`
				} `json:"heatingPower"`
			} `json:"activityDataPoints"`
			Setting struct {
				Power       string `json:"power"`
				Temperature struct {
					Celsius *float64 `json:"celsius"`
				} `json:"temperature"`
			} `json:"setting"`
			OverlayType *string `json:"overlayType"`
		} `json:"zoneStates"`
	}

	if err := c.getJSON(ctx, fmt.Sprintf("/homes/%d/zoneStates", homeID), &resp); err != nil {
		return nil, err
	}

	states := make(map[int]ZoneState, len(resp.ZoneStates))
	for zoneID, state := range resp.ZoneStates {
		id, err := strconv.Atoi(zoneID)
		if err != nil {
			continue
		}
		zoneState := ZoneState{
			InsideTemperatureCelsius:   state.SensorDataPoints.InsideTemperature.Celsius,
			InsideTemperatureTimestamp: parseTimestamp(state.SensorDataPoints.InsideTemperature.Timestamp),
			HumidityPercent:            state.SensorDataPoints.Humidity.Percentage,
			HumidityTimestamp:          parseTimestamp(state.SensorDataPoints.Humidity.Timestamp),
			SetpointCelsius:            state.Setting.Temperature.Celsius,
			HeatingPowerPercent:        state.ActivityDataPoints.HeatingPower.Percentage,
		}
		if state.Setting.Power != "" {
			powerOn := strings.EqualFold(state.Setting.Power, "ON")
			zoneState.PowerOn = &powerOn
		}
		if state.OverlayType != nil {
			active := strings.TrimSpace(*state.OverlayType) != ""
			zoneState.OverrideActive = &active
		}
		states[id] = zoneState
	}
	return states, nil
}

func (c *Client) Weather(ctx context.Context) (Weather, error) {
	homeID, err := c.HomeID(ctx)
	if err != nil {
		return Weather{}, err
	}

	var resp struct {
		SolarIntensity struct {
			Percentage *float64 `json:"percentage"`
			Timestamp  string   `json:"timestamp"`
		} `json:"solarIntensity"`
		OutsideTemperature struct {
			Celsius   *float64 `json:"celsius"`
			Timestamp string   `json:"timestamp"`
		} `json:"outsideTemperature"`
	}

	if err := c.getJSON(ctx, fmt.Sprintf("/homes/%d/weather", homeID), &resp); err != nil {
		return Weather{}, err
	}

	return Weather{
		OutsideTemperatureCelsius:   resp.OutsideTemperature.Celsius,
		OutsideTemperatureTimestamp: parseTimestamp(resp.OutsideTemperature.Timestamp),
		SolarIntensityPercent:       resp.SolarIntensity.Percentage,
		SolarIntensityTimestamp:     parseTimestamp(resp.SolarIntensity.Timestamp),
	}, nil
}

func (c *Client) SetZoneTemperature(ctx context.Context, zoneID int, temperatureC float64) error {
	homeID, err := c.HomeID(ctx)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"setting": map[string]any{
			"type":  "HEATING",
			"power": "ON",
			"temperature": map[string]any{
				"celsius": temperatureC,
			},
		},
		"termination": map[string]any{
			"type": "MANUAL",
		},
	}

	return c.putJSON(ctx, fmt.Sprintf("/homes/%d/zones/%d/overlay", homeID, zoneID), payload)
}

func (c *Client) DayReport(ctx context.Context, homeID, zoneID int, day time.Time) (dayReport, error) {
	path := fmt.Sprintf("/homes/%d/zones/%d/dayReport?date=%s", homeID, zoneID, day.Format("2006-01-02"))
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return dayReport{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return dayReport{}, ErrDayReportNotFound
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return dayReport{}, HTTPStatusError{Status: resp.StatusCode, Body: string(body)}
	}

	var report dayReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return dayReport{}, err
	}
	return report, nil
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("tado api error %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) putJSON(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := c.doRequest(ctx, http.MethodPut, path, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("tado api error %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	return nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	accessToken, err := c.oauth.AccessToken(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	resp.Body.Close()
	c.oauth.TriggerRefresh(ctx)
	return nil, fmt.Errorf("tado api unauthorized; refresh triggered")
}

func parseTimestamp(value string) *time.Time {
	if value == "" {
		return nil
	}
	if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return &ts
	}
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return &ts
	}
	return nil
}
