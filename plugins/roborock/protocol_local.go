package roborock

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

type LocalProtocolVersion string

const (
	LocalProtocolV1  LocalProtocolVersion = "1.0"
	LocalProtocolL01 LocalProtocolVersion = "L01"
)

type MessageProtocol uint16

const (
	ProtocolHelloRequest  MessageProtocol = 0
	ProtocolHelloResponse MessageProtocol = 1
	ProtocolPingRequest   MessageProtocol = 2
	ProtocolPingResponse  MessageProtocol = 3
	ProtocolGeneralReq    MessageProtocol = 4
	ProtocolGeneralResp   MessageProtocol = 5
	ProtocolRpcRequest    MessageProtocol = 101
	ProtocolRpcResponse   MessageProtocol = 102
)

type RoborockMessage struct {
	Version   LocalProtocolVersion
	Seq       uint32
	Random    uint32
	Timestamp uint32
	Protocol  MessageProtocol
	Payload   []byte
}

type messageDecoder struct {
	localKey     string
	connectNonce uint32
	ackNonce     *uint32
	buffer       []byte
}

func newMessageDecoder(localKey string, connectNonce uint32, ackNonce *uint32) *messageDecoder {
	return &messageDecoder{localKey: localKey, connectNonce: connectNonce, ackNonce: ackNonce}
}

func (d *messageDecoder) Feed(data []byte) ([]RoborockMessage, error) {
	d.buffer = append(d.buffer, data...)
	var messages []RoborockMessage
	minFrameLen := 3 + 4 + 4 + 4 + 2
	minFrameLenWithLen := minFrameLen + 2 + 4
	validVersions := map[string]struct{}{
		string(LocalProtocolV1):  {},
		string(LocalProtocolL01): {},
		"A01":                    {},
		"B01":                    {},
	}
	for {
		if len(d.buffer) >= 3 {
			if _, ok := validVersions[string(d.buffer[:3])]; ok {
				if len(d.buffer) < minFrameLen {
					return messages, nil
				}
				frameLen := 0
				if len(d.buffer) >= minFrameLenWithLen {
					payloadLen := int(binary.BigEndian.Uint16(d.buffer[17:19]))
					candidate := minFrameLenWithLen + payloadLen
					if len(d.buffer) >= candidate {
						frameLen = candidate
					}
				}
				if frameLen == 0 {
					frameLen = minFrameLen
					if len(d.buffer) < frameLen {
						return messages, nil
					}
				}
				frame := d.buffer[:frameLen]
				msg, err := decodeMessage(frame, d.localKey, d.connectNonce, d.ackNonce)
				if err != nil {
					d.buffer = d.buffer[1:]
					return messages, err
				}
				d.buffer = d.buffer[frameLen:]
				messages = append(messages, msg)
				continue
			}
		}
		if len(d.buffer) < 4 {
			return messages, nil
		}
		length := binary.BigEndian.Uint32(d.buffer[:4])
		if length == 0 || int(length)+4 > len(d.buffer) {
			return messages, nil
		}
		if int(length) < minFrameLen {
			d.buffer = d.buffer[1:]
			continue
		}
		frame := d.buffer[4 : 4+length]
		if _, ok := validVersions[string(frame[:3])]; !ok {
			d.buffer = d.buffer[1:]
			continue
		}
		msg, err := decodeMessage(frame, d.localKey, d.connectNonce, d.ackNonce)
		if err != nil {
			d.buffer = d.buffer[1:]
			return messages, err
		}
		d.buffer = d.buffer[4+length:]
		messages = append(messages, msg)
	}
}

func encodeMessage(msg RoborockMessage, localKey string, connectNonce uint32, ackNonce *uint32) ([]byte, error) {
	if msg.Timestamp == 0 {
		msg.Timestamp = nowTimestamp()
	}
	if msg.Seq == 0 {
		msg.Seq = uint32(nextInt(100000, 999999))
	}
	if msg.Random == 0 {
		msg.Random = uint32(nextInt(10000, 99999))
	}

	payload, err := encryptPayload(msg, localKey, connectNonce, ackNonce)
	if err != nil {
		return nil, err
	}

	buf := &bytes.Buffer{}
	buf.Write([]byte(msg.Version))
	_ = binary.Write(buf, binary.BigEndian, msg.Seq)
	_ = binary.Write(buf, binary.BigEndian, msg.Random)
	_ = binary.Write(buf, binary.BigEndian, msg.Timestamp)
	_ = binary.Write(buf, binary.BigEndian, uint16(msg.Protocol))
	if payload == nil {
		_ = binary.Write(buf, binary.BigEndian, uint16(0))
	} else {
		_ = binary.Write(buf, binary.BigEndian, uint16(len(payload)))
		buf.Write(payload)
	}
	checksum := crc32sum(buf.Bytes())
	_ = binary.Write(buf, binary.BigEndian, checksum)
	frame := buf.Bytes()

	out := &bytes.Buffer{}
	_ = binary.Write(out, binary.BigEndian, uint32(len(frame)))
	out.Write(frame)
	return out.Bytes(), nil
}

func decodeMessage(frame []byte, localKey string, connectNonce uint32, ackNonce *uint32) (RoborockMessage, error) {
	minHeaderLen := 3 + 4 + 4 + 4 + 2
	if len(frame) < minHeaderLen {
		return RoborockMessage{}, errors.New("frame too short")
	}
	if len(frame) == minHeaderLen {
		version := LocalProtocolVersion(string(frame[:3]))
		seq := binary.BigEndian.Uint32(frame[3:7])
		random := binary.BigEndian.Uint32(frame[7:11])
		timestamp := binary.BigEndian.Uint32(frame[11:15])
		proto := binary.BigEndian.Uint16(frame[15:17])
		return RoborockMessage{
			Version:   version,
			Seq:       seq,
			Random:    random,
			Timestamp: timestamp,
			Protocol:  MessageProtocol(proto),
			Payload:   nil,
		}, nil
	}
	if len(frame) == minHeaderLen+4 {
		version := LocalProtocolVersion(string(frame[:3]))
		seq := binary.BigEndian.Uint32(frame[3:7])
		random := binary.BigEndian.Uint32(frame[7:11])
		timestamp := binary.BigEndian.Uint32(frame[11:15])
		proto := binary.BigEndian.Uint16(frame[15:17])
		checksum := binary.BigEndian.Uint32(frame[17:21])
		if checksum != 0 && crc32sum(frame[:17]) != checksum {
			return RoborockMessage{}, errors.New("checksum mismatch")
		}
		return RoborockMessage{
			Version:   version,
			Seq:       seq,
			Random:    random,
			Timestamp: timestamp,
			Protocol:  MessageProtocol(proto),
			Payload:   nil,
		}, nil
	}
	checksumOffset := len(frame) - 4
	data := frame[:checksumOffset]
	checksum := binary.BigEndian.Uint32(frame[checksumOffset:])
	if checksum != 0 && crc32sum(data) != checksum {
		return RoborockMessage{}, errors.New("checksum mismatch")
	}

	reader := bytes.NewReader(data)
	versionBytes := make([]byte, 3)
	if _, err := reader.Read(versionBytes); err != nil {
		return RoborockMessage{}, err
	}
	version := LocalProtocolVersion(string(versionBytes))
	var seq, random, timestamp uint32
	if err := binary.Read(reader, binary.BigEndian, &seq); err != nil {
		return RoborockMessage{}, err
	}
	if err := binary.Read(reader, binary.BigEndian, &random); err != nil {
		return RoborockMessage{}, err
	}
	if err := binary.Read(reader, binary.BigEndian, &timestamp); err != nil {
		return RoborockMessage{}, err
	}
	var proto uint16
	if err := binary.Read(reader, binary.BigEndian, &proto); err != nil {
		return RoborockMessage{}, err
	}
	var payloadEnc []byte
	if remaining := reader.Len(); remaining == 4 {
		// Some devices omit the 2-byte payload length when payload is empty.
		payloadEnc = nil
	} else {
		var payloadLen uint16
		if err := binary.Read(reader, binary.BigEndian, &payloadLen); err != nil {
			return RoborockMessage{}, err
		}
		if payloadLen > 0 {
			payloadEnc = make([]byte, payloadLen)
			if _, err := reader.Read(payloadEnc); err != nil {
				return RoborockMessage{}, err
			}
		}
	}

	payload, err := decryptPayload(version, payloadEnc, localKey, timestamp, seq, random, connectNonce, ackNonce)
	if err != nil {
		return RoborockMessage{}, fmt.Errorf("decrypt payload: %w", err)
	}

	return RoborockMessage{
		Version:   version,
		Seq:       seq,
		Random:    random,
		Timestamp: timestamp,
		Protocol:  MessageProtocol(proto),
		Payload:   payload,
	}, nil
}

func encryptPayload(msg RoborockMessage, localKey string, connectNonce uint32, ackNonce *uint32) ([]byte, error) {
	if len(msg.Payload) == 0 {
		return nil, nil
	}
	if msg.Version == LocalProtocolL01 {
		return encryptL01Payload(msg.Payload, localKey, msg.Timestamp, msg.Seq, msg.Random, connectNonce, ackNonce)
	}
	key := md5Bytes(append(append(encodeTimestamp(msg.Timestamp), []byte(localKey)...), []byte(roborockSalt)...))
	return aesEcbEncrypt(msg.Payload, key)
}

func decryptPayload(version LocalProtocolVersion, payload []byte, localKey string, timestamp, seq, random, connectNonce uint32, ackNonce *uint32) ([]byte, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	if version == LocalProtocolL01 {
		return decryptL01Payload(payload, localKey, timestamp, seq, random, connectNonce, ackNonce)
	}
	key := md5Bytes(append(append(encodeTimestamp(timestamp), []byte(localKey)...), []byte(roborockSalt)...))
	return aesEcbDecrypt(payload, key)
}

func encryptL01Payload(payload []byte, localKey string, timestamp, sequence, nonce, connectNonce uint32, ackNonce *uint32) ([]byte, error) {
	key := l01Key(localKey, timestamp)
	iv := l01IV(timestamp, nonce, sequence)
	var aad []byte
	if ackNonce != nil {
		aad = l01AAD(timestamp, nonce, sequence, connectNonce, *ackNonce)
	} else {
		aad = l01AAD(timestamp, nonce, sequence, connectNonce, 0)
	}
	ciphertext, err := gcmEncrypt(key, iv, aad, payload)
	if err != nil {
		return nil, err
	}
	return ciphertext, nil
}

func decryptL01Payload(payload []byte, localKey string, timestamp, sequence, nonce, connectNonce uint32, ackNonce *uint32) ([]byte, error) {
	var ack uint32
	if ackNonce != nil {
		ack = *ackNonce
	}
	key := l01Key(localKey, timestamp)
	iv := l01IV(timestamp, nonce, sequence)
	aad := l01AAD(timestamp, nonce, sequence, connectNonce, ack)
	return gcmDecrypt(key, iv, aad, payload)
}

func l01Key(localKey string, timestamp uint32) []byte {
	input := append(append(encodeTimestamp(timestamp), []byte(localKey)...), []byte(roborockSalt)...)
	return sha256Bytes(input)
}

func l01IV(timestamp, nonce, sequence uint32) []byte {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint32(buf[0:4], sequence)
	binary.BigEndian.PutUint32(buf[4:8], nonce)
	binary.BigEndian.PutUint32(buf[8:12], timestamp)
	return sha256Bytes(buf)[:12]
}

func l01AAD(timestamp, nonce, sequence, connectNonce, ackNonce uint32) []byte {
	buf := &bytes.Buffer{}
	_ = binary.Write(buf, binary.BigEndian, sequence)
	_ = binary.Write(buf, binary.BigEndian, connectNonce)
	if ackNonce != 0 {
		_ = binary.Write(buf, binary.BigEndian, ackNonce)
	}
	_ = binary.Write(buf, binary.BigEndian, nonce)
	_ = binary.Write(buf, binary.BigEndian, timestamp)
	return buf.Bytes()
}
