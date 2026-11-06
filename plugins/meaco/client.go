package meaco

import (
	"bytes"
	"context"
	"crypto/aes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	meacov1 "github.com/joshp123/gohome/proto/gen/plugins/meaco/v1"
	"hash/crc32"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

// Each operation opens a connection; the next scrape retries after any outage.
// Serialize requests because Tuya devices often support only one local client.
type Client struct {
	config      Config
	operation   sync.Mutex
	healthMutex sync.Mutex
	lastRead    time.Time
	lastError   error
}

func (c *Client) Status(ctx context.Context) (*meacov1.DeviceStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c.operation.Lock()
	defer c.operation.Unlock()
	payload, err := c.exchange(ctx, 10, nil)
	// Some 3.3 firmware requires CONTROL_NEW with null-valued DPS for reads.
	if err == nil && bytes.Contains(payload, []byte("data unvalid")) {
		payload, err = c.exchange(ctx, 13, map[string]any{"1": nil, "2": nil, "4": nil, "14": nil, "16": nil, "17": nil, "19": nil, "101": nil})
	}
	var state *meacov1.DeviceStatus
	if err == nil {
		state, err = decodeStatus(payload)
	}
	c.healthMutex.Lock()
	c.lastError = err
	if err == nil {
		c.lastRead = time.Now()
	}
	c.healthMutex.Unlock()
	return state, err
}

func decodeStatus(payload []byte) (*meacov1.DeviceStatus, error) {
	var envelope struct {
		DPS map[string]json.RawMessage `json:"dps"`
	}
	if json.Unmarshal(payload, &envelope) != nil || len(envelope.DPS) == 0 {
		return nil, fmt.Errorf("meaco response contains no DPS")
	}
	state := &meacov1.DeviceStatus{ObservedAtUnix: time.Now().Unix()}
	fields := map[string]any{"1": &state.Power, "2": &state.TargetHumidityPercent, "4": &state.Mode, "14": &state.ChildLock, "16": &state.HumidityPercent, "17": &state.Countdown, "19": &state.FaultCode, "101": &state.OnTimer}
	present := false
	for id, dest := range fields {
		if raw, ok := envelope.DPS[id]; ok && string(raw) != "null" {
			if json.Unmarshal(raw, dest) != nil {
				return nil, fmt.Errorf("invalid type for meaco DPS %s", id)
			}
			present = true
		}
	}
	if !present {
		return nil, fmt.Errorf("meaco response contains no recognized state")
	}
	return state, nil
}

func (c *Client) Set(ctx context.Context, id string, value any) error {
	c.operation.Lock()
	defer c.operation.Unlock()
	_, err := c.exchange(ctx, 7, map[string]any{id: value})
	return err
}

// Tuya 3.3: AES-128 ECB/PKCS7 payload, 55aa frame, CRC32 and aa55 suffix.
// Wire reference: github.com/jasonacox/tinytuya, core/XenonDevice.py.
func (c *Client) exchange(ctx context.Context, command uint32, dps map[string]any) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(c.config.IP, "6668"))
	if err != nil {
		return nil, fmt.Errorf("connect meaco: %w", err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, err
	}
	request := map[string]any{"devId": c.config.DeviceID, "uid": c.config.DeviceID, "t": strconv.FormatInt(time.Now().Unix(), 10)}
	if command == 10 {
		request["gwId"] = c.config.DeviceID
	}
	if dps != nil {
		request["dps"] = dps
	}
	plain, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	cipher, err := aes.NewCipher([]byte(c.config.LocalKey))
	if err != nil {
		return nil, fmt.Errorf("invalid meaco encryption key")
	}
	padding := aes.BlockSize - len(plain)%aes.BlockSize
	plain = append(plain, bytes.Repeat([]byte{byte(padding)}, padding)...)
	encrypted := make([]byte, len(plain))
	for offset := 0; offset < len(plain); offset += aes.BlockSize {
		cipher.Encrypt(encrypted[offset:offset+aes.BlockSize], plain[offset:offset+aes.BlockSize])
	}
	if command != 10 {
		encrypted = append(append([]byte("3.3"), make([]byte, 12)...), encrypted...)
	}
	frame := make([]byte, 16+len(encrypted)+8)
	binary.BigEndian.PutUint32(frame, 0x55aa)
	binary.BigEndian.PutUint32(frame[4:], 1)
	binary.BigEndian.PutUint32(frame[8:], command)
	binary.BigEndian.PutUint32(frame[12:], uint32(len(encrypted)+8))
	copy(frame[16:], encrypted)
	binary.BigEndian.PutUint32(frame[len(frame)-8:], crc32.ChecksumIEEE(frame[:len(frame)-8]))
	binary.BigEndian.PutUint32(frame[len(frame)-4:], 0xaa55)
	if _, err = io.Copy(conn, bytes.NewReader(frame)); err != nil {
		return nil, err
	}
	for {
		header := make([]byte, 16)
		if _, err = io.ReadFull(conn, header); err != nil {
			return nil, fmt.Errorf("read meaco: %w", err)
		}
		length := binary.BigEndian.Uint32(header[12:])
		if binary.BigEndian.Uint32(header) != 0x55aa || length < 12 || length > 65536 {
			return nil, fmt.Errorf("invalid meaco frame")
		}
		body := make([]byte, int(length))
		if _, err = io.ReadFull(conn, body); err != nil {
			return nil, err
		}
		check := crc32.NewIEEE()
		check.Write(header)
		check.Write(body[:len(body)-8])
		if check.Sum32() != binary.BigEndian.Uint32(body[len(body)-8:]) || binary.BigEndian.Uint32(body[len(body)-4:]) != 0xaa55 {
			return nil, fmt.Errorf("invalid meaco frame checksum")
		}
		if binary.BigEndian.Uint32(body) != 0 {
			return nil, fmt.Errorf("meaco rejected request")
		}
		payload := body[4 : len(body)-8]
		responseCommand := binary.BigEndian.Uint32(header[8:])
		if len(payload) == 0 {
			if command == 7 && responseCommand == 7 {
				return nil, nil
			}
			continue
		}
		if bytes.HasPrefix(payload, []byte("3.3")) && len(payload) >= 15 {
			payload = payload[15:]
		}
		if len(payload) == 0 || len(payload)%aes.BlockSize != 0 {
			return nil, fmt.Errorf("invalid encrypted meaco payload")
		}
		for offset := 0; offset < len(payload); offset += aes.BlockSize {
			cipher.Decrypt(payload[offset:offset+aes.BlockSize], payload[offset:offset+aes.BlockSize])
		}
		padding := int(payload[len(payload)-1])
		if padding < 1 || padding > aes.BlockSize || !bytes.Equal(payload[len(payload)-padding:], bytes.Repeat([]byte{byte(padding)}, padding)) {
			return nil, fmt.Errorf("cannot decrypt meaco response; check local key")
		}
		payload = payload[:len(payload)-padding]
		if command == 7 {
			if responseCommand != 7 {
				continue
			}
			if len(payload) > 0 && !json.Valid(payload) {
				return nil, fmt.Errorf("meaco rejected control")
			}
		}
		return payload, nil
	}
}
