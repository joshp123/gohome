package p1_homewizard

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
		return nil, fmt.Errorf("p1_homewizard base_url is required")
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
	}, nil
}

func (c *Client) Info(ctx context.Context) (Info, error) {
	var info Info
	if err := c.getJSON(ctx, "/api", &info); err != nil {
		return Info{}, err
	}
	return info, nil
}

func (c *Client) Data(ctx context.Context) (Data, error) {
	var data Data
	if err := c.getJSON(ctx, "/api/v1/data", &data); err != nil {
		return Data{}, err
	}
	return data, nil
}

func (c *Client) RawData(ctx context.Context) ([]byte, error) {
	return c.getBytes(ctx, "/api/v1/data")
}

func (c *Client) Telegram(ctx context.Context) (string, error) {
	bytes, err := c.getBytes(ctx, "/api/v1/telegram")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (c *Client) getJSON(ctx context.Context, path string, dest any) error {
	bytes, err := c.getBytes(ctx, path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(bytes, dest); err != nil {
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
