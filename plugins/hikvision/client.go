package hikvision

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// The only camera resources this plugin reads. Every request is a GET; the
// client has no way to send anything else.
const (
	pathDeviceInfo             = "/ISAPI/System/deviceInfo"
	pathSystemCapabilities     = "/ISAPI/System/capabilities"
	pathEventCapabilities      = "/ISAPI/Event/capabilities"
	pathMotionDetection        = "/ISAPI/System/Video/inputs/channels/1/motionDetection"
	pathLineDetection          = "/ISAPI/Smart/LineDetection/1"
	pathLineDetectionCaps      = "/ISAPI/Smart/LineDetection/1/capabilities"
	pathFieldDetection         = "/ISAPI/Smart/FieldDetection/1"
	pathFieldDetectionCaps     = "/ISAPI/Smart/FieldDetection/1/capabilities"
	pathMainStreamPicture      = "/ISAPI/Streaming/channels/101/picture"
	pathAlertStream            = "/ISAPI/Event/notification/alertStream"
	readTimeout                = 10 * time.Second
	maxXMLResponseBytes        = 1 << 20
	maxSnapshotBytes           = 8 << 20
	credentialsRejectedBackoff = 30 * time.Minute
)

// Client reads one camera over ISAPI with HTTP Digest authentication.
type Client struct {
	baseURL, username, password string
	readClient                  *http.Client
	streamClient                *http.Client

	mu sync.Mutex
	// The camera locks out a login after repeated wrong passwords, which would
	// also lock out anyone else using it. After one rejection, stop asking.
	credentialsRejectedAt time.Time
}

func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" || cfg.Username == "" || cfg.Password == "" {
		return nil, fmt.Errorf("hikvision base_url, username and password are required")
	}
	return &Client{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		username:   cfg.Username,
		password:   cfg.Password,
		readClient: &http.Client{Timeout: readTimeout},
		// No overall timeout: the stream never ends. The alert-stream reader
		// closes it when the camera goes quiet.
		streamClient: &http.Client{Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: readTimeout}).DialContext,
			ResponseHeaderTimeout: readTimeout,
		}},
	}, nil
}

// get answers the camera's Digest challenge and returns the authorised
// response, which the caller must close.
func (c *Client) get(ctx context.Context, httpClient *http.Client, path string) (*http.Response, error) {
	c.mu.Lock()
	rejectedAt := c.credentialsRejectedAt
	c.mu.Unlock()
	if !rejectedAt.IsZero() && time.Since(rejectedAt) < credentialsRejectedBackoff {
		return nil, fmt.Errorf("camera rejected the credentials at %s; not retrying until %s",
			rejectedAt.Format(time.RFC3339), rejectedAt.Add(credentialsRejectedBackoff).Format(time.RFC3339))
	}

	endpoint := c.baseURL + path
	challengeResp, err := c.send(ctx, httpClient, endpoint, "")
	if err != nil {
		return nil, err
	}
	if challengeResp.StatusCode != http.StatusUnauthorized {
		challengeResp.Body.Close()
		return nil, fmt.Errorf("GET %s: expected a Digest challenge, got %s", path, challengeResp.Status)
	}
	header := challengeResp.Header.Get("WWW-Authenticate")
	drainAndClose(challengeResp)
	challenge, err := parseDigestChallenge(header)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}

	resp, err := c.send(ctx, httpClient, endpoint, challenge.authorization(c.username, c.password, http.MethodGet, path))
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		drainAndClose(resp)
		c.mu.Lock()
		c.credentialsRejectedAt = time.Now()
		c.mu.Unlock()
		return nil, fmt.Errorf("GET %s: camera rejected the credentials", path)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		drainAndClose(resp)
		return nil, fmt.Errorf("GET %s: %s", path, resp.Status)
	}
	c.mu.Lock()
	c.credentialsRejectedAt = time.Time{}
	c.mu.Unlock()
	return resp, nil
}

func (c *Client) send(ctx context.Context, httpClient *http.Client, endpoint, authorization string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", req.URL.Path, err)
	}
	return resp, nil
}

func drainAndClose(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
}

func (c *Client) getBody(ctx context.Context, path string, limit int64) ([]byte, http.Header, error) {
	resp, err := c.get(ctx, c.readClient, path)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, nil, fmt.Errorf("GET %s: read body: %w", path, err)
	}
	if int64(len(body)) > limit {
		return nil, nil, fmt.Errorf("GET %s: response larger than %d bytes", path, limit)
	}
	return body, resp.Header, nil
}

func (c *Client) getXML(ctx context.Context, path string, dest any) error {
	body, _, err := c.getBody(ctx, path, maxXMLResponseBytes)
	if err != nil {
		return err
	}
	return decodeXML(path, body, dest)
}

func (c *Client) DeviceInfo(ctx context.Context) (deviceInfoXML, error) {
	var info deviceInfoXML
	return info, c.getXML(ctx, pathDeviceInfo, &info)
}

func (c *Client) MotionDetection(ctx context.Context) (motionDetectionXML, error) {
	var motion motionDetectionXML
	return motion, c.getXML(ctx, pathMotionDetection, &motion)
}

func (c *Client) LineDetection(ctx context.Context) (smartDetectionXML, error) {
	var line smartDetectionXML
	return line, c.getXML(ctx, pathLineDetection, &line)
}

func (c *Client) FieldDetection(ctx context.Context) (smartDetectionXML, error) {
	var field smartDetectionXML
	return field, c.getXML(ctx, pathFieldDetection, &field)
}

func (c *Client) LineDetectionCapabilities(ctx context.Context) (smartDetectionXML, error) {
	var caps smartDetectionXML
	return caps, c.getXML(ctx, pathLineDetectionCaps, &caps)
}

func (c *Client) FieldDetectionCapabilities(ctx context.Context) (smartDetectionXML, error) {
	var caps smartDetectionXML
	return caps, c.getXML(ctx, pathFieldDetectionCaps, &caps)
}

// SupportedEvents lists the event and smart-event capabilities the camera
// reports as supported, in its own words.
func (c *Client) SupportedEvents(ctx context.Context) ([]string, error) {
	eventCaps, _, err := c.getBody(ctx, pathEventCapabilities, maxXMLResponseBytes)
	if err != nil {
		return nil, err
	}
	systemCaps, _, err := c.getBody(ctx, pathSystemCapabilities, maxXMLResponseBytes)
	if err != nil {
		return nil, err
	}
	events, err := supportedCapabilities(eventCaps, "EventCap")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", pathEventCapabilities, err)
	}
	smart, err := supportedCapabilities(systemCaps, "SmartCap")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", pathSystemCapabilities, err)
	}
	return mergeSorted(events, smart), nil
}

// Snapshot returns one JPEG from the main stream.
func (c *Client) Snapshot(ctx context.Context) ([]byte, error) {
	body, header, err := c.getBody(ctx, pathMainStreamPicture, maxSnapshotBytes)
	if err != nil {
		return nil, err
	}
	if contentType := header.Get("Content-Type"); !strings.HasPrefix(contentType, "image/jpeg") {
		return nil, fmt.Errorf("GET %s: expected image/jpeg, got %q", pathMainStreamPicture, contentType)
	}
	if len(body) < 4 || body[0] != 0xFF || body[1] != 0xD8 {
		return nil, fmt.Errorf("GET %s: response is not a JPEG", pathMainStreamPicture)
	}
	return body, nil
}

// OpenAlertStream opens the camera's never-ending event stream. The caller
// cancels ctx to close it.
func (c *Client) OpenAlertStream(ctx context.Context) (*http.Response, error) {
	return c.get(ctx, c.streamClient, pathAlertStream)
}
