package cluster

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mrzack99s/cocodb"
	"github.com/mrzack99s/cocodb/queue"
)

// ServerConfig configures a cluster node server.
type ServerConfig struct {
	NodeID      string
	Addr        string
	Secret      string        // Authentication secret token
	TLSConfig   *tls.Config   // Optional mTLS configuration
	ReadTimeout time.Duration
	Listener    net.Listener  // Optional custom in-memory listener
}

// Server handles cluster RPC connections from clients and peer nodes.
type Server struct {
	cfg       ServerConfig
	db        *cocodb.DB
	listener  net.Listener
	ring      *HashRing
	peers     map[string]*Client // Peer node RPC client pool for forwarding
	peersMu   sync.RWMutex
	conns     map[net.Conn]bool
	connsMu   sync.Mutex
	running   atomic.Bool
	closed    chan struct{}
}

// NewServer creates a new cluster node server.
func NewServer(db *cocodb.DB, ring *HashRing, cfg ServerConfig) *Server {
	if cfg.NodeID == "" {
		cfg.NodeID = fmt.Sprintf("node_%s", cfg.Addr)
	}
	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = 30 * time.Second
	}
	return &Server{
		cfg:     cfg,
		db:      db,
		ring:    ring,
		peers:   make(map[string]*Client),
		conns:   make(map[net.Conn]bool),
		closed:  make(chan struct{}),
	}
}

// Start begins listening on the configured network address.
func (s *Server) Start() error {
	if s.cfg.Listener != nil {
		s.listener = s.cfg.Listener
		s.running.Store(true)
		go s.acceptLoop()
		return nil
	}

	var l net.Listener
	var err error

	if s.cfg.TLSConfig != nil {
		l, err = tls.Listen("tcp", s.cfg.Addr, s.cfg.TLSConfig)
	} else {
		l, err = net.Listen("tcp", s.cfg.Addr)
	}
	if err != nil {
		return err
	}

	s.listener = l
	s.running.Store(true)

	go s.acceptLoop()
	return nil
}

// Addr returns the listener's network address.
func (s *Server) Addr() net.Addr {
	if s.listener != nil {
		return s.listener.Addr()
	}
	return nil
}

func (s *Server) acceptLoop() {
	for s.running.Load() {
		conn, err := s.listener.Accept()
		if err != nil {
			if !s.running.Load() {
				return
			}
			continue
		}

		s.connsMu.Lock()
		s.conns[conn] = true
		s.connsMu.Unlock()

		if s.cfg.Listener != nil && s.cfg.TLSConfig != nil {
			conn = tls.Server(conn, s.cfg.TLSConfig)
		}

		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer func() {
		_ = conn.Close()
		s.connsMu.Lock()
		delete(s.conns, conn)
		s.connsMu.Unlock()
	}()

	authenticated := s.cfg.Secret == "" // If no secret configured, open auth

	for s.running.Load() {
		_ = conn.SetReadDeadline(time.Now().Add(s.cfg.ReadTimeout))
		frame, err := DecodeFrame(conn)
		if err != nil {
			if err != io.EOF && s.running.Load() {
				// Socket closed or read timeout
			}
			return
		}

		// First step: Handle Authentication
		if !authenticated {
			if frame.Cmd != CmdAuth {
				_ = EncodeFrame(conn, &Frame{
					Cmd:           CmdResponse,
					Status:        StatusUnauthorized,
					CorrelationID: frame.CorrelationID,
					Payload:       []byte("authentication required before commands"),
				})
				return
			}

			var authReq struct {
				Secret string `json:"secret"`
				NodeID string `json:"node_id"`
			}
			_ = json.Unmarshal(frame.Payload, &authReq)

			if authReq.Secret != s.cfg.Secret {
				_ = EncodeFrame(conn, &Frame{
					Cmd:           CmdResponse,
					Status:        StatusUnauthorized,
					CorrelationID: frame.CorrelationID,
					Payload:       []byte("invalid cluster secret token"),
				})
				return
			}

			authenticated = true
			_ = EncodeFrame(conn, &Frame{
				Cmd:           CmdResponse,
				Status:        StatusOK,
				CorrelationID: frame.CorrelationID,
				Payload:       []byte("authenticated"),
			})
			continue
		}

		// Authenticated command dispatch
		s.dispatch(conn, frame)
	}
}

func (s *Server) dispatch(conn net.Conn, f *Frame) {
	switch f.Cmd {
	case CmdPing:
		_ = EncodeFrame(conn, &Frame{
			Cmd:           CmdResponse,
			Status:        StatusOK,
			CorrelationID: f.CorrelationID,
			Payload:       []byte("PONG"),
		})

	case CmdEnqueue:
		s.handleEnqueue(conn, f)

	case CmdDequeue:
		s.handleDequeue(conn, f)

	case CmdAck:
		s.handleAck(conn, f)

	case CmdNack:
		s.handleNack(conn, f)

	case CmdPublish:
		s.handlePublish(conn, f)

	default:
		_ = EncodeFrame(conn, &Frame{
			Cmd:           CmdResponse,
			Status:        StatusError,
			CorrelationID: f.CorrelationID,
			Payload:       []byte(fmt.Sprintf("unknown command: %d", f.Cmd)),
		})
	}
}

// Request and response payloads
type EnqueueReq struct {
	Queue    string `json:"queue"`
	Payload  []byte `json:"payload"`
	DedupID  string `json:"dedup_id,omitempty"`
	Priority uint8  `json:"priority,omitempty"`
	DelayMs  int64  `json:"delay_ms,omitempty"`
}

type EnqueueResp struct {
	MessageID string `json:"message_id"`
	Queue     string `json:"queue"`
	State     string `json:"state"`
}

func (s *Server) handleEnqueue(conn net.Conn, f *Frame) {
	var req EnqueueReq
	if err := json.Unmarshal(f.Payload, &req); err != nil {
		s.replyError(conn, f.CorrelationID, StatusError, err.Error())
		return
	}

	// Distributed Deduplication Routing Check:
	// If DedupID is present and hash ring routes to a peer node, forward to primary owner!
	if req.DedupID != "" && s.ring != nil {
		ownerNode, ok := s.ring.GetNode(req.DedupID)
		if ok && ownerNode != s.cfg.NodeID && ownerNode != s.cfg.Addr {
			// Forward to owner node via peer client
			peerClient := s.getPeer(ownerNode)
			if peerClient != nil {
				resp, err := peerClient.rawRequest(f)
				if err == nil {
					_ = EncodeFrame(conn, resp)
					return
				}
			}
		}
	}

	q := s.db.Queue(req.Queue)
	var opts []queue.Option
	if req.DedupID != "" {
		opts = append(opts, queue.WithDedupID(req.DedupID, 5*time.Minute))
	}
	if req.Priority > 0 {
		opts = append(opts, queue.WithPriority(req.Priority))
	}
	if req.DelayMs > 0 {
		opts = append(opts, queue.WithDelay(time.Duration(req.DelayMs)*time.Millisecond))
	}

	msg, err := q.Enqueue(context.Background(), req.Payload, opts...)
	if err != nil {
		if err == queue.ErrDuplicateMessage {
			s.replyError(conn, f.CorrelationID, StatusDuplicate, err.Error())
			return
		}
		s.replyError(conn, f.CorrelationID, StatusError, err.Error())
		return
	}

	respBytes, _ := json.Marshal(EnqueueResp{
		MessageID: msg.ID,
		Queue:     msg.Queue,
		State:     msg.State.String(),
	})

	_ = EncodeFrame(conn, &Frame{
		Cmd:           CmdResponse,
		Status:        StatusOK,
		CorrelationID: f.CorrelationID,
		Payload:       respBytes,
	})
}

type DequeueReq struct {
	Queue        string `json:"queue"`
	AutoAck      bool   `json:"auto_ack"`
	VisibilityMs int64  `json:"visibility_ms"`
}

type DequeueResp struct {
	Found        bool   `json:"found"`
	MessageID    string `json:"message_id,omitempty"`
	Queue        string `json:"queue,omitempty"`
	Payload      []byte `json:"payload,omitempty"`
	RetryCount   int    `json:"retry_count,omitempty"`
	Priority     uint8  `json:"priority,omitempty"`
	State        string `json:"state,omitempty"`
}

func (s *Server) handleDequeue(conn net.Conn, f *Frame) {
	var req DequeueReq
	if err := json.Unmarshal(f.Payload, &req); err != nil {
		s.replyError(conn, f.CorrelationID, StatusError, err.Error())
		return
	}

	q := s.db.Queue(req.Queue)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var opts []queue.Option
	if req.VisibilityMs > 0 {
		opts = append(opts, queue.WithVisibilityTimeout(time.Duration(req.VisibilityMs)*time.Millisecond))
	}

	msg, err := q.Dequeue(ctx, opts...)
	if err != nil {
		if err == context.DeadlineExceeded || err == queue.ErrQueueEmpty {
			respBytes, _ := json.Marshal(DequeueResp{Found: false})
			_ = EncodeFrame(conn, &Frame{
				Cmd:           CmdResponse,
				Status:        StatusOK,
				CorrelationID: f.CorrelationID,
				Payload:       respBytes,
			})
			return
		}
		s.replyError(conn, f.CorrelationID, StatusError, err.Error())
		return
	}

	if req.AutoAck {
		_ = msg.Ack()
	}

	respBytes, _ := json.Marshal(DequeueResp{
		Found:      true,
		MessageID:  msg.ID,
		Queue:      msg.Queue,
		Payload:    msg.Payload,
		RetryCount: msg.RetryCount,
		Priority:   msg.Priority,
		State:      msg.State.String(),
	})

	_ = EncodeFrame(conn, &Frame{
		Cmd:           CmdResponse,
		Status:        StatusOK,
		CorrelationID: f.CorrelationID,
		Payload:       respBytes,
	})
}

type AckReq struct {
	Queue     string `json:"queue"`
	MessageID string `json:"message_id"`
}

func (s *Server) handleAck(conn net.Conn, f *Frame) {
	var req AckReq
	if err := json.Unmarshal(f.Payload, &req); err != nil {
		s.replyError(conn, f.CorrelationID, StatusError, err.Error())
		return
	}

	q := s.db.Queue(req.Queue)
	err := q.Ack(req.MessageID)
	if err != nil {
		s.replyError(conn, f.CorrelationID, StatusError, err.Error())
		return
	}

	_ = EncodeFrame(conn, &Frame{
		Cmd:           CmdResponse,
		Status:        StatusOK,
		CorrelationID: f.CorrelationID,
		Payload:       []byte("OK"),
	})
}

type NackReq struct {
	Queue     string `json:"queue"`
	MessageID string `json:"message_id"`
	Requeue   bool   `json:"requeue"`
}

func (s *Server) handleNack(conn net.Conn, f *Frame) {
	var req NackReq
	if err := json.Unmarshal(f.Payload, &req); err != nil {
		s.replyError(conn, f.CorrelationID, StatusError, err.Error())
		return
	}

	q := s.db.Queue(req.Queue)
	err := q.Nack(req.MessageID, req.Requeue)
	if err != nil {
		s.replyError(conn, f.CorrelationID, StatusError, err.Error())
		return
	}

	_ = EncodeFrame(conn, &Frame{
		Cmd:           CmdResponse,
		Status:        StatusOK,
		CorrelationID: f.CorrelationID,
		Payload:       []byte("OK"),
	})
}

type PublishReq struct {
	Topic   string `json:"topic"`
	Payload []byte `json:"payload"`
	DedupID string `json:"dedup_id,omitempty"`
}

func (s *Server) handlePublish(conn net.Conn, f *Frame) {
	var req PublishReq
	if err := json.Unmarshal(f.Payload, &req); err != nil {
		s.replyError(conn, f.CorrelationID, StatusError, err.Error())
		return
	}

	count, err := s.db.Publish(context.Background(), req.Topic, req.Payload)
	if err != nil {
		s.replyError(conn, f.CorrelationID, StatusError, err.Error())
		return
	}

	_ = EncodeFrame(conn, &Frame{
		Cmd:           CmdResponse,
		Status:        StatusOK,
		CorrelationID: f.CorrelationID,
		Payload:       []byte(fmt.Sprintf("%d", count)),
	})
}

func (s *Server) replyError(conn net.Conn, corrID uint32, status uint8, msg string) {
	_ = EncodeFrame(conn, &Frame{
		Cmd:           CmdResponse,
		Status:        status,
		CorrelationID: corrID,
		Payload:       []byte(msg),
	})
}

func (s *Server) getPeer(addr string) *Client {
	s.peersMu.RLock()
	client, exists := s.peers[addr]
	s.peersMu.RUnlock()
	if exists {
		return client
	}

	s.peersMu.Lock()
	defer s.peersMu.Unlock()
	if client, exists := s.peers[addr]; exists {
		return client
	}

	newClient := NewClient(ClientConfig{
		Addrs:       []string{addr},
		Secret:      s.cfg.Secret,
		TLSConfig:   s.cfg.TLSConfig,
		DialTimeout: 2 * time.Second,
	})
	s.peers[addr] = newClient
	return newClient
}

// Close gracefully stops the server and closes all active client connections.
func (s *Server) Close() error {
	if !s.running.CompareAndSwap(true, false) {
		return nil
	}
	close(s.closed)

	if s.listener != nil {
		_ = s.listener.Close()
	}

	s.connsMu.Lock()
	for conn := range s.conns {
		_ = conn.Close()
	}
	s.conns = make(map[net.Conn]bool)
	s.connsMu.Unlock()

	s.peersMu.Lock()
	for _, p := range s.peers {
		_ = p.Close()
	}
	s.peers = make(map[string]*Client)
	s.peersMu.Unlock()

	return nil
}

// PeerStatus represents the health and latency of a peer cluster node.
type PeerStatus struct {
	Addr      string `json:"addr"`
	Status    string `json:"status"` // "online", "unreachable", "local"
	LatencyUs int64  `json:"latency_us"`
	IsLocal   bool   `json:"is_local"`
}

// ClusterStatus represents the complete live status of the cluster node.
type ClusterStatus struct {
	Enabled      bool         `json:"enabled"`
	NodeID       string       `json:"node_id"`
	Addr         string       `json:"addr"`
	TLSEnabled   bool         `json:"tls_enabled"`
	mTLSEnforced bool         `json:"mtls_enforced"`
	AuthEnforced bool         `json:"auth_enforced"`
	TotalNodes   int          `json:"total_nodes"`
	VirtualNodes int          `json:"virtual_nodes"`
	ActiveConns  int          `json:"active_conns"`
	Peers        []PeerStatus `json:"peers"`
}

// Status returns the live cluster health and peer status.
func (s *Server) Status() ClusterStatus {
	ringNodes := s.ring.Members()
	vNodes := s.ring.VirtualNodes()

	s.connsMu.Lock()
	activeConns := len(s.conns)
	s.connsMu.Unlock()

	peers := make([]PeerStatus, 0, len(ringNodes))
	for _, nodeAddr := range ringNodes {
		if nodeAddr == s.cfg.Addr {
			peers = append(peers, PeerStatus{
				Addr:      nodeAddr,
				Status:    "online (local node)",
				LatencyUs: 0,
				IsLocal:   true,
			})
			continue
		}

		// Ping peer with short timeout
		client := s.getPeer(nodeAddr)
		t0 := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		err := client.Ping(ctx)
		cancel()
		latencyUs := time.Since(t0).Microseconds()
		status := "online"
		if err != nil {
			status = "unreachable"
			latencyUs = -1
		}

		peers = append(peers, PeerStatus{
			Addr:      nodeAddr,
			Status:    status,
			LatencyUs: latencyUs,
			IsLocal:   false,
		})
	}

	return ClusterStatus{
		Enabled:      s.running.Load(),
		NodeID:       s.cfg.NodeID,
		Addr:         s.cfg.Addr,
		TLSEnabled:   s.cfg.TLSConfig != nil,
		mTLSEnforced: s.cfg.TLSConfig != nil && s.cfg.TLSConfig.ClientAuth == tls.RequireAndVerifyClientCert,
		AuthEnforced: s.cfg.Secret != "",
		TotalNodes:   len(ringNodes),
		VirtualNodes: vNodes,
		ActiveConns:  activeConns,
		Peers:        peers,
	}
}
