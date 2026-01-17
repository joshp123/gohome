package airgradient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	requestTimeout = 10 * time.Second
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("airgradient base_url is required")
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
	}, nil
}

func (c *Client) Current(ctx context.Context) (CurrentMeasures, error) {
	var current CurrentMeasures
	if err := c.getJSON(ctx, "/measures/current", &current); err != nil {
		return CurrentMeasures{}, err
	}
	return current, nil
}

func (c *Client) RawCurrent(ctx context.Context) ([]byte, error) {
	return c.getBytes(ctx, "/measures/current")
}

func (c *Client) Config(ctx context.Context) ([]byte, error) {
	return c.getBytes(ctx, "/config")
}

func (c *Client) Metrics(ctx context.Context) ([]byte, error) {
	return c.getBytes(ctx, "/metrics")
}

func (c *Client) getJSON(ctx context.Context, path string, dest any) error {
	payload, err := c.getBytes(ctx, path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(payload, dest); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func (c *Client) getBytes(ctx context.Context, path string) ([]byte, error) {
	endpoint, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return nil, fmt.Errorf("build url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", endpoint, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("request %s: %s", endpoint, strings.TrimSpace(string(payload)))
	}

	return payload, nil
}
