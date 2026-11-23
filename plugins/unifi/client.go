package unifi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const unifiRequestTimeout = 15 * time.Second

// Client performs read-only telemetry reads against a local UniFi OS / Network controller.
type Client struct {
	baseURL    string
	site       string
	apiKeyFile string
	httpClient *http.Client
}

// NewClient constructs a UniFi Network API client backed by apiKeyFile.
func NewClient(baseURL, apiKeyFile, site string, insecureSkipVerify bool) (*Client, error) {
	trimmedURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmedURL == "" {
		return nil, fmt.Errorf("unifi base_url is required")
	}
	parsed, err := url.Parse(trimmedURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("unifi base_url %q must be an http(s) origin", baseURL)
	}
	trimmedKey := strings.TrimSpace(apiKeyFile)
	if trimmedKey == "" {
		return nil, fmt.Errorf("unifi api_key_file is required")
	}
	trimmedSite := strings.TrimSpace(site)
	if trimmedSite == "" {
		trimmedSite = "default"
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // local UniFi gateways use self-signed certificates
	}

	return &Client{
		baseURL:    trimmedURL,
		site:       trimmedSite,
		apiKeyFile: trimmedKey,
		httpClient: &http.Client{
			Timeout:   unifiRequestTimeout,
			Transport: transport,
		},
	}, nil
}

// FetchSiteSummary reads controller identity, subsystem health, adopted devices, and active client counts.
func (c *Client) FetchSiteSummary(ctx context.Context) (SiteSummary, error) {
	apiKey, err := c.readAPIKey()
	if err != nil {
		return SiteSummary{}, err
	}

	summary := SiteSummary{
		Site:       c.site,
		ObservedAt: time.Now().UTC(),
	}

	var sys systemResponse
	if err := c.getSystem(ctx, apiKey, &sys); err == nil {
		summary.ControllerName = sys.Name
		summary.HardwareShortname = sys.Hardware.Shortname
	}

	sitePath := "/proxy/network/api/s/" + url.PathEscape(c.site)

	var health healthEnvelope
	if err := c.getHealth(ctx, apiKey, sitePath+"/stat/health", &health); err != nil {
		return SiteSummary{}, err
	}
	for _, entry := range health.Data {
		summary.Subsystems = append(summary.Subsystems, SubsystemHealth{
			Subsystem:              entry.Subsystem,
			Status:                 entry.Status,
			ActiveUsers:            entry.NumUser,
			ActiveGuests:           entry.NumGuest,
			AdoptedDevices:         entry.NumAdopted,
			DisconnectedDevices:    entry.NumDisconnected,
			TxBytesPerSecond:       entry.TxBytesRate,
			RxBytesPerSecond:       entry.RxBytesRate,
			WANLatencyMilliseconds: entry.Latency,
			WANUptimeSeconds:       entry.Uptime,
			WANIP:                  entry.WANIP,
			ISPName:                entry.ISPName,
		})
	}

	var devices deviceEnvelope
	if err := c.getDevices(ctx, apiKey, sitePath+"/stat/device", &devices); err != nil {
		return SiteSummary{}, err
	}
	for _, dev := range devices.Data {
		name := dev.Name
		if name == "" {
			name = dev.Model
		}
		summary.Devices = append(summary.Devices, NetworkDevice{
			MAC:                 dev.MAC,
			Name:                name,
			Model:               dev.Model,
			DeviceType:          dev.Type,
			IP:                  dev.IP,
			Version:             dev.Version,
			Online:              dev.State == 1,
			UptimeSeconds:       dev.Uptime,
			CPUPercent:          parsePercentString(dev.SystemStats.CPU),
			MemoryPercent:       parsePercentString(dev.SystemStats.Mem),
			TemperatureCelsius:  dev.GeneralTemperature,
			ActiveClients:       dev.NumSta,
			SatisfactionPercent: dev.Satisfaction,
		})
	}

	var clients clientEnvelope
	if err := c.getClients(ctx, apiKey, sitePath+"/stat/sta", &clients); err != nil {
		return SiteSummary{}, err
	}
	for _, sta := range clients.Data {
		if sta.IsWired {
			summary.WiredClients++
		} else {
			summary.WirelessClients++
		}
	}

	return summary, nil
}

func (c *Client) readAPIKey() (string, error) {
	raw, err := os.ReadFile(c.apiKeyFile)
	if err != nil {
		return "", fmt.Errorf("read unifi api key file: %w", err)
	}
	key := strings.TrimSpace(string(raw))
	if key == "" {
		return "", fmt.Errorf("unifi api key file is empty")
	}
	return key, nil
}

func (c *Client) fetchBytes(ctx context.Context, apiKey, path string) ([]byte, error) {
	endpoint := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build unifi request %s: %w", path, err)
	}
	req.Header.Set("X-API-KEY", apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch unifi %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read unifi %s: %w", path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unifi %s returned HTTP %d", path, resp.StatusCode)
	}
	return body, nil
}

func (c *Client) getSystem(ctx context.Context, apiKey string, out *systemResponse) error {
	body, err := c.fetchBytes(ctx, apiKey, "/api/system")
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func (c *Client) getHealth(ctx context.Context, apiKey, path string, out *healthEnvelope) error {
	body, err := c.fetchBytes(ctx, apiKey, path)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func (c *Client) getDevices(ctx context.Context, apiKey, path string, out *deviceEnvelope) error {
	body, err := c.fetchBytes(ctx, apiKey, path)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func (c *Client) getClients(ctx context.Context, apiKey, path string, out *clientEnvelope) error {
	body, err := c.fetchBytes(ctx, apiKey, path)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}
