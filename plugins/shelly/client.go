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
// Shelly.GetInfoExt and Sys.GetInfo, which read and change nothing.
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
	System          *SystemReading
	SystemReadError string

	UptimeSeconds                 *float64
	Failsafe                      *bool
	Overheat                      *bool
	HomekitRunning                *bool
	HomekitPaired                 *bool
	HomekitConnections            *float64
	HomekitPendingConnections     *float64
	WifiConnected                 *bool
	WifiConnecting                *bool
	OTAProgress                   *float64
	CalibrationTravelMilliseconds *float64
	CalibrationPowerWatts         *float64

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
	UptimeSeconds             *float64 `json:"uptime"`
	Failsafe                  *bool    `json:"failsafe_mode"`
	Overheat                  *bool    `json:"overheat_on"`
	HomekitRunning            *bool    `json:"hap_running"`
	HomekitPaired             *bool    `json:"hap_paired"`
	HomekitConnections        *float64 `json:"hap_ip_conns_active"`
	HomekitPendingConnections *float64 `json:"hap_ip_conns_pending"`
	WifiConnected             *bool    `json:"wifi_connected"`
	WifiConnecting            *bool    `json:"wifi_connecting"`
	OTAProgress               *float64 `json:"ota_progress"`

	DeviceID string   `json:"device_id"`
	SysTemp  *float64 `json:"sys_temp"`
	WifiRssi *float64 `json:"wifi_conn_rssi"`

	Components []struct {
		CalibrationTravelMilliseconds *float64 `json:"move_time_ms"`
		CalibrationPowerWatts         *float64 `json:"move_power"`
		Type                          int      `json:"type"`
		CalDone                       bool     `json:"cal_done"`
		State                         int      `json:"state"`
		StateStr                      string   `json:"state_str"`
		CurPos                        *int     `json:"cur_pos"`
		TgtPos                        *int     `json:"tgt_pos"`
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
		reading.CalibrationTravelMilliseconds = component.CalibrationTravelMilliseconds
		reading.CalibrationPowerWatts = component.CalibrationPowerWatts

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
	reading.UptimeSeconds = info.UptimeSeconds
	reading.Failsafe = info.Failsafe
	reading.Overheat = info.Overheat
	reading.HomekitRunning = info.HomekitRunning
	reading.HomekitPaired = info.HomekitPaired
	reading.HomekitConnections = info.HomekitConnections
	reading.HomekitPendingConnections = info.HomekitPendingConnections
	reading.WifiConnected = info.WifiConnected
	reading.WifiConnecting = info.WifiConnecting
	reading.OTAProgress = info.OTAProgress

	reading.DeviceTemperatureCelsius = info.SysTemp
	if info.WifiConnected != nil && *info.WifiConnected {
		reading.WifiRssiDbm = info.WifiRssi
	}
	var system SystemReading
	if err := c.get(ctx, cover.Address, "Sys.GetInfo", &system); err != nil {
		reading.SystemReadError = err.Error()
	} else {
		reading.System = &system
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

type SystemReading struct {
	RamSize    *float64 `json:"ram_size"`
	RamFree    *float64 `json:"ram_free"`
	RamMinFree *float64 `json:"ram_min_free"`
	FsSize     *float64 `json:"fs_size"`
	FsFree     *float64 `json:"fs_free"`
}

func (c *Client) getInfoExt(ctx context.Context, address string) (infoExt, error) {
	var info infoExt
	err := c.get(ctx, address, "Shelly.GetInfoExt", &info)
	return info, err
}

func (c *Client) get(ctx context.Context, address, method string, result any) error {
	endpoint := (&url.URL{Scheme: "http", Host: address, Path: "/rpc/" + method}).String()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read %s: %w", endpoint, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request %s: %s: %s", endpoint, resp.Status, strings.TrimSpace(string(payload)))
	}
	if err := json.Unmarshal(payload, result); err != nil {
		return fmt.Errorf("decode %s: %w", endpoint, err)
	}
	return nil
}
