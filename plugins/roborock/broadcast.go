package roborock

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"time"
)

type BroadcastMessage struct {
	DUID    string
	IP      string
	Version LocalProtocolVersion
}

func DiscoverBroadcast(ctx context.Context, timeout time.Duration) ([]BroadcastMessage, error) {
	addr := &net.UDPAddr{IP: net.IPv4zero, Port: 58866}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetReadDeadline(deadline)
	} else {
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
	}

	var out []BroadcastMessage
	buf := make([]byte, 2048)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				return out, nil
			}
			return out, err
		}
		msg, err := decodeBroadcast(buf[:n])
		if err != nil {
			continue
		}
		out = append(out, msg)
	}
}

func decodeBroadcast(data []byte) (BroadcastMessage, error) {
	if len(data) < 9 {
		return BroadcastMessage{}, errors.New("broadcast too short")
	}
	version := LocalProtocolVersion(string(data[:3]))
	if version == LocalProtocolL01 {
		return decodeBroadcastL01(data)
	}
	return decodeBroadcastV1(data)
}

func decodeBroadcastV1(data []byte) (BroadcastMessage, error) {
	if len(data) < 3+4+2+2+4 {
		return BroadcastMessage{}, errors.New("broadcast v1 too short")
	}
	payloadLen := binary.BigEndian.Uint16(data[3+4+2 : 3+4+2+2])
	payloadStart := 3 + 4 + 2 + 2
	payloadEnd := payloadStart + int(payloadLen)
	if payloadEnd+4 > len(data) {
		return BroadcastMessage{}, errors.New("broadcast v1 payload out of range")
	}
	payloadEnc := data[payloadStart:payloadEnd]
	key := []byte(broadcastToken)
	payload, err := aesEcbDecrypt(payloadEnc, key)
	if err != nil {
		return BroadcastMessage{}, err
	}
	var parsed map[string]any
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return BroadcastMessage{}, err
	}
	duid, _ := parsed["duid"].(string)
	ip, _ := parsed["ip"].(string)
	return BroadcastMessage{DUID: duid, IP: ip, Version: LocalProtocolVersion(string(data[:3]))}, nil
}

func decodeBroadcastL01(data []byte) (BroadcastMessage, error) {
	if len(data) < 3+4+2+2+4 {
		return BroadcastMessage{}, errors.New("broadcast l01 too short")
	}
	payloadLen := binary.BigEndian.Uint16(data[3+4+2 : 3+4+2+2])
	payloadStart := 3 + 4 + 2 + 2
	payloadEnd := payloadStart + int(payloadLen)
	if payloadEnd+4 > len(data) {
		return BroadcastMessage{}, errors.New("broadcast l01 payload out of range")
	}
	payloadEnc := data[payloadStart:payloadEnd]
	iv := sha256Bytes(data[:9])[:12]
	key := sha256Bytes([]byte(broadcastToken))
	payload, err := gcmDecrypt(key, iv, nil, payloadEnc)
	if err != nil {
		return BroadcastMessage{}, err
	}
	var parsed map[string]any
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return BroadcastMessage{}, err
	}
	duid, _ := parsed["duid"].(string)
	ip, _ := parsed["ip"].(string)
	return BroadcastMessage{DUID: duid, IP: ip, Version: LocalProtocolVersion(string(data[:3]))}, nil
}
