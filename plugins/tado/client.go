package tado

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client talks to the Tado REST API.
type Client struct {
	baseURL string
	authURL string

	clientID     string
	clientSecret string
	refreshToken string
	scope        string

	httpClient *http.Client

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
	homeID      int
}

func NewClient(cfg Config) (*Client, error) {
	return &Client{
		baseURL:      cfg.BaseURL,
		authURL:      cfg.AuthURL,
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		refreshToken: cfg.RefreshToken,
		scope:        cfg.Scope,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
		homeID:       cfg.HomeID,
	}, nil
}

func (c *Client) HomeID(ctx context.Context) (int, error) {
	if c.homeID != 0 {
		return c.homeID, nil
	}

	var resp struct {
		Homes []struct {
			ID int `json:"id"`
		} `json:"homes"`
	}

	if err := c.getJSON(ctx, "/me", &resp); err != nil {
		return 0, err
	}
	if len(resp.Homes) == 0 {
		return 0, fmt.Errorf("no homes found in /me response")
	}

	c.homeID = resp.Homes[0].ID
	return c.homeID, nil
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
					Celsius float64 `json:"celsius"`
				} `json:"insideTemperature"`
				Humidity struct {
					Percentage float64 `json:"percentage"`
				} `json:"humidity"`
			} `json:"sensorDataPoints"`
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
		states[id] = ZoneState{
			InsideTemperatureCelsius: state.SensorDataPoints.InsideTemperature.Celsius,
			HumidityPercent:          state.SensorDataPoints.Humidity.Percentage,
		}
	}
	return states, nil
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
			"type":              "MANUAL",
			"typeSkillBasedApp": "MANUAL",
		},
	}

	return c.putJSON(ctx, fmt.Sprintf("/homes/%d/zones/%d/overlay", homeID, zoneID), payload)
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
	accessToken, err := c.accessTokenFor(ctx)
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
	if err := c.refresh(ctx, true); err != nil {
		return nil, err
	}

	accessToken, err = c.accessTokenFor(ctx)
	if err != nil {
		return nil, err
	}

	req, err = http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	return c.httpClient.Do(req)
}

func (c *Client) accessTokenFor(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.accessToken != "" && time.Until(c.expiresAt) > time.Minute {
		return c.accessToken, nil
	}

	if err := c.refreshLocked(ctx); err != nil {
		return "", err
	}
	return c.accessToken, nil
}

func (c *Client) refresh(ctx context.Context, force bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && c.accessToken != "" && time.Until(c.expiresAt) > time.Minute {
		return nil
	}
	return c.refreshLocked(ctx)
}

func (c *Client) refreshLocked(ctx context.Context) error {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", c.refreshToken)
	data.Set("client_id", c.clientID)
	data.Set("client_secret", c.clientSecret)
	if c.scope != "" {
		data.Set("scope", c.scope)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.authURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token refresh failed %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var token struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return err
	}

	c.accessToken = token.AccessToken
	c.expiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	if token.RefreshToken != "" {
		c.refreshToken = token.RefreshToken
	}

	return nil
}
