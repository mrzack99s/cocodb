package queue

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mrzack99s/cocodb/internal/btree"
	"github.com/mrzack99s/cocodb/internal/storage"
)

// Options for configuring an internal Queue instance.
type Config struct {
	DefaultVisibilityTimeout time.Duration
	DefaultMaxRetries        int
	DedupWindow              time.Duration
	EnableDLQ                bool
}

func DefaultConfig() Config {
	return Config{
		DefaultVisibilityTimeout: 30 * time.Second,
		DefaultMaxRetries:        5,
		DedupWindow:              10 * time.Minute,
		EnableDLQ:                true,
	}
}

// QueueEngine manages message queuing, priority scheduling, visibility timeouts, and DLQ.
type QueueEngine struct {
	mu           sync.Mutex
	name         string
	cfg          Config
	readyHeap    priorityHeap
	inFlight     map[string]*Message
	dlq          *QueueEngine
	dedup        *Deduplicator
	tree         *btree.BTree
	pager        storage.Pager
	notifyCh     chan struct{}
	msgSeq       atomic.Uint64
	closed       bool
	stopWorkerCh chan struct{}
}

// NewQueueEngine creates a new queue engine instance.
func NewQueueEngine(name string, cfg Config, tree *btree.BTree, pager storage.Pager, isDLQ bool) *QueueEngine {
	if cfg.DefaultVisibilityTimeout <= 0 {
		cfg.DefaultVisibilityTimeout = 30 * time.Second
	}
	if cfg.DefaultMaxRetries <= 0 {
		cfg.DefaultMaxRetries = 5
	}
	if cfg.DedupWindow <= 0 {
		cfg.DedupWindow = 10 * time.Minute
	}

	q := &QueueEngine{
		name:         name,
		cfg:          cfg,
		readyHeap:    make(priorityHeap, 0),
		inFlight:     make(map[string]*Message),
		dedup:        NewDeduplicator(),
		tree:         tree,
		pager:        pager,
		notifyCh:     make(chan struct{}, 1),
		stopWorkerCh: make(chan struct{}),
	}
	heap.Init(&q.readyHeap)

	// Create DLQ if enabled and not already DLQ
	if cfg.EnableDLQ && !isDLQ {
		dlqCfg := cfg
		dlqCfg.EnableDLQ = false
		q.dlq = NewQueueEngine(name+".dlq", dlqCfg, nil, pager, true)
	}

	// Restore messages from persistent B+Tree if provided
	if tree != nil {
		q.restoreFromTree()
	}

	// Start visibility timeout checker background goroutine
	go q.visibilityWorker()

	return q
}

func (q *QueueEngine) Name() string {
	return q.name
}

func (q *QueueEngine) DLQ() *QueueEngine {
	return q.dlq
}

func (q *QueueEngine) notify() {
	select {
	case q.notifyCh <- struct{}{}:
	default:
	}
}

// Enqueue adds a new message to the queue.
func (q *QueueEngine) Enqueue(msg *Message) (*Message, error) {
	if msg == nil {
		return nil, errors.New("coco/queue: nil message")
	}

	now := time.Now().UnixNano()

	// Check deduplication
	if msg.DedupID != "" {
		if !q.dedup.CheckOrSet(msg.DedupID, q.cfg.DedupWindow) {
			return nil, ErrDuplicateMessage
		}
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return nil, errors.New("coco/queue: queue closed")
	}

	if msg.ID == "" {
		seq := q.msgSeq.Add(1)
		msg.ID = fmt.Sprintf("msg_%d_%d", now, seq)
	}
	msg.Queue = q.name
	msg.CreatedAt = now
	msg.State = StateReady

	if msg.MaxRetries <= 0 {
		msg.MaxRetries = q.cfg.DefaultMaxRetries
	}
	if msg.VisibleAt <= 0 {
		msg.VisibleAt = now
	}

	// Bind runtime callbacks
	q.bindCallbacks(msg)

	// Persist to B+Tree if tree is available
	if q.tree != nil {
		k := []byte(msg.ID)
		v := msg.Encode()
		_ = q.tree.Insert(k, v)
	}

	heap.Push(&q.readyHeap, msg)
	q.notify()

	return msg, nil
}

// Dequeue retrieves the next visible message from the queue.
func (q *QueueEngine) Dequeue(ctx context.Context, visibilityTimeout time.Duration) (*Message, error) {
	if visibilityTimeout <= 0 {
		visibilityTimeout = q.cfg.DefaultVisibilityTimeout
	}

	for {
		q.mu.Lock()
		if q.closed {
			q.mu.Unlock()
			return nil, errors.New("coco/queue: queue closed")
		}

		now := time.Now().UnixNano()

		// Scan ready heap for next visible message
		for q.readyHeap.Len() > 0 {
			msg := q.readyHeap[0]

			// Check TTL expiration
			if msg.ExpireAt > 0 && msg.ExpireAt <= now {
				heap.Pop(&q.readyHeap)
				if q.tree != nil {
					_ = q.tree.Delete([]byte(msg.ID))
				}
				continue
			}

			// Check scheduled delay / visibility
			if msg.VisibleAt > now {
				// Next message is delayed; stop looking in ready heap
				break
			}

			// Pop ready message
			heap.Pop(&q.readyHeap)

			// Mark as in-flight / invisible
			msg.State = StateInvisible
			msg.RetryCount++
			msg.VisibleAt = now + int64(visibilityTimeout)
			q.inFlight[msg.ID] = msg

			q.bindCallbacks(msg)

			// Update persistent store
			if q.tree != nil {
				_ = q.tree.Insert([]byte(msg.ID), msg.Encode())
			}

			q.mu.Unlock()
			return msg, nil
		}
		q.mu.Unlock()

		// No visible message ready -> wait for notification or context cancel
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-q.notifyCh:
			// Woken up by new enqueue or visibility timer
			continue
		}
	}
}

func (q *QueueEngine) bindCallbacks(msg *Message) {
	id := msg.ID
	msg.SetCallbacks(
		func() error {
			return q.Ack(id)
		},
		func(requeue bool) error {
			return q.Nack(id, requeue)
		},
		func(d time.Duration) error {
			return q.Extend(id, d)
		},
	)
}

// Ack marks a message as completed and removes it.
func (q *QueueEngine) Ack(msgID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	msg, inFlight := q.inFlight[msgID]
	if !inFlight {
		return ErrMessageNotFound
	}

	delete(q.inFlight, msgID)
	msg.State = StateCompleted

	// Remove from persistent B+Tree
	if q.tree != nil {
		_ = q.tree.Delete([]byte(msgID))
	}

	return nil
}

// Nack rejects an in-flight message.
func (q *QueueEngine) Nack(msgID string, requeue bool) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	msg, inFlight := q.inFlight[msgID]
	if !inFlight {
		return ErrMessageNotFound
	}

	delete(q.inFlight, msgID)

	if !requeue || msg.RetryCount >= msg.MaxRetries {
		// Route to DLQ if enabled
		msg.State = StateDeadLetter
		if q.tree != nil {
			_ = q.tree.Delete([]byte(msgID))
		}
		if q.dlq != nil {
			msg.VisibleAt = time.Now().UnixNano()
			msg.DedupID = ""
			_, _ = q.dlq.Enqueue(msg)
		}
		return ErrMaxRetriesExceeded
	}

	// Requeue immediately
	msg.State = StateReady
	msg.VisibleAt = time.Now().UnixNano()
	heap.Push(&q.readyHeap, msg)

	if q.tree != nil {
		_ = q.tree.Insert([]byte(msgID), msg.Encode())
	}

	q.notify()
	return nil
}

// Extend extends visibility timeout lease for an in-flight message.
func (q *QueueEngine) Extend(msgID string, d time.Duration) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	msg, inFlight := q.inFlight[msgID]
	if !inFlight {
		return ErrMessageNotFound
	}

	msg.VisibleAt = time.Now().Add(d).UnixNano()
	if q.tree != nil {
		_ = q.tree.Insert([]byte(msgID), msg.Encode())
	}
	return nil
}

// Stats returns runtime queue statistics.
type Stats struct {
	ReadyCount     int
	InFlightCount  int
	DLQCount       int
}

func (q *QueueEngine) Stats() Stats {
	q.mu.Lock()
	defer q.mu.Unlock()

	dlqCount := 0
	if q.dlq != nil {
		dlqStats := q.dlq.Stats()
		dlqCount = dlqStats.ReadyCount + dlqStats.InFlightCount
	}

	return Stats{
		ReadyCount:    q.readyHeap.Len(),
		InFlightCount: len(q.inFlight),
		DLQCount:      dlqCount,
	}
}

// Close gracefully stops the queue workers.
func (q *QueueEngine) Close() error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return nil
	}
	q.closed = true
	close(q.stopWorkerCh)
	q.mu.Unlock()

	if q.dlq != nil {
		_ = q.dlq.Close()
	}
	return nil
}

func (q *QueueEngine) visibilityWorker() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-q.stopWorkerCh:
			return
		case <-ticker.C:
			q.checkInFlightAndDelayed()
		}
	}
}

func (q *QueueEngine) checkInFlightAndDelayed() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return
	}

	now := time.Now().UnixNano()
	hasChanges := false

	// Check expired in-flight messages (worker crash / unacknowledged lease expiry)
	for id, msg := range q.inFlight {
		if msg.VisibleAt <= now {
			delete(q.inFlight, id)

			if msg.RetryCount >= msg.MaxRetries {
				msg.State = StateDeadLetter
				if q.tree != nil {
					_ = q.tree.Delete([]byte(id))
				}
				if q.dlq != nil {
					msg.VisibleAt = now
					msg.DedupID = ""
					_, _ = q.dlq.Enqueue(msg)
				}
			} else {
				// Re-insert into ready queue
				msg.State = StateReady
				msg.VisibleAt = now
				heap.Push(&q.readyHeap, msg)
				if q.tree != nil {
					_ = q.tree.Insert([]byte(id), msg.Encode())
				}
				hasChanges = true
			}
		}
	}

	if hasChanges {
		q.notify()
	}
}

func (q *QueueEngine) restoreFromTree() {
	cur := btree.NewCursor(q.tree)
	defer cur.Close()

	if !cur.First() {
		return
	}

	for cur.Valid() {
		val := cur.Value()
		msg, err := DecodeMessage(val)
		if err == nil {
			if msg.State == StateInvisible {
				// Restore uncompleted in-flight message as ready on restart
				msg.State = StateReady
				msg.VisibleAt = time.Now().UnixNano()
			}
			heap.Push(&q.readyHeap, msg)
		}
		if !cur.Next() {
			break
		}
	}
}

// priorityHeap implements heap.Interface ordered by Priority DESC, then VisibleAt ASC.
type priorityHeap []*Message

func (h priorityHeap) Len() int { return len(h) }
func (h priorityHeap) Less(i, j int) bool {
	// Higher priority first
	if h[i].Priority != h[j].Priority {
		return h[i].Priority > h[j].Priority
	}
	// Earlier visible time first
	return h[i].VisibleAt < h[j].VisibleAt
}
func (h priorityHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *priorityHeap) Push(x any)        { *h = append(*h, x.(*Message)) }
func (h *priorityHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
