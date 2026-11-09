package hikvision

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// The only camera resources this plugin reads. Every request is a GET,
// except the two searches, which ISAPI asks for as a POST of the search terms
// and which only read: the camera's logs and its recordings. The client has
// no way to send anything else.
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
	pathLogSearch              = "/ISAPI/ContentMgmt/logSearch"
	pathRecordingSearch        = "/ISAPI/ContentMgmt/search"
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

// searchPaths are the only resources the client POSTs to. Both are searches.
var searchPaths = map[string]bool{pathLogSearch: true, pathRecordingSearch: true}

// get answers the camera's Digest challenge and returns the authorised
// response, which the caller must close.
func (c *Client) get(ctx context.Context, httpClient *http.Client, path string) (*http.Response, error) {
	return c.request(ctx, httpClient, http.MethodGet, path, nil)
}

// request sends a GET, or a POST of search terms to a search resource.
func (c *Client) request(ctx context.Context, httpClient *http.Client, method, path string, body []byte) (*http.Response, error) {
	if method != http.MethodGet && !(method == http.MethodPost && searchPaths[path]) {
		return nil, fmt.Errorf("%s %s: this client only reads", method, path)
	}
	c.mu.Lock()
	rejectedAt := c.credentialsRejectedAt
	c.mu.Unlock()
	if !rejectedAt.IsZero() && time.Since(rejectedAt) < credentialsRejectedBackoff {
		return nil, fmt.Errorf("camera rejected the credentials at %s; not retrying until %s",
			rejectedAt.Format(time.RFC3339), rejectedAt.Add(credentialsRejectedBackoff).Format(time.RFC3339))
	}

	endpoint := c.baseURL + path
	challengeResp, err := c.send(ctx, httpClient, method, endpoint, body, "")
	if err != nil {
		return nil, err
	}
	if challengeResp.StatusCode != http.StatusUnauthorized {
		challengeResp.Body.Close()
		return nil, fmt.Errorf("%s %s: expected a Digest challenge, got %s", method, path, challengeResp.Status)
	}
	header := challengeResp.Header.Get("WWW-Authenticate")
	drainAndClose(challengeResp)
	challenge, err := parseDigestChallenge(header)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}

	resp, err := c.send(ctx, httpClient, method, endpoint, body, challenge.authorization(c.username, c.password, method, path))
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		drainAndClose(resp)
		c.mu.Lock()
		c.credentialsRejectedAt = time.Now()
		c.mu.Unlock()
		return nil, fmt.Errorf("%s %s: camera rejected the credentials", method, path)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		drainAndClose(resp)
		return nil, fmt.Errorf("%s %s: %s", method, path, resp.Status)
	}
	c.mu.Lock()
	c.credentialsRejectedAt = time.Time{}
	c.mu.Unlock()
	return resp, nil
}

func (c *Client) send(ctx context.Context, httpClient *http.Client, method, endpoint string, body []byte, authorization string) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/xml")
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, req.URL.Path, err)
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

// search POSTs search terms to a search resource and decodes the result.
func (c *Client) search(ctx context.Context, path string, terms any, dest any) error {
	body, err := xml.Marshal(terms)
	if err != nil {
		return fmt.Errorf("encode %s search: %w", path, err)
	}
	resp, err := c.request(ctx, c.readClient, http.MethodPost, path, append([]byte(xml.Header), body...))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	result, err := io.ReadAll(io.LimitReader(resp.Body, maxXMLResponseBytes+1))
	if err != nil {
		return fmt.Errorf("POST %s: read body: %w", path, err)
	}
	if len(result) > maxXMLResponseBytes {
		return fmt.Errorf("POST %s: response larger than %d bytes", path, maxXMLResponseBytes)
	}
	return decodeXML(path, result, dest)
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
