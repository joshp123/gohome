package daikin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joshp123/gohome/internal/oauth"
	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
)

const gatewayCacheTTL = 10 * time.Minute

// RateLimits captures Daikin API rate limit headers.
type RateLimits struct {
	Minute          int
	Day             int
	RemainingMinute int
	RemainingDay    int
	RetryAfter      int
	ResetAfter      int
	LastStatusCode  int
	LastStatusText  string
}

// Client talks to the Daikin Onecta REST API.
type Client struct {
	baseURL    string
	oauth      *oauth.Manager
	httpClient *http.Client

	cloudMu        sync.Mutex
	lastPatch      time.Time
	lastGatewayAt  time.Time
	lastGatewayRaw []json.RawMessage
	cooldownUntil  time.Time
	rateLimits     RateLimits
}

func NewClient(cfg Config, decl oauth.Declaration, oauthCfg *configv1.OAuthConfig) (*Client, error) {
	blobStore, err := oauth.NewS3Store(oauthCfg)
	if err != nil {
		return nil, err
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
	}, nil
}

// Devices returns the Daikin units from the gateway devices endpoint.
func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	states, err := c.DeviceStates(ctx)
	if err != nil {
		return nil, err
	}

	devices := make([]Device, 0, len(states))
	for _, state := range states {
		devices = append(devices, state.Device)
	}

	return devices, nil
}

// DeviceStates returns parsed device state for metrics and inspection.
func (c *Client) DeviceStates(ctx context.Context) ([]DeviceState, error) {
	raw, err := c.gatewayDevicesRaw(ctx)
	if err != nil {
		return nil, err
	}

	states := make([]DeviceState, 0, len(raw))
	for _, item := range raw {
		var entry deviceStateEntry
		if err := json.Unmarshal(item, &entry); err != nil {
			continue
		}
		states = append(states, entry.toDeviceState())
	}

	return states, nil
}

// RateLimits returns the last observed rate limit headers.
func (c *Client) RateLimits() RateLimits {
	c.cloudMu.Lock()
	defer c.cloudMu.Unlock()
	return c.rateLimits
}

// DeviceStateJSON returns the raw JSON payload for a single device.
func (c *Client) DeviceStateJSON(ctx context.Context, deviceID string) (string, error) {
	raw, err := c.gatewayDevicesRaw(ctx)
	if err != nil {
		return "", err
	}

	for _, item := range raw {
		var entry deviceStateEntry
		if err := json.Unmarshal(item, &entry); err != nil {
			continue
		}
		if entry.ID == deviceID {
			return string(item), nil
		}
	}

	return "", fmt.Errorf("device %q not found", deviceID)
}

// SetOnOff updates onOffMode for the given management point.
func (c *Client) SetOnOff(ctx context.Context, deviceID, embeddedID, onOffMode string) error {
	return c.patchCharacteristic(ctx, deviceID, embeddedID, "onOffMode", "", onOffMode)
}

// SetOperationMode updates operationMode for the given management point.
func (c *Client) SetOperationMode(ctx context.Context, deviceID, embeddedID, mode string) error {
	return c.patchCharacteristic(ctx, deviceID, embeddedID, "operationMode", "", mode)
}

// SetTemperature updates the setpoint for the given operation mode and setpoint key.
func (c *Client) SetTemperature(ctx context.Context, deviceID, embeddedID, operationMode, setpoint string, temperature float64) error {
	path := fmt.Sprintf("/operationModes/%s/setpoints/%s", operationMode, setpoint)
	return c.patchCharacteristic(ctx, deviceID, embeddedID, "temperatureControl", path, temperature)
}

func (c *Client) resolveClimateControlID(ctx context.Context, deviceID string) (string, error) {
	devices, err := c.Devices(ctx)
	if err != nil {
		return "", err
	}
	for _, device := range devices {
		if device.ID == deviceID {
			if device.ClimateControlID == "" {
				return "", fmt.Errorf("device %q has no climateControl management point", deviceID)
			}
			return device.ClimateControlID, nil
		}
	}
	return "", fmt.Errorf("device %q not found", deviceID)
}

func (c *Client) gatewayDevicesRaw(ctx context.Context) ([]json.RawMessage, error) {
	accessToken, err := c.oauth.AccessToken(ctx)
	if err != nil {
		return nil, err
	}

	c.cloudMu.Lock()
	if time.Since(c.lastPatch) < 10*time.Second && len(c.lastGatewayRaw) > 0 {
		cached := cloneRawMessages(c.lastGatewayRaw)
		c.cloudMu.Unlock()
		return cached, nil
	}
	if time.Now().Before(c.cooldownUntil) {
		if len(c.lastGatewayRaw) > 0 {
			cached := cloneRawMessages(c.lastGatewayRaw)
			c.cloudMu.Unlock()
			return cached, nil
		}
		until := c.cooldownUntil
		c.cloudMu.Unlock()
		return nil, fmt.Errorf("daikin api rate limited until %s", until.UTC().Format(time.RFC3339))
	}
	if time.Since(c.lastGatewayAt) < gatewayCacheTTL && len(c.lastGatewayRaw) > 0 {
		cached := cloneRawMessages(c.lastGatewayRaw)
		c.cloudMu.Unlock()
		return cached, nil
	}

	resp, err := c.doRequestWithToken(ctx, http.MethodGet, "/v1/gateway-devices", nil, accessToken)
	if err != nil {
		c.cloudMu.Unlock()
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		c.cloudMu.Unlock()
		c.oauth.TriggerRefresh(ctx)
		return nil, fmt.Errorf("daikin api unauthorized; refresh triggered")
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		c.cloudMu.Unlock()
		return nil, fmt.Errorf("read devices: %w", err)
	}

	if resp.StatusCode >= 300 {
		c.cloudMu.Unlock()
		return nil, fmt.Errorf("daikin api error %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out []json.RawMessage
	if err := json.Unmarshal(body, &out); err != nil {
		c.cloudMu.Unlock()
		return nil, fmt.Errorf("decode devices: %w", err)
	}
	c.lastGatewayRaw = out
	c.lastGatewayAt = time.Now()
	cached := cloneRawMessages(out)
	c.cloudMu.Unlock()
	return cached, nil
}

func (c *Client) patchCharacteristic(ctx context.Context, deviceID, embeddedID, dataPoint, dataPointPath string, value any) error {
	payload := map[string]any{"value": value}
	if dataPointPath != "" {
		payload["path"] = dataPointPath
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/v1/gateway-devices/%s/management-points/%s/characteristics/%s", deviceID, embeddedID, dataPoint)
	resp, err := c.doRequest(ctx, http.MethodPatch, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		c.cloudMu.Lock()
		c.lastPatch = time.Now()
		c.cloudMu.Unlock()
		return nil
	}

	data, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("daikin api error %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
}

func (c *Client) doRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	accessToken, err := c.oauth.AccessToken(ctx)
	if err != nil {
		return nil, err
	}

	c.cloudMu.Lock()
	defer c.cloudMu.Unlock()

	resp, err := c.doRequestWithToken(ctx, method, path, body, accessToken)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	resp.Body.Close()
	c.oauth.TriggerRefresh(ctx)
	return nil, fmt.Errorf("daikin api unauthorized; refresh triggered")
}

func (c *Client) doRequestWithToken(ctx context.Context, method, path string, body []byte, accessToken string) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	c.updateRateLimits(resp)
	return resp, nil
}

func (c *Client) updateRateLimits(resp *http.Response) {
	c.rateLimits = RateLimits{
		Minute:          headerInt(resp, "X-RateLimit-Limit-minute"),
		Day:             headerInt(resp, "X-RateLimit-Limit-day"),
		RemainingMinute: headerInt(resp, "X-RateLimit-Remaining-minute"),
		RemainingDay:    headerInt(resp, "X-RateLimit-Remaining-day"),
		RetryAfter:      headerInt(resp, "retry-after"),
		ResetAfter:      headerInt(resp, "ratelimit-reset"),
		LastStatusCode:  resp.StatusCode,
		LastStatusText:  resp.Status,
	}
	if resp.StatusCode == http.StatusTooManyRequests && c.rateLimits.RetryAfter > 0 {
		c.cooldownUntil = time.Now().Add(time.Duration(c.rateLimits.RetryAfter) * time.Second)
	} else if resp.StatusCode != http.StatusTooManyRequests {
		c.cooldownUntil = time.Time{}
	}
}

func headerInt(resp *http.Response, key string) int {
	value := resp.Header.Get(key)
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func cloneRawMessages(in []json.RawMessage) []json.RawMessage {
	out := make([]json.RawMessage, len(in))
	for i, item := range in {
		out[i] = append(json.RawMessage(nil), item...)
	}
	return out
}

type measurement struct {
	Value float64 `json:"value"`
}

type setpoint struct {
	Value float64 `json:"value"`
}

type managementPoint struct {
	EmbeddedID          string `json:"embeddedId"`
	ManagementPointType string `json:"managementPointType"`
	Name                *struct {
		Value string `json:"value"`
	} `json:"name"`
	OnOffMode *struct {
		Value string `json:"value"`
	} `json:"onOffMode"`
	OperationMode *struct {
		Value string `json:"value"`
	} `json:"operationMode"`
	IsInErrorState *struct {
		Value bool `json:"value"`
	} `json:"isInErrorState"`
	IsInWarningState *struct {
		Value bool `json:"value"`
	} `json:"isInWarningState"`
	IsInCautionState *struct {
		Value bool `json:"value"`
	} `json:"isInCautionState"`
	IsHolidayModeActive *struct {
		Value bool `json:"value"`
	} `json:"isHolidayModeActive"`
	SensoryData *struct {
		Value map[string]measurement `json:"value"`
	} `json:"sensoryData"`
	TemperatureControl *struct {
		Value struct {
			OperationModes map[string]struct {
				Setpoints map[string]setpoint `json:"setpoints"`
			} `json:"operationModes"`
		} `json:"value"`
	} `json:"temperatureControl"`
}

// DeviceState represents a Daikin unit with full management point data.
type DeviceState struct {
	Device           Device
	ManagementPoints []managementPoint
}

// deviceStateEntry is a partial representation of /v1/gateway-devices response.
type deviceStateEntry struct {
	ID                  string            `json:"id"`
	DeviceModel         string            `json:"deviceModel"`
	ManagementPoints    []managementPoint `json:"managementPoints"`
	IsCloudConnectionUp *struct {
		Value bool `json:"value"`
	} `json:"isCloudConnectionUp"`
}

func (d deviceStateEntry) toDeviceState() DeviceState {
	name := d.DeviceModel
	climateID := ""
	for _, mp := range d.ManagementPoints {
		if mp.ManagementPointType != "climateControl" && mp.ManagementPointType != "climateControlMainZone" {
			continue
		}
		if mp.Name != nil && mp.Name.Value != "" {
			name = mp.Name.Value
		}
		if climateID == "" {
			climateID = mp.EmbeddedID
		}
	}

	cloudConnected := false
	if d.IsCloudConnectionUp != nil {
		cloudConnected = d.IsCloudConnectionUp.Value
	}

	return DeviceState{
		Device: Device{
			ID:               d.ID,
			Name:             name,
			Model:            d.DeviceModel,
			ClimateControlID: climateID,
			CloudConnected:   cloudConnected,
		},
		ManagementPoints: d.ManagementPoints,
	}
}
