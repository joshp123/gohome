package shelly

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const requestTimeout = 8 * time.Second

// coverComponentType is the shelly-homekit firmware's component type for a window covering.
const coverComponentType = 4

// Client reads relays running the shelly-homekit firmware. It only ever calls
// Shelly.GetInfoExt, which reads and changes nothing.
type Client struct {
	covers     []CoverConfig
	httpClient *http.Client
}

func NewClient(covers []CoverConfig) *Client {
	return &Client{covers: covers, httpClient: &http.Client{Timeout: requestTimeout}}
}

// CoverReading is one successful read of a shutter's relay, or the error that
// stopped it. When Err is set, no other reading field is meaningful.
type CoverReading struct {
	Cover  CoverConfig
	Err    error
	ReadAt time.Time

	// PositionPercent and TargetPositionPercent are nil when the relay does not
	// know them, or is not calibrated and so cannot track the shutter.
	PositionPercent       *int
	TargetPositionPercent *int
	State                 string
	MotorRunning          bool
	Calibrated            bool

	DeviceTemperatureCelsius *float64
	WifiRssiDbm              *float64
}

// ReadAll reads every configured relay concurrently, in config order.
func (c *Client) ReadAll(ctx context.Context) []CoverReading {
	readings := make([]CoverReading, len(c.covers))
	var wg sync.WaitGroup
	for i, cover := range c.covers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			readings[i] = c.read(ctx, cover)
		}()
	}
	wg.Wait()
	return readings
}

type infoExt struct {
	DeviceID      string   `json:"device_id"`
	SysTemp       *float64 `json:"sys_temp"`
	WifiRssi      *float64 `json:"wifi_conn_rssi"`
	WifiConnected bool     `json:"wifi_connected"`
	Components    []struct {
		Type     int    `json:"type"`
		CalDone  bool   `json:"cal_done"`
		State    int    `json:"state"`
		StateStr string `json:"state_str"`
		CurPos   *int   `json:"cur_pos"`
		TgtPos   *int   `json:"tgt_pos"`
	} `json:"components"`
}

func (c *Client) read(ctx context.Context, cover CoverConfig) CoverReading {
	reading := CoverReading{Cover: cover}
	info, err := c.getInfoExt(ctx, cover.Address)
	if err != nil {
		reading.Err = err
		return reading
	}
	// Relays get their addresses by DHCP. Never report one shutter's state under another's name.
	if info.DeviceID != cover.DeviceID {
		reading.Err = fmt.Errorf("relay at %s is %q, expected %q", cover.Address, info.DeviceID, cover.DeviceID)
		return reading
	}
	found := false
	for _, component := range info.Components {
		if component.Type != coverComponentType {
			continue
		}
		found = true
		reading.State = component.StateStr
		reading.MotorRunning = motorRunning(component.State)
		reading.Calibrated = component.CalDone
		if component.CalDone {
			reading.PositionPercent = knownPosition(component.CurPos)
			reading.TargetPositionPercent = knownPosition(component.TgtPos)
		}
		break
	}
	if !found {
		reading.Err = fmt.Errorf("relay %s has no cover component; is it in cover mode?", cover.DeviceID)
		return reading
	}
	reading.ReadAt = time.Now()
	reading.DeviceTemperatureCelsius = info.SysTemp
	if info.WifiConnected {
		reading.WifiRssiDbm = info.WifiRssi
	}
	return reading
}

// motorRunning reports whether the firmware's state machine is driving the
// motor: calibration states 10-15 and movement states 20-25 (move, rampup,
// moving, stop, stopping). See WindowCovering::State in shelly-homekit.
func motorRunning(state int) bool {
	return (state >= 10 && state <= 15) || (state >= 20 && state <= 25)
}

// knownPosition maps a missing position, or the firmware's "not set" (-1), to nil.
func knownPosition(position *int) *int {
	if position == nil || *position < 0 || *position > 100 {
		return nil
	}
	return position
}

func (c *Client) getInfoExt(ctx context.Context, address string) (infoExt, error) {
	endpoint := (&url.URL{Scheme: "http", Host: address, Path: "/rpc/Shelly.GetInfoExt"}).String()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return infoExt{}, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return infoExt{}, fmt.Errorf("request %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return infoExt{}, fmt.Errorf("read %s: %w", endpoint, err)
	}
	if resp.StatusCode != http.StatusOK {
		return infoExt{}, fmt.Errorf("request %s: %s: %s", endpoint, resp.Status, strings.TrimSpace(string(payload)))
	}
	var info infoExt
	if err := json.Unmarshal(payload, &info); err != nil {
		return infoExt{}, fmt.Errorf("decode %s: %w", endpoint, err)
	}
	return info, nil
}
