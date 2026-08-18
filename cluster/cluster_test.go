package cluster_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	coco "cocodb"
	"cocodb/cluster"
)

// In-memory pipe network router for sandbox-safe unit testing
type pipeListener struct {
	addr   string
	ch     chan net.Conn
	closed atomic.Bool
}

func newPipeListener(addr string) *pipeListener {
	return &pipeListener{
		addr: addr,
		ch:   make(chan net.Conn, 256),
	}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	conn, ok := <-l.ch
	if !ok {
		return nil, net.ErrClosed
	}
	return conn, nil
}

func (l *pipeListener) Close() error {
	if l.closed.CompareAndSwap(false, true) {
		close(l.ch)
	}
	return nil
}

func (l *pipeListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9000}
}

type memNetwork struct {
	mu        sync.RWMutex
	listeners map[string]*pipeListener
}

func newMemNetwork() *memNetwork {
	return &memNetwork{
		listeners: make(map[string]*pipeListener),
	}
}

func (m *memNetwork) Listen(addr string) *pipeListener {
	m.mu.Lock()
	defer m.mu.Unlock()
	l := newPipeListener(addr)
	m.listeners[addr] = l
	return l
}

func (m *memNetwork) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	m.mu.RLock()
	l, exists := m.listeners[addr]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("node %s not found in network", addr)
	}

	c1, c2 := net.Pipe()
	select {
	case l.ch <- c2:
		return c1, nil
	case <-time.After(1 * time.Second):
		_ = c1.Close()
		_ = c2.Close()
		return nil, errors.New("connection accept timeout")
	}
}

func setupMemNode(t *testing.T, netw *memNetwork, addr string, secret string, peers []string, nodeOpt cluster.NodeOption) (*coco.DB, *cluster.Node) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, fmt.Sprintf("node_%d.coco", time.Now().UnixNano()))
	db, err := coco.Open(dbPath, coco.Profile(coco.Balanced))
	if err != nil {
		t.Fatalf("Open DB failed: %v", err)
	}

	l := netw.Listen(addr)
	var opts []cluster.NodeOption
	opts = append(opts, cluster.WithListener(l))
	if secret != "" {
		opts = append(opts, cluster.WithSecret(secret))
	}
	if len(peers) > 0 {
		opts = append(opts, cluster.WithPeers(peers...))
	}
	if nodeOpt != nil {
		opts = append(opts, nodeOpt)
	}

	node, err := cluster.StartNode(db, addr, opts...)
	if err != nil {
		_ = db.Close()
		t.Fatalf("StartNode failed: %v", err)
	}

	return db, node
}

func TestCluster_PingAndAuth(t *testing.T) {
	netw := newMemNetwork()
	addr := "node-1:9001"
	secret := "cluster_super_secret_key_123"

	db, node := setupMemNode(t, netw, addr, secret, nil, nil)
	defer db.Close()
	defer node.Close()

	ctx := context.Background()

	// 1. Connect with valid secret
	client, err := cluster.Dial([]string{addr},
		cluster.WithSecret(secret),
		cluster.WithDialer(netw.Dial),
	)
	if err != nil {
		t.Fatalf("Dial with secret failed: %v", err)
	}
	defer client.Close()

	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestCluster_AuthRejection(t *testing.T) {
	netw := newMemNetwork()
	addr := "node-auth:9001"
	secret := "correct_cluster_token_999"

	db, node := setupMemNode(t, netw, addr, secret, nil, nil)
	defer db.Close()
	defer node.Close()

	ctx := context.Background()

	// Connect with invalid secret -> Ping must fail with unauthorized
	badClient, err := cluster.Dial([]string{addr},
		cluster.WithSecret("wrong_token"),
		cluster.WithDialer(netw.Dial),
	)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer badClient.Close()

	err = badClient.Ping(ctx)
	if err == nil {
		t.Fatalf("expected ping with wrong token to fail with unauthorized error")
	}
}

func TestCluster_mTLSEncryption(t *testing.T) {
	netw := newMemNetwork()
	addr := "node-tls:9001"
	nodeOpt, clientOpt := cluster.WithDevmTLS("127.0.0.1", "localhost")

	db, node := setupMemNode(t, netw, addr, "tls_secret", nil, nodeOpt)
	defer db.Close()
	defer node.Close()

	ctx := context.Background()

	// Connect using mTLS 1.3
	client, err := cluster.Dial([]string{addr},
		cluster.WithSecret("tls_secret"),
		clientOpt,
		cluster.WithDialer(netw.Dial),
	)
	if err != nil {
		t.Fatalf("Dial with mTLS failed: %v", err)
	}
	defer client.Close()

	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping over mTLS failed: %v", err)
	}

	// Queue operations over mTLS
	q := client.Queue("secure_orders")
	msg, err := q.Enqueue(ctx, []byte(`{"order_id": "secure_01"}`))
	if err != nil || msg == nil {
		t.Fatalf("Enqueue over mTLS failed: %v", err)
	}

	dequeued, err := q.Dequeue(ctx)
	if err != nil || dequeued == nil {
		t.Fatalf("Dequeue over mTLS failed: %v", err)
	}
	if err := dequeued.Ack(); err != nil {
		t.Fatalf("Ack over mTLS failed: %v", err)
	}
}

func TestCluster_DistributedQueue_Deduplication(t *testing.T) {
	netw := newMemNetwork()
	addr1 := "node-1:9001"
	addr2 := "node-2:9002"
	addr3 := "node-3:9003"
	secret := "cluster_dedup_secret_tok"

	peers := []string{addr1, addr2, addr3}

	db1, node1 := setupMemNode(t, netw, addr1, secret, peers, nil)
	defer db1.Close()
	defer node1.Close()

	db2, node2 := setupMemNode(t, netw, addr2, secret, peers, nil)
	defer db2.Close()
	defer node2.Close()

	db3, node3 := setupMemNode(t, netw, addr3, secret, peers, nil)
	defer db3.Close()
	defer node3.Close()

	ctx := context.Background()

	// Client connected to all 3 nodes
	client, err := cluster.Dial(peers,
		cluster.WithSecret(secret),
		cluster.WithDialer(netw.Dial),
	)
	if err != nil {
		t.Fatalf("Dial cluster failed: %v", err)
	}
	defer client.Close()

	q := client.Queue("distributed_payments")
	dedupID := "payment_txn_99881122"

	// First Enqueue with DedupID -> MUST SUCCEED
	msg, err := q.Enqueue(ctx, []byte(`{"amount": 500, "currency": "USD"}`), coco.WithDedupID(dedupID, 5*time.Minute))
	if err != nil || msg == nil {
		t.Fatalf("Initial Enqueue failed: %v", err)
	}

	// Concurrent duplicate enqueues from multiple clients across all nodes
	var wg sync.WaitGroup
	duplicateRejectionCount := 0
	var countMu sync.Mutex

	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			_, err := q.Enqueue(ctx, []byte(fmt.Sprintf("duplicate payload from %d", workerID)), coco.WithDedupID(dedupID, 5*time.Minute))
			if err != nil {
				countMu.Lock()
				duplicateRejectionCount++
				countMu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if duplicateRejectionCount != 15 {
		t.Fatalf("expected all 15 duplicate attempts to be rejected, got %d", duplicateRejectionCount)
	}

	// Dequeue task and verify payload
	task, err := q.Dequeue(ctx)
	if err != nil || task == nil {
		t.Fatalf("Dequeue task failed: %v", err)
	}
	if string(task.Payload) != `{"amount": 500, "currency": "USD"}` {
		t.Fatalf("unexpected task payload: %s", string(task.Payload))
	}
	if err := task.Ack(); err != nil {
		t.Fatalf("Ack failed: %v", err)
	}
}

func TestCluster_PubSubBroadcast(t *testing.T) {
	netw := newMemNetwork()
	addr := "node-ps:9001"
	db, node := setupMemNode(t, netw, addr, "ps_secret", nil, nil)
	defer db.Close()
	defer node.Close()

	ctx := context.Background()

	// Local direct subscription on DB
	sub := db.Subscribe(ctx, "cluster.events")
	defer sub.Unsubscribe()

	// Client publishes over cluster RPC
	client, err := cluster.Dial([]string{addr},
		cluster.WithSecret("ps_secret"),
		cluster.WithDialer(netw.Dial),
	)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	ps := client.PubSub()
	count, err := ps.Publish(ctx, "cluster.events", []byte("distributed event broadcast payload"))
	if err != nil || count != 1 {
		t.Fatalf("Publish failed: count=%d, err=%v", count, err)
	}

	select {
	case m := <-sub.Channel():
		if string(m.Payload) != "distributed event broadcast payload" {
			t.Fatalf("received unexpected payload: %s", string(m.Payload))
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timeout waiting for subscriber to receive broadcast")
	}
}
