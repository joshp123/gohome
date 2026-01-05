package roborock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrNotImplemented = errors.New("roborock client not implemented")

// BootstrapState captures persisted Roborock login state.
type BootstrapState struct {
	SchemaVersion int             `json:"schema_version"`
	Username      string          `json:"username"`
	UserData      json.RawMessage `json:"user_data"`
	BaseURL       string          `json:"base_url"`
}

// Client talks to Roborock cloud/local APIs.
type Client struct {
	cfg       Config
	bootstrap BootstrapState
	userData  *UserData
	api       *RoborockApiClient
	mu        sync.Mutex
	homeData  *HomeData
	devices   map[string]HomeDataDevice
	products  map[string]HomeDataProduct
	channels  map[string]*LocalChannel
	ipCache   map[string]string
	overrides map[string]string
	mqtt      *mqttClient
	mapCache  map[string]mapSnapshot
}

func LoadBootstrap(path string) (BootstrapState, error) {
	data, err := osReadFile(path)
	if err != nil {
		return BootstrapState{}, fmt.Errorf("read roborock bootstrap: %w", err)
	}

	var state BootstrapState
	if err := json.Unmarshal(data, &state); err != nil {
		return BootstrapState{}, fmt.Errorf("parse roborock bootstrap: %w", err)
	}
	if state.SchemaVersion != 1 {
		return BootstrapState{}, fmt.Errorf("unsupported roborock bootstrap schema_version %d", state.SchemaVersion)
	}
	if state.Username == "" {
		return BootstrapState{}, fmt.Errorf("roborock bootstrap missing username")
	}
	if len(state.UserData) == 0 {
		return BootstrapState{}, fmt.Errorf("roborock bootstrap missing user_data")
	}
	if state.BaseURL == "" {
		return BootstrapState{}, fmt.Errorf("roborock bootstrap missing base_url")
	}

	return state, nil
}

func NewClient(cfg Config) (*Client, error) {
	bootstrap, err := LoadBootstrap(cfg.BootstrapFile)
	if err != nil {
		return nil, err
	}

	userData, err := parseUserData(bootstrap.UserData)
	if err != nil {
		return nil, err
	}

	api := NewRoborockApiClient(bootstrap.Username, bootstrap.BaseURL)

	return &Client{
		cfg:       cfg,
		bootstrap: bootstrap,
		userData:  userData,
		api:       api,
		devices:   make(map[string]HomeDataDevice),
		products:  make(map[string]HomeDataProduct),
		channels:  make(map[string]*LocalChannel),
		ipCache:   make(map[string]string),
		overrides: cfg.IPOverrides,
		mapCache:  make(map[string]mapSnapshot),
	}, nil
}

func parseUserData(raw json.RawMessage) (*UserData, error) {
	var data UserData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse user data: %w", err)
	}
	if data.RRIOT.U == "" || data.RRIOT.S == "" || data.RRIOT.H == "" || data.RRIOT.K == "" {
		return nil, errors.New("user data missing rriot fields")
	}
	return &data, nil
}

func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	if err := c.ensureHomeData(ctx); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Device, 0, len(c.devices))
	for _, dev := range c.devices {
		product := c.products[dev.ProductID]
		supportsMop := false
		if dev.DeviceStatus != nil {
			if _, ok := dev.DeviceStatus["124"]; ok {
				supportsMop = true
			}
		}
		out = append(out, Device{
			ID:          dev.DUID,
			Name:        dev.Name,
			Model:       product.Model,
			Firmware:    dev.Firmware,
			SupportsMop: supportsMop,
		})
	}
	return out, nil
}

func (c *Client) DeviceStates(ctx context.Context) ([]DeviceState, error) {
	if err := c.ensureHomeData(ctx); err != nil {
		return nil, err
	}
	devices, err := c.Devices(ctx)
	if err != nil {
		return nil, err
	}
	var out []DeviceState
	for _, dev := range devices {
		status, err := c.Status(ctx, dev.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, DeviceState{Device: dev, Status: status})
	}
	return out, nil
}

func (c *Client) Status(ctx context.Context, deviceID string) (Status, error) {
	if err := c.ensureHomeData(ctx); err != nil {
		return Status{}, err
	}
	device, err := c.deviceByID(deviceID)
	if err != nil {
		return Status{}, err
	}
	channel, err := c.getLocalChannel(ctx, device)
	if err != nil {
		if c.cfg.CloudFallback {
			return parseStatusFromDeviceStatus(device.DeviceStatus), nil
		}
		return Status{}, err
	}
	statusData, err := c.sendRPC(ctx, channel, "get_status", nil)
	if err != nil {
		return Status{}, err
	}
	consumable, err := c.sendRPC(ctx, channel, "get_consumable", nil)
	if err != nil {
		return Status{}, err
	}
	cleanSummary, err := c.sendRPC(ctx, channel, "get_clean_summary", nil)
	if err != nil {
		return Status{}, err
	}

	status := parseStatus(statusData, consumable, cleanSummary)
	return status, nil
}

func (c *Client) StartClean(ctx context.Context, deviceID string) error {
	return c.simpleCommand(ctx, deviceID, "app_start", nil)
}

func (c *Client) Pause(ctx context.Context, deviceID string) error {
	return c.simpleCommand(ctx, deviceID, "app_pause", nil)
}

func (c *Client) Stop(ctx context.Context, deviceID string) error {
	return c.simpleCommand(ctx, deviceID, "app_stop", nil)
}

func (c *Client) Dock(ctx context.Context, deviceID string) error {
	return c.simpleCommand(ctx, deviceID, "app_charge", nil)
}

func (c *Client) Locate(ctx context.Context, deviceID string) error {
	return c.simpleCommand(ctx, deviceID, "find_me", nil)
}

func (c *Client) SetFanSpeed(ctx context.Context, deviceID string, speed string) error {
	return c.simpleCommand(ctx, deviceID, "set_custom_mode", map[string]any{"fan_power": numericOrString(speed)})
}

func (c *Client) SetMopMode(ctx context.Context, deviceID string, mode string) error {
	return c.simpleCommand(ctx, deviceID, "set_mop_mode", []any{numericOrString(mode)})
}

func (c *Client) SetMopIntensity(ctx context.Context, deviceID string, intensity string) error {
	return c.simpleCommand(ctx, deviceID, "set_water_box_custom_mode", []any{numericOrString(intensity)})
}

func (c *Client) CleanZone(ctx context.Context, deviceID string, zones []Zone, repeats int) error {
	zoneParams := make([][]int, 0, len(zones))
	for _, z := range zones {
		zoneParams = append(zoneParams, []int{z.X1, z.Y1, z.X2, z.Y2})
	}
	if repeats == 0 {
		repeats = 1
	}
	params := []any{zoneParams, repeats}
	return c.simpleCommand(ctx, deviceID, "app_zoned_clean", params)
}

func (c *Client) CleanSegment(ctx context.Context, deviceID string, segments []int, repeats int) error {
	if repeats == 0 {
		repeats = 1
	}
	params := []any{segments, repeats}
	return c.simpleCommand(ctx, deviceID, "app_segment_clean", params)
}

func (c *Client) GoTo(ctx context.Context, deviceID string, x int, y int) error {
	params := []any{x, y}
	return c.simpleCommand(ctx, deviceID, "app_goto_target", params)
}

func (c *Client) SetDND(ctx context.Context, deviceID string, start string, end string, enabled bool) error {
	startHour, startMin, err := parseClock(start)
	if err != nil {
		return err
	}
	endHour, endMin, err := parseClock(end)
	if err != nil {
		return err
	}
	params := []any{startHour, startMin, endHour, endMin}
	cmd := "set_dnd_timer"
	if !enabled {
		cmd = "close_dnd_timer"
		params = nil
	}
	return c.simpleCommand(ctx, deviceID, cmd, params)
}

func (c *Client) ResetConsumable(ctx context.Context, deviceID string, consumable string) error {
	params := []any{consumable}
	return c.simpleCommand(ctx, deviceID, "reset_consumable", params)
}

func (c *Client) simpleCommand(ctx context.Context, deviceID, method string, params any) error {
	if err := c.ensureHomeData(ctx); err != nil {
		return err
	}
	device, err := c.deviceByID(deviceID)
	if err != nil {
		return err
	}
	channel, err := c.getLocalChannel(ctx, device)
	if err != nil {
		if c.cfg.CloudFallback {
			return fmt.Errorf("cloud fallback not implemented for command %s", method)
		}
		return err
	}
	_, err = c.sendRPC(ctx, channel, method, params)
	return err
}

// RawRPC sends a raw RPC to the device and returns the decoded result.
// This is intended for diagnostics and probing unsupported features.
func (c *Client) RawRPC(ctx context.Context, deviceID, method string, params any) (any, error) {
	if err := c.ensureHomeData(ctx); err != nil {
		return nil, err
	}
	device, err := c.deviceByID(deviceID)
	if err != nil {
		return nil, err
	}
	channel, err := c.getLocalChannel(ctx, device)
	if err != nil {
		return nil, err
	}
	return c.sendRPC(ctx, channel, method, params)
}

func (c *Client) sendRPC(ctx context.Context, channel *LocalChannel, method string, params any) (any, error) {
	if params == nil {
		params = []any{}
	}
	rpcCtx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()
	req := requestMessage{
		Method:    method,
		Params:    params,
		RequestID: nextInt(10000, 32767),
		Timestamp: nowTimestamp(),
	}
	msg := RoborockMessage{
		Version:  channel.ProtocolVersion(),
		Protocol: ProtocolGeneralReq,
	}
	var err error
	if msg.Version == LocalProtocolL01 {
		msg.Protocol = ProtocolRpcRequest
		msg.Payload, err = json.Marshal(req)
	} else {
		msg.Payload, err = encodeRequestPayload(req)
	}
	if err != nil {
		return nil, err
	}
	respCh := make(chan rpcResponse, 1)
	unsub := channel.Subscribe(func(respMsg RoborockMessage) {
		var resp rpcResponse
		switch respMsg.Protocol {
		case ProtocolGeneralReq, ProtocolGeneralResp:
			parsed, err := decodeResponsePayload(respMsg.Payload)
			if err != nil {
				return
			}
			resp = parsed
		case ProtocolRpcResponse:
			if len(respMsg.Payload) == 0 {
				return
			}
			if err := json.Unmarshal(respMsg.Payload, &resp); err != nil {
				return
			}
		default:
			return
		}
		if resp.RequestID != 0 && resp.RequestID != req.RequestID {
			return
		}
		respCh <- resp
	})
	defer unsub()

	if err := channel.Publish(rpcCtx, msg); err != nil {
		return nil, err
	}

	select {
	case <-rpcCtx.Done():
		return nil, rpcCtx.Err()
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, fmt.Errorf("device error: %v", resp.Error)
		}
		return resp.Result, nil
	}
}

func (c *Client) ensureHomeData(ctx context.Context) error {
	c.mu.Lock()
	if c.homeData != nil {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	home, err := c.api.GetHomeDataV3(ctx, c.userData)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.homeData = home
	c.devices = make(map[string]HomeDataDevice)
	for _, dev := range home.Devices {
		c.devices[dev.DUID] = dev
	}
	for _, dev := range home.ReceivedDevices {
		c.devices[dev.DUID] = dev
	}
	c.products = make(map[string]HomeDataProduct)
	for _, prod := range home.Products {
		c.products[prod.ID] = prod
	}
	return nil
}

func (c *Client) RefreshHomeData(ctx context.Context) error {
	home, err := c.api.GetHomeDataV3(ctx, c.userData)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.homeData = home
	c.devices = make(map[string]HomeDataDevice)
	for _, dev := range home.Devices {
		c.devices[dev.DUID] = dev
	}
	for _, dev := range home.ReceivedDevices {
		c.devices[dev.DUID] = dev
	}
	c.products = make(map[string]HomeDataProduct)
	for _, prod := range home.Products {
		c.products[prod.ID] = prod
	}
	return nil
}

func (c *Client) deviceByID(id string) (HomeDataDevice, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	dev, ok := c.devices[id]
	if !ok {
		return HomeDataDevice{}, fmt.Errorf("device %s not found", id)
	}
	return dev, nil
}

func (c *Client) getLocalChannel(ctx context.Context, device HomeDataDevice) (*LocalChannel, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	c.mu.Lock()
	if channel := c.channels[device.DUID]; channel != nil {
		c.mu.Unlock()
		return channel, nil
	}
	c.mu.Unlock()

	ip, err := c.deviceIP(ctx, device.DUID)
	if err != nil {
		return nil, err
	}
	channel := NewLocalChannel(ip, device.LocalKey, device.DUID)
	if err := channel.Connect(ctx); err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.channels[device.DUID] = channel
	c.mu.Unlock()

	return channel, nil
}

func (c *Client) deviceIP(ctx context.Context, deviceID string) (string, error) {
	c.mu.Lock()
	if override := c.overrides[deviceID]; override != "" {
		c.ipCache[deviceID] = override
		c.mu.Unlock()
		return override, nil
	}
	if ip := c.ipCache[deviceID]; ip != "" {
		c.mu.Unlock()
		return ip, nil
	}
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	msgs, err := DiscoverBroadcast(ctx, 3*time.Second)
	if err != nil {
		return "", err
	}
	for _, msg := range msgs {
		if msg.DUID == deviceID {
			c.mu.Lock()
			c.ipCache[deviceID] = msg.IP
			c.mu.Unlock()
			return msg.IP, nil
		}
	}
	return "", fmt.Errorf("device %s not found on broadcast", deviceID)
}

func parseStatus(statusData, consumable, cleanSummary any) Status {
	status := Status{}
	if statusDataMap, ok := normalizeMap(statusData); ok {
		stateValue := statusDataMap["state"]
		stateCode := intFrom(stateValue)
		if stateCode != 0 {
			status.State = stateName(stateCode)
		} else {
			status.State = stringFrom(stateValue)
		}
		status.BatteryPercent = intFrom(statusDataMap["battery"])
		status.ErrorCode = stringFrom(statusDataMap["error_code"])
		status.ErrorMessage = stringFrom(statusDataMap["error"])
		status.CleaningTimeSeconds = intFrom(statusDataMap["clean_time"])
		cleanArea := intFrom(statusDataMap["clean_area"])
		if cleanArea > 0 {
			status.CleaningAreaSquareMeters = float64(cleanArea) / 1000000
		}
		status.FanSpeed = stringFrom(statusDataMap["fan_power"])
		status.MopIntensity = stringFrom(statusDataMap["water_box_mode"])
		status.MopMode = stringFrom(statusDataMap["mop_mode"])
		status.WaterTankAttached = intFrom(statusDataMap["water_box_status"]) == 1
		status.MopAttached = intFrom(statusDataMap["water_box_carriage_status"]) == 1
		status.WaterShortage = intFrom(statusDataMap["water_shortage_status"]) == 1
		status.Charging = intFrom(statusDataMap["charge_status"]) == 1
	}
	if summaryMap, ok := normalizeMap(cleanSummary); ok {
		status.TotalCleaningTimeSeconds = intFrom(summaryMap["clean_time"])
		status.TotalCleaningCount = intFrom(summaryMap["clean_count"])
		area := intFrom(summaryMap["clean_area"])
		if area > 0 {
			status.TotalCleaningAreaSquareM = float64(area) / 1000000
		}
		if last, ok := summaryMap["last_clean_record"].(map[string]any); ok {
			status.LastCleanStart = timeFromUnix(last["begin"])
			status.LastCleanEnd = timeFromUnix(last["end"])
		}
	}
	_ = consumable
	return status
}

func parseStatusFromDeviceStatus(deviceStatus map[string]any) Status {
	status := Status{}
	if deviceStatus == nil {
		return status
	}
	status.State = stateName(intFrom(deviceStatus["121"]))
	status.BatteryPercent = intFrom(deviceStatus["122"])
	status.FanSpeed = stringFrom(deviceStatus["123"])
	status.MopIntensity = stringFrom(deviceStatus["124"])
	status.ErrorCode = stringFrom(deviceStatus["120"])
	status.Charging = intFrom(deviceStatus["133"]) == 1
	return status
}

func normalizeMap(value any) (map[string]any, bool) {
	switch v := value.(type) {
	case map[string]any:
		return v, true
	case []any:
		if len(v) > 0 {
			if item, ok := v[0].(map[string]any); ok {
				return item, true
			}
		}
	}
	return nil, false
}

func stringFrom(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprintf("%.0f", t)
	default:
		return ""
	}
}

func intFrom(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case string:
		i, _ := strconv.Atoi(t)
		return i
	default:
		return 0
	}
}

func timeFromUnix(v any) time.Time {
	switch t := v.(type) {
	case float64:
		return time.Unix(int64(t), 0)
	case int64:
		return time.Unix(t, 0)
	case int:
		return time.Unix(int64(t), 0)
	case string:
		val, _ := strconv.ParseInt(t, 10, 64)
		return time.Unix(val, 0)
	default:
		return time.Time{}
	}
}

func numericOrString(value string) any {
	if value == "" {
		return value
	}
	if i, err := strconv.Atoi(value); err == nil {
		return i
	}
	return value
}

func parseClock(value string) (int, int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid time format %q, expected HH:MM", value)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("invalid hour %q", parts[0])
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("invalid minute %q", parts[1])
	}
	return hour, minute, nil
}

func osReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
