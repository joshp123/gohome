package growatt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	v1 "github.com/joshp123/gohome/proto/gen/plugins/growatt/v1"
)

const (
	apiPrefix   = "v1/"
	apiV4Prefix = "v4/"
	timeLayout  = "2006-01-02 15:04:05"
)

// APIError surfaces Growatt error codes.
type APIError struct {
	Code int
	Msg  string
}

func (e APIError) Error() string {
	return fmt.Sprintf("growatt api error %d: %s", e.Code, e.Msg)
}

func isRateLimit(err error) bool {
	var apiErr APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code == 10012 || apiErr.Code == 100 || apiErr.Code == 102
	}
	return false
}

// Client talks to the Growatt OpenAPI.
type Client struct {
	baseURL      string
	token        string
	http         *http.Client
	plantID      *int64
	plantCacheMu sync.Mutex
	plantCache   []Plant
}

func NewClient(cfg Config) (*Client, error) {
	tokenBytes, err := os.ReadFile(cfg.TokenFile)
	if err != nil {
		return nil, fmt.Errorf("read growatt token file: %w", err)
	}
	token := strings.TrimSpace(string(tokenBytes))
	if token == "" {
		return nil, fmt.Errorf("growatt token is empty")
	}

	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = regionEndpoints[defaultRegion]
	}
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	return &Client{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 15 * time.Second},
		plantID: cfg.PlantID,
	}, nil
}

func (c *Client) ListPlants(ctx context.Context) ([]Plant, error) {
	var data struct {
		Count  int `json:"count"`
		Plants []struct {
			PlantID int64  `json:"plant_id"`
			Name    string `json:"name"`
			Status  int32  `json:"status"`
		} `json:"plants"`
	}

	if err := c.getJSON(ctx, "plant/list", nil, &data); err != nil {
		return nil, err
	}

	plants := make([]Plant, 0, len(data.Plants))
	for _, plant := range data.Plants {
		plants = append(plants, Plant{
			ID:     plant.PlantID,
			Name:   plant.Name,
			Status: plant.Status,
		})
	}
	c.plantCacheMu.Lock()
	c.plantCache = plants
	c.plantCacheMu.Unlock()
	return plants, nil
}

func (c *Client) ResolvePlant(ctx context.Context, requestedID int64) (Plant, error) {
	var targetID int64
	if requestedID > 0 {
		targetID = requestedID
	} else if c.plantID != nil {
		targetID = *c.plantID
	}

	if targetID > 0 {
		if plant, ok := c.cachedPlant(targetID); ok {
			return plant, nil
		}
	}

	plants, err := c.ListPlants(ctx)
	if err != nil {
		return Plant{}, err
	}

	if targetID == 0 {
		if len(plants) == 1 {
			return plants[0], nil
		}
		return Plant{}, fmt.Errorf("multiple plants found; set plant_id")
	}

	for _, plant := range plants {
		if plant.ID == targetID {
			return plant, nil
		}
	}

	return Plant{}, fmt.Errorf("plant_id %d not found", targetID)
}

func (c *Client) cachedPlant(id int64) (Plant, bool) {
	c.plantCacheMu.Lock()
	defer c.plantCacheMu.Unlock()
	for _, plant := range c.plantCache {
		if plant.ID == id {
			return plant, true
		}
	}
	return Plant{}, false
}

func (c *Client) EnergyOverview(ctx context.Context, plantID int64) (PlantEnergy, error) {
	params := map[string]string{"plant_id": strconv.FormatInt(plantID, 10)}

	var raw map[string]any
	if err := c.getJSON(ctx, "plant/data", params, &raw); err != nil {
		return PlantEnergy{}, err
	}

	energy := PlantEnergy{PlantID: plantID}
	energy.TodayEnergyKWh = parseFloat(raw["today_energy"])
	energy.MonthlyEnergyKWh = parseFloat(raw["monthly_energy"])
	energy.YearlyEnergyKWh = parseFloat(raw["yearly_energy"])
	energy.TotalEnergyKWh = parseFloat(raw["total_energy"])
	energy.CurrentPowerW = parseFloat(raw["current_power"])
	energy.Timezone = parseString(raw["timezone"])
	if t, err := parseGrowattTimestamp(parseString(raw["last_update_time"]), energy.Timezone); err == nil {
		energy.LastUpdate = t
	}
	if reading, err := c.plantLiveReading(ctx, plantID, energy.Timezone); err != nil {
		return PlantEnergy{}, err
	} else {
		energy.CurrentPowerW = reading.Watts
		energy.Inverter = reading.Telemetry
		if reading.LastUpdate != nil {
			energy.LastUpdate = reading.LastUpdate
		}
	}

	return energy, nil
}

func (c *Client) getJSON(ctx context.Context, path string, params map[string]string, out any) error {
	return c.doJSON(ctx, http.MethodGet, apiPrefix, path, params, out)
}

func (c *Client) postJSON(ctx context.Context, path string, params map[string]string, out any) error {
	return c.doJSON(ctx, http.MethodPost, apiV4Prefix, path, params, out)
}

func (c *Client) doJSON(ctx context.Context, method, prefix, path string, params map[string]string, out any) error {
	return c.doJSONOnce(ctx, method, prefix, path, params, out)
}

func (c *Client) doJSONOnce(ctx context.Context, method, prefix, path string, params map[string]string, out any) error {
	endpoint := c.baseURL + prefix + strings.TrimPrefix(path, "/")
	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	if params != nil {
		query := reqURL.Query()
		for key, value := range params {
			query.Set(key, value)
		}
		reqURL.RawQuery = query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("token", c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("growatt http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var wrapper struct {
		ErrorCode int             `json:"error_code"`
		ErrorMsg  string          `json:"error_msg"`
		Code      *int            `json:"code"`
		Message   string          `json:"message"`
		Data      json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if wrapper.ErrorCode != 0 {
		return APIError{Code: wrapper.ErrorCode, Msg: wrapper.ErrorMsg}
	}
	if wrapper.Code != nil && *wrapper.Code != 0 {
		return APIError{Code: *wrapper.Code, Msg: wrapper.Message}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(wrapper.Data, out); err != nil {
		return fmt.Errorf("decode data: %w", err)
	}
	return nil
}

type growattDevice struct {
	SN   string `json:"deviceSn"`
	Type string `json:"deviceType"`
}

type livePowerReading struct {
	Telemetry  *v1.InverterTelemetry
	Watts      float64
	LastUpdate *time.Time
}

func (c *Client) plantLivePower(ctx context.Context, plantID int64, timezone string) (float64, *time.Time, error) {
	reading, err := c.plantLiveReading(ctx, plantID, timezone)
	return reading.Watts, reading.LastUpdate, err
}

func (c *Client) plantLiveReading(ctx context.Context, plantID int64, timezone string) (livePowerReading, error) {
	var list struct {
		Count   int `json:"count"`
		Devices []struct {
			SN   string `json:"device_sn"`
			Type int    `json:"type"`
		} `json:"devices"`
	}
	if err := c.getJSON(ctx, "device/list", map[string]string{"plant_id": strconv.FormatInt(plantID, 10), "page": "1", "perpage": "100"}, &list); err != nil {
		return livePowerReading{}, err
	}
	if list.Count != 1 || len(list.Devices) != 1 {
		return livePowerReading{}, fmt.Errorf("plant must contain exactly one inverter; found %d", list.Count)
	}
	device := list.Devices[0]
	// Growatt OpenAPI device type 7 is MIN, the v4 queryLastData type.
	if device.Type != 7 || device.SN == "" {
		return livePowerReading{}, fmt.Errorf("unsupported Growatt device type %d", device.Type)
	}
	reading, err := c.deviceLivePower(ctx, growattDevice{SN: device.SN, Type: "min"}, timezone)
	return reading, err
}

func (c *Client) deviceLivePower(ctx context.Context, device growattDevice, timezone string) (livePowerReading, error) {
	var data map[string][]map[string]any
	if err := c.postJSON(ctx, "new-api/queryLastData", map[string]string{
		"deviceSn":   device.SN,
		"deviceType": device.Type,
	}, &data); err != nil {
		return livePowerReading{}, err
	}

	rows := data[device.Type]
	if len(rows) == 0 {
		return livePowerReading{}, fmt.Errorf("growatt live power data missing for device type %q", device.Type)
	}
	lastUpdate, err := parseGrowattTimestamp(parseString(rows[0]["time"]), timezone)
	if err != nil {
		return livePowerReading{}, err
	}
	power, ok := parseFloatOK(rows[0]["pac"])
	if !ok {
		return livePowerReading{}, fmt.Errorf("growatt live power pac missing")
	}
	telemetry, err := decodeInverterTelemetry(rows[0], device.SN, timezone)
	if err != nil {
		return livePowerReading{}, err
	}
	return livePowerReading{Watts: power, LastUpdate: lastUpdate, Telemetry: telemetry}, nil
}

func parseGrowattTimestamp(value, timezone string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	loc := time.Local
	if parsed, ok := fixedTimezone(timezone); ok {
		loc = parsed
	} else if timezone != "" {
		if loaded, err := time.LoadLocation(timezone); err == nil {
			loc = loaded
		}
	}

	t, err := time.ParseInLocation(timeLayout, value, loc)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func fixedTimezone(name string) (*time.Location, bool) {
	text := strings.TrimSpace(name)
	for _, prefix := range []string{"GMT", "UTC"} {
		if offset, ok := strings.CutPrefix(strings.ToUpper(text), prefix); ok && offset != "" {
			hours, err := strconv.Atoi(offset)
			if err != nil || hours < -23 || hours > 23 {
				return nil, false
			}
			return time.FixedZone(name, hours*60*60), true
		}
	}
	return nil, false
}

func parseFloat(value any) float64 {
	parsed, ok := parseFloatOK(value)
	if ok {
		return parsed
	}
	return 0
}

func parseFloatOK(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, !math.IsNaN(typed) && !math.IsInf(typed, 0)
	case float32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int:
		return float64(typed), true
	case string:
		if typed == "" {
			return 0, false
		}
		if parsed, err := strconv.ParseFloat(typed, 64); err == nil {
			return parsed, !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
		}
	}
	return 0, false
}

func parseString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	}
	return ""
}
