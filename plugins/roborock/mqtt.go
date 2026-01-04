package roborock

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type mqttClient struct {
	client mqtt.Client
	mu     sync.Mutex
	subs   map[string]map[int]func([]byte)
	nextID int
}

type mqttConfig struct {
	host     string
	port     int
	tls      bool
	username string
	password string
}

func newMQTTClient(cfg mqttConfig) (*mqttClient, error) {
	opts := mqtt.NewClientOptions()
	scheme := "tcp"
	if cfg.tls {
		scheme = "ssl"
		opts.SetTLSConfig(&tls.Config{})
	}
	opts.AddBroker(fmt.Sprintf("%s://%s:%d", scheme, cfg.host, cfg.port))
	opts.SetUsername(cfg.username)
	opts.SetPassword(cfg.password)
	opts.SetClientID(randomClientID())
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectTimeout(10 * time.Second)

	mc := &mqttClient{subs: make(map[string]map[int]func([]byte))}
	opts.SetDefaultPublishHandler(mc.dispatch)
	opts.OnConnect = func(_ mqtt.Client) {
		mc.resubscribeAll()
	}
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}
	mc.client = client
	return mc, nil
}

func (c *mqttClient) subscribe(topic string, cb func([]byte)) (func(), error) {
	c.mu.Lock()
	if c.subs[topic] == nil {
		c.subs[topic] = make(map[int]func([]byte))
	}
	id := c.nextID
	c.nextID++
	c.subs[topic][id] = cb
	needSubscribe := len(c.subs[topic]) == 1
	c.mu.Unlock()

	if needSubscribe {
		if token := c.client.Subscribe(topic, 0, nil); token.Wait() && token.Error() != nil {
			return nil, token.Error()
		}
	}

	return func() {
		c.mu.Lock()
		callbacks := c.subs[topic]
		if callbacks == nil {
			c.mu.Unlock()
			return
		}
		delete(callbacks, id)
		shouldUnsub := len(callbacks) == 0
		if shouldUnsub {
			delete(c.subs, topic)
		}
		c.mu.Unlock()
		if shouldUnsub {
			_ = c.client.Unsubscribe(topic).Wait()
		}
	}, nil
}

func (c *mqttClient) publish(topic string, payload []byte) error {
	if token := c.client.Publish(topic, 0, false, payload); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (c *mqttClient) dispatch(_ mqtt.Client, msg mqtt.Message) {
	c.mu.Lock()
	callbacks := c.subs[msg.Topic()]
	list := make([]func([]byte), 0, len(callbacks))
	for _, cb := range callbacks {
		list = append(list, cb)
	}
	c.mu.Unlock()
	for _, cb := range list {
		cb(msg.Payload())
	}
}

func (c *mqttClient) resubscribeAll() {
	c.mu.Lock()
	topics := make([]string, 0, len(c.subs))
	for topic := range c.subs {
		topics = append(topics, topic)
	}
	c.mu.Unlock()
	for _, topic := range topics {
		_ = c.client.Subscribe(topic, 0, nil).Wait()
	}
}

func (c *Client) mqttSession() (*mqttClient, error) {
	c.mu.Lock()
	if c.mqtt != nil {
		mc := c.mqtt
		c.mu.Unlock()
		return mc, nil
	}
	c.mu.Unlock()

	cfg, err := mqttConfigFromUserData(c.userData)
	if err != nil {
		return nil, err
	}
	mc, err := newMQTTClient(cfg)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.mqtt == nil {
		c.mqtt = mc
	}
	c.mu.Unlock()
	return mc, nil
}

func mqttConfigFromUserData(userData *UserData) (mqttConfig, error) {
	if userData == nil {
		return mqttConfig{}, errors.New("missing user data")
	}
	rawURL := userData.RRIOT.R.M
	if rawURL == "" {
		return mqttConfig{}, errors.New("missing rriot mqtt url")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return mqttConfig{}, err
	}
	if parsed.Hostname() == "" || parsed.Port() == "" {
		return mqttConfig{}, fmt.Errorf("invalid mqtt url %q", rawURL)
	}
	hashedUser := md5Hex([]byte(userData.RRIOT.U + ":" + userData.RRIOT.K))[2:10]
	hashedPass := md5Hex([]byte(userData.RRIOT.S + ":" + userData.RRIOT.K))[16:]
	port := 0
	_, _ = fmt.Sscanf(parsed.Port(), "%d", &port)
	if port == 0 {
		return mqttConfig{}, fmt.Errorf("invalid mqtt port %q", parsed.Port())
	}
	return mqttConfig{
		host:     parsed.Hostname(),
		port:     port,
		tls:      parsed.Scheme == "ssl",
		username: hashedUser,
		password: hashedPass,
	}, nil
}

func mqttTopics(userData *UserData, deviceID string, mqttUser string) (pub string, sub string) {
	return fmt.Sprintf("rr/m/i/%s/%s/%s", userData.RRIOT.U, mqttUser, deviceID),
		fmt.Sprintf("rr/m/o/%s/%s/%s", userData.RRIOT.U, mqttUser, deviceID)
}

type securityData struct {
	Endpoint string `json:"endpoint"`
	Nonce    string `json:"nonce"`
}

func newSecurityData(rriot RRiot) (securityData, []byte, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return securityData{}, nil, err
	}
	hash := md5Bytes([]byte(rriot.K))
	if len(hash) < 14 {
		return securityData{}, nil, errors.New("invalid md5 hash")
	}
	endpoint := base64.StdEncoding.EncodeToString(hash[8:14])
	return securityData{Endpoint: endpoint, Nonce: fmt.Sprintf("%x", nonce)}, nonce, nil
}

func (c *Client) fetchMapViaMQTT(ctx context.Context, device HomeDataDevice) ([]byte, error) {
	mqttSession, err := c.mqttSession()
	if err != nil {
		return nil, err
	}
	sec, nonce, err := newSecurityData(c.userData.RRIOT)
	if err != nil {
		return nil, err
	}
	req := requestMessage{
		Method:    "get_map_v1",
		Params:    []any{},
		RequestID: nextInt(10000, 32767),
		Timestamp: nowTimestamp(),
		Security:  &sec,
	}
	payload, err := encodeRequestPayload(req)
	if err != nil {
		return nil, err
	}
	msg := RoborockMessage{
		Version:  LocalProtocolV1,
		Protocol: ProtocolRpcRequest,
		Payload:  payload,
	}

	mqttUser := md5Hex([]byte(c.userData.RRIOT.U + ":" + c.userData.RRIOT.K))[2:10]
	pubTopic, subTopic := mqttTopics(c.userData, device.DUID, mqttUser)

	respCh := make(chan []byte, 1)
	errCh := make(chan error, 1)
	decoder := newMessageDecoder(device.LocalKey, 0, nil)

	unsub, err := mqttSession.subscribe(subTopic, func(data []byte) {
		messages, err := decoder.Feed(data)
		if err != nil {
			return
		}
		for _, message := range messages {
			switch message.Protocol {
			case ProtocolMapResponse:
				mapResp, err := decodeMapResponse(message.Payload, sec, nonce)
				if err != nil {
					errCh <- err
					return
				}
				if mapResp.RequestID == req.RequestID {
					respCh <- mapResp.Data
					return
				}
			case ProtocolRpcResponse, ProtocolGeneralResp:
				resp, err := decodeResponsePayload(message.Payload)
				if err != nil {
					continue
				}
				if resp.RequestID != req.RequestID {
					continue
				}
				if resp.Error != nil {
					errCh <- fmt.Errorf("device error: %v", resp.Error)
				} else {
					errCh <- fmt.Errorf("map request failed: %v", resp.Result)
				}
				return
			}
		}
	})
	if err != nil {
		return nil, err
	}
	defer unsub()

	frame, err := encodeMessageFrame(msg, device.LocalKey, 0, nil)
	if err != nil {
		return nil, err
	}
	if err := mqttSession.publish(pubTopic, frame); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errCh:
		return nil, err
	case data := <-respCh:
		return data, nil
	}
}

type mapResponse struct {
	RequestID int
	Data      []byte
}

func decodeMapResponse(payload []byte, sec securityData, nonce []byte) (mapResponse, error) {
	if len(payload) < 24 {
		return mapResponse{}, errors.New("invalid map response payload")
	}
	header := payload[:24]
	body := payload[24:]
	endpoint := string(header[:8])
	if !strings.HasPrefix(endpoint, sec.Endpoint) {
		return mapResponse{}, errors.New("map response endpoint mismatch")
	}
	reqID := int(int16le(header, 16))
	decrypted, err := aesCbcDecrypt(body, nonce)
	if err != nil {
		return mapResponse{}, err
	}
	decompressed, err := gzipDecompress(decrypted)
	if err != nil {
		return mapResponse{}, err
	}
	return mapResponse{RequestID: reqID, Data: decompressed}, nil
}

func randomClientID() string {
	nonce := make([]byte, 8)
	_, _ = rand.Read(nonce)
	return "gohome-" + base64.RawURLEncoding.EncodeToString(nonce)
}
