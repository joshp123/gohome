package brother

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maintenanceRequestTimeout = 10 * time.Second

// Client fetches read-only maintenance telemetry from a Brother network printer.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient constructs a Brother printer HTTP client for baseURL (e.g. "http://192.0.2.50").
func NewClient(baseURL string) (*Client, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return nil, fmt.Errorf("brother base_url is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("brother base_url %q must be an http(s) origin", baseURL)
	}
	return &Client{
		baseURL:    trimmed,
		httpClient: &http.Client{Timeout: maintenanceRequestTimeout},
	}, nil
}

// FetchPrinterStatus reads /etc/mnt_info.csv and parses it into a strongly typed PrinterStatus.
func (c *Client) FetchPrinterStatus(ctx context.Context) (PrinterStatus, error) {
	endpoint := c.baseURL + "/etc/mnt_info.csv"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return PrinterStatus{}, fmt.Errorf("build brother request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return PrinterStatus{}, fmt.Errorf("fetch %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return PrinterStatus{}, fmt.Errorf("read %s: %w", endpoint, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return PrinterStatus{}, fmt.Errorf("fetch %s: HTTP %d", endpoint, resp.StatusCode)
	}
	return ParseMaintenanceCSV(bytes.NewReader(body), time.Now().UTC())
}
