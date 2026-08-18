package cluster

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// ClientConfig configures a cluster client.
type ClientConfig struct {
	Addrs       []string      // List of seed node addresses
	Secret      string        // Cluster authentication secret token
	TLSConfig   *tls.Config   // Optional mTLS config
	DialTimeout time.Duration
	Dialer      func(ctx context.Context, network, addr string) (net.Conn, error)
}

// Client manages connections to a CoCoDB cluster with consistent hash routing.
type Client struct {
	cfg     ClientConfig
	ring    *HashRing
	conns   map[string]net.Conn
	mu      sync.RWMutex
	nextID  atomic.Uint32
	closed  bool
}

// NewClient creates a new cluster client.
func NewClient(cfg ClientConfig) *Client {
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 3 * time.Second
	}
	ring := NewHashRing(DefaultVirtualNodes)
	for _, addr := range cfg.Addrs {
		ring.AddNode(addr)
	}

	return &Client{
		cfg:   cfg,
		ring:  ring,
		conns: make(map[string]net.Conn),
	}
}

func (c *Client) getConn(addr string) (net.Conn, error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, errors.New("coco/cluster: client is closed")
	}
	conn, exists := c.conns[addr]
	c.mu.RUnlock()

	if exists {
		return conn, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, errors.New("coco/cluster: client is closed")
	}
	if conn, exists := c.conns[addr]; exists {
		return conn, nil
	}

	var rawConn net.Conn
	var err error
	if c.cfg.Dialer != nil {
		rawConn, err = c.cfg.Dialer(context.Background(), "tcp", addr)
		if err == nil && c.cfg.TLSConfig != nil {
			rawConn = tls.Client(rawConn, c.cfg.TLSConfig)
		}
	} else if c.cfg.TLSConfig != nil {
		rawConn, err = tls.DialWithDialer(&net.Dialer{Timeout: c.cfg.DialTimeout}, "tcp", addr, c.cfg.TLSConfig)
	} else {
		rawConn, err = net.DialTimeout("tcp", addr, c.cfg.DialTimeout)
	}
	if err != nil {
		return nil, err
	}

	// If secret configured, perform authentication handshake
	if c.cfg.Secret != "" {
		authPayload, _ := json.Marshal(map[string]string{
			"secret": c.cfg.Secret,
		})
		authFrame := &Frame{
			Cmd:           CmdAuth,
			CorrelationID: c.nextID.Add(1),
			Payload:       authPayload,
		}
		if err := EncodeFrame(rawConn, authFrame); err != nil {
			_ = rawConn.Close()
			return nil, err
		}

		resp, err := DecodeFrame(rawConn)
		if err != nil {
			_ = rawConn.Close()
			return nil, err
		}
		if resp.Status != StatusOK {
			_ = rawConn.Close()
			if resp.Status == StatusUnauthorized {
				return nil, ErrUnauthorized
			}
			return nil, fmt.Errorf("coco/cluster auth failed: %s", string(resp.Payload))
		}
	}

	c.conns[addr] = rawConn
	return rawConn, nil
}

func (c *Client) routeNode(key string) string {
	if node, ok := c.ring.GetNode(key); ok {
		return node
	}
	if len(c.cfg.Addrs) > 0 {
		return c.cfg.Addrs[0]
	}
	return "127.0.0.1:9001"
}

func (c *Client) rawRequest(f *Frame) (*Frame, error) {
	node := ""
	if len(c.cfg.Addrs) > 0 {
		node = c.cfg.Addrs[0]
	} else {
		node = "127.0.0.1:9001"
	}
	return c.requestNode(node, f)
}

func (c *Client) requestNode(addr string, f *Frame) (*Frame, error) {
	conn, err := c.getConn(addr)
	if err != nil {
		return nil, err
	}

	if f.CorrelationID == 0 {
		f.CorrelationID = c.nextID.Add(1)
	}

	if err := EncodeFrame(conn, f); err != nil {
		// Drop bad connection and retry once
		c.mu.Lock()
		_ = conn.Close()
		delete(c.conns, addr)
		c.mu.Unlock()
		return nil, err
	}

	resp, err := DecodeFrame(conn)
	if err != nil {
		c.mu.Lock()
		_ = conn.Close()
		delete(c.conns, addr)
		c.mu.Unlock()
		return nil, err
	}

	return resp, nil
}

// Ping checks cluster connectivity.
func (c *Client) Ping(ctx context.Context) error {
	addr := c.routeNode("ping")
	resp, err := c.requestNode(addr, &Frame{Cmd: CmdPing})
	if err != nil {
		return err
	}
	if resp.Status != StatusOK {
		return fmt.Errorf("ping error: %s", string(resp.Payload))
	}
	return nil
}

// Enqueue enqueues a task to the cluster with automatic deduplication routing.
func (c *Client) Enqueue(ctx context.Context, req EnqueueReq) (*EnqueueResp, error) {
	routeKey := req.Queue
	if req.DedupID != "" {
		routeKey = req.DedupID
	}
	addr := c.routeNode(routeKey)

	payloadBytes, _ := json.Marshal(req)
	resp, err := c.requestNode(addr, &Frame{
		Cmd:     CmdEnqueue,
		Payload: payloadBytes,
	})
	if err != nil {
		return nil, err
	}

	if resp.Status != StatusOK {
		if resp.Status == StatusDuplicate {
			return nil, errors.New("coco/queue: duplicate message detected within deduplication window")
		}
		if resp.Status == StatusUnauthorized {
			return nil, ErrUnauthorized
		}
		return nil, errors.New(string(resp.Payload))
	}

	var res EnqueueResp
	if err := json.Unmarshal(resp.Payload, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// Dequeue pulls a task from the cluster.
func (c *Client) Dequeue(ctx context.Context, req DequeueReq) (*DequeueResp, error) {
	primary := c.routeNode(req.Queue)
	payloadBytes, _ := json.Marshal(req)

	resp, err := c.requestNode(primary, &Frame{
		Cmd:     CmdDequeue,
		Payload: payloadBytes,
	})
	if err == nil && resp.Status == StatusOK {
		var res DequeueResp
		if err := json.Unmarshal(resp.Payload, &res); err == nil && res.Found {
			return &res, nil
		}
	} else if err != nil {
		return nil, err
	}

	// If primary returned empty or if sharded across nodes, check other cluster nodes
	for _, addr := range c.cfg.Addrs {
		if addr == primary {
			continue
		}
		resp, err := c.requestNode(addr, &Frame{
			Cmd:     CmdDequeue,
			Payload: payloadBytes,
		})
		if err == nil && resp.Status == StatusOK {
			var res DequeueResp
			if err := json.Unmarshal(resp.Payload, &res); err == nil && res.Found {
				return &res, nil
			}
		}
	}

	return &DequeueResp{Found: false}, nil
}

// Ack acknowledges completion of a task.
func (c *Client) Ack(ctx context.Context, queueName, msgID string) error {
	payloadBytes, _ := json.Marshal(AckReq{Queue: queueName, MessageID: msgID})
	frame := &Frame{
		Cmd:     CmdAck,
		Payload: payloadBytes,
	}

	for _, addr := range c.cfg.Addrs {
		resp, err := c.requestNode(addr, frame)
		if err == nil && resp.Status == StatusOK {
			return nil
		}
	}
	return nil
}

// Nack rejects an in-flight task.
func (c *Client) Nack(ctx context.Context, queueName, msgID string, requeue bool) error {
	payloadBytes, _ := json.Marshal(NackReq{Queue: queueName, MessageID: msgID, Requeue: requeue})
	frame := &Frame{
		Cmd:     CmdNack,
		Payload: payloadBytes,
	}

	for _, addr := range c.cfg.Addrs {
		resp, err := c.requestNode(addr, frame)
		if err == nil && resp.Status == StatusOK {
			return nil
		}
	}
	return nil
}

// Publish broadcasts an event to a topic.
func (c *Client) Publish(ctx context.Context, topic string, payload []byte, dedupID string) (int, error) {
	routeKey := topic
	if dedupID != "" {
		routeKey = dedupID
	}
	addr := c.routeNode(routeKey)

	payloadBytes, _ := json.Marshal(PublishReq{Topic: topic, Payload: payload, DedupID: dedupID})
	resp, err := c.requestNode(addr, &Frame{
		Cmd:     CmdPublish,
		Payload: payloadBytes,
	})
	if err != nil {
		return 0, err
	}

	if resp.Status != StatusOK {
		if resp.Status == StatusDuplicate {
			return 0, errors.New("coco/pubsub: duplicate message detected within deduplication window")
		}
		return 0, errors.New(string(resp.Payload))
	}

	count, _ := strconv.Atoi(string(resp.Payload))
	return count, nil
}

// Close closes all pooled node connections.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	for _, conn := range c.conns {
		_ = conn.Close()
	}
	c.conns = make(map[string]net.Conn)
	return nil
}
