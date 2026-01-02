package growatt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	apiPrefix  = "v1/"
	timeLayout = "2006-01-02 15:04:05"
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
		return apiErr.Code == 10012
	}
	return false
}

// Client talks to the Growatt OpenAPI.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
	plantID *int64
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
	return plants, nil
}

func (c *Client) ResolvePlant(ctx context.Context, requestedID int64) (Plant, error) {
	plants, err := c.ListPlants(ctx)
	if err != nil {
		return Plant{}, err
	}

	var targetID int64
	if requestedID > 0 {
		targetID = requestedID
	} else if c.plantID != nil {
		targetID = *c.plantID
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
	if value := parseString(raw["last_update_time"]); value != "" {
		if t, err := time.ParseInLocation(timeLayout, value, time.Local); err == nil {
			energy.LastUpdate = &t
		}
	}

	return energy, nil
}

func (c *Client) getJSON(ctx context.Context, path string, params map[string]string, out any) error {
	endpoint := c.baseURL + apiPrefix + strings.TrimPrefix(path, "/")
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
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
		Data      json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if wrapper.ErrorCode != 0 {
		return APIError{Code: wrapper.ErrorCode, Msg: wrapper.ErrorMsg}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(wrapper.Data, out); err != nil {
		return fmt.Errorf("decode data: %w", err)
	}
	return nil
}

func parseFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int64:
		return float64(typed)
	case int:
		return float64(typed)
	case string:
		if typed == "" {
			return 0
		}
		if parsed, err := strconv.ParseFloat(typed, 64); err == nil {
			return parsed
		}
	}
	return 0
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
