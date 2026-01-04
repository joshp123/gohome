package roborock

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

const (
	localPort      = 58867
	localPingEvery = 10 * time.Second
	localTimeout   = 5 * time.Second
)

type LocalChannel struct {
	host         string
	localKey     string
	deviceID     string
	protocol     LocalProtocolVersion
	connectNonce uint32
	ackNonce     *uint32

	conn    net.Conn
	decoder *messageDecoder
	mu      sync.Mutex

	subscribers []func(RoborockMessage)
	closed      chan struct{}
}

func NewLocalChannel(host, localKey, deviceID string) *LocalChannel {
	return &LocalChannel{
		host:         host,
		localKey:     localKey,
		deviceID:     deviceID,
		protocol:     LocalProtocolV1,
		connectNonce: uint32(nextInt(10000, 32767)),
		closed:       make(chan struct{}),
	}
}

func (c *LocalChannel) ProtocolVersion() LocalProtocolVersion {
	return c.protocol
}

func (c *LocalChannel) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.conn != nil {
		c.mu.Unlock()
		return nil
	}

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", c.host, localPort))
	if err != nil {
		c.mu.Unlock()
		return err
	}
	c.conn = conn
	c.decoder = newMessageDecoder(c.localKey, c.connectNonce, c.ackNonce)
	c.mu.Unlock()
	go c.readLoop()

	if err := c.hello(ctx); err != nil {
		c.mu.Lock()
		c.closeLocked()
		c.mu.Unlock()
		return err
	}
	go c.keepAlive()
	return nil
}

func (c *LocalChannel) hello(ctx context.Context) error {
	attempts := []LocalProtocolVersion{LocalProtocolV1, LocalProtocolL01}
	for _, version := range attempts {
		c.protocol = version
		resp, err := c.sendRaw(ctx, RoborockMessage{
			Version:  version,
			Protocol: ProtocolHelloRequest,
			Seq:      1,
			Random:   c.connectNonce,
		}, ProtocolHelloResponse)
		if err != nil {
			continue
		}
		ack := resp.Random
		c.ackNonce = &ack
		c.decoder = newMessageDecoder(c.localKey, c.connectNonce, c.ackNonce)
		return nil
	}
	return errors.New("local hello failed")
}

func (c *LocalChannel) keepAlive() {
	ticker := time.NewTicker(localPingEvery)
	defer ticker.Stop()
	for {
		select {
		case <-c.closed:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), localTimeout)
			_ = c.sendPing(ctx)
			cancel()
		}
	}
}

func (c *LocalChannel) sendPing(ctx context.Context) error {
	_, err := c.sendRaw(ctx, RoborockMessage{
		Version:  c.protocol,
		Protocol: ProtocolPingRequest,
	}, ProtocolPingResponse)
	return err
}

func (c *LocalChannel) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := c.conn.Read(buf)
		if err != nil {
			c.close()
			return
		}
		messages, err := c.decoder.Feed(buf[:n])
		if err != nil {
			continue
		}
		c.mu.Lock()
		subs := append([]func(RoborockMessage){}, c.subscribers...)
		c.mu.Unlock()
		for _, msg := range messages {
			for _, cb := range subs {
				cb(msg)
			}
		}
	}
}

func (c *LocalChannel) Subscribe(cb func(RoborockMessage)) func() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.subscribers = append(c.subscribers, cb)
	idx := len(c.subscribers) - 1
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if idx < 0 || idx >= len(c.subscribers) {
			return
		}
		c.subscribers = append(c.subscribers[:idx], c.subscribers[idx+1:]...)
	}
}

func (c *LocalChannel) Publish(ctx context.Context, msg RoborockMessage) error {
	c.mu.Lock()
	conn := c.conn
	localKey := c.localKey
	connectNonce := c.connectNonce
	ackNonce := c.ackNonce
	c.mu.Unlock()
	if conn == nil {
		return errors.New("local channel not connected")
	}
	payload, err := encodeMessage(msg, localKey, connectNonce, ackNonce)
	if err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetWriteDeadline(deadline); err != nil {
			return err
		}
	} else {
		if err := conn.SetWriteDeadline(time.Now().Add(localTimeout)); err != nil {
			return err
		}
	}
	_, err = conn.Write(payload)
	_ = conn.SetWriteDeadline(time.Time{})
	return err
}

func (c *LocalChannel) sendRaw(ctx context.Context, msg RoborockMessage, responseProtocol MessageProtocol) (RoborockMessage, error) {
	if msg.Seq == 0 {
		msg.Seq = uint32(nextInt(100000, 999999))
	}
	respCh := make(chan RoborockMessage, 1)
	unsub := c.Subscribe(func(resp RoborockMessage) {
		if resp.Protocol == responseProtocol && resp.Seq == msg.Seq {
			respCh <- resp
		}
	})
	defer unsub()
	if err := c.Publish(ctx, msg); err != nil {
		return RoborockMessage{}, err
	}
	select {
	case <-ctx.Done():
		return RoborockMessage{}, ctx.Err()
	case resp := <-respCh:
		return resp, nil
	}
}

func (c *LocalChannel) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLocked()
}

func (c *LocalChannel) closeLocked() {
	select {
	case <-c.closed:
		return
	default:
		close(c.closed)
	}
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

func (c *LocalChannel) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLocked()
}
