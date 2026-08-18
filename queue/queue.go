package queue

import (
	"context"
	"time"

	internalQueue "cocodb/internal/queue"
)

// Re-export common types and errors for developer convenience.
type Message = internalQueue.Message
type MessageState = internalQueue.MessageState

const (
	StateReady      = internalQueue.StateReady
	StateInvisible  = internalQueue.StateInvisible
	StateCompleted  = internalQueue.StateCompleted
	StateDeadLetter = internalQueue.StateDeadLetter
)

var (
	ErrDuplicateMessage   = internalQueue.ErrDuplicateMessage
	ErrQueueEmpty         = internalQueue.ErrQueueEmpty
	ErrMessageNotFound    = internalQueue.ErrMessageNotFound
	ErrMessageExpired     = internalQueue.ErrMessageExpired
	ErrMaxRetriesExceeded = internalQueue.ErrMaxRetriesExceeded
)

// Queue represents a public handle to a durable or in-memory task queue.
type Queue struct {
	engine *internalQueue.QueueEngine
}

// New creates a new public Queue handle around an internal engine.
func New(engine *internalQueue.QueueEngine) *Queue {
	return &Queue{engine: engine}
}

// Name returns the queue name.
func (q *Queue) Name() string {
	return q.engine.Name()
}

// DLQ returns the associated Dead-Letter Queue handle, or nil if disabled.
func (q *Queue) DLQ() *Queue {
	dlqEngine := q.engine.DLQ()
	if dlqEngine == nil {
		return nil
	}
	return New(dlqEngine)
}

// Enqueue inserts a task or payload into the queue with optional deduplication and scheduling.
func (q *Queue) Enqueue(ctx context.Context, payload []byte, opts ...Option) (*Message, error) {
	cfg := DefaultOptions()
	for _, o := range opts {
		o(&cfg)
	}

	var visibleAt int64
	now := time.Now().UnixNano()
	if cfg.Delay > 0 {
		visibleAt = now + int64(cfg.Delay)
	} else {
		visibleAt = now
	}

	var expireAt int64
	if cfg.TTL > 0 {
		expireAt = now + int64(cfg.TTL)
	}

	msg := &Message{
		Payload:    payload,
		Priority:   cfg.Priority,
		DedupID:    cfg.DedupID,
		MaxRetries: cfg.MaxRetries,
		VisibleAt:  visibleAt,
		ExpireAt:   expireAt,
	}

	return q.engine.Enqueue(msg)
}

// Dequeue retrieves the next visible message from the queue, blocking until one is available or ctx is cancelled.
func (q *Queue) Dequeue(ctx context.Context, opts ...Option) (*Message, error) {
	cfg := DefaultOptions()
	for _, o := range opts {
		o(&cfg)
	}
	return q.engine.Dequeue(ctx, cfg.VisibilityTimeout)
}

// DequeueBatch retrieves up to count visible messages from the queue.
func (q *Queue) DequeueBatch(ctx context.Context, count int, opts ...Option) ([]*Message, error) {
	if count <= 0 {
		return nil, nil
	}

	results := make([]*Message, 0, count)
	for i := 0; i < count; i++ {
		// Only first dequeue blocks on context; subsequent ones use non-blocking / short timeout
		msg, err := q.Dequeue(ctx, opts...)
		if err != nil {
			if len(results) > 0 {
				return results, nil
			}
			return nil, err
		}
		results = append(results, msg)
	}
	return results, nil
}

// Stats returns the real-time queue depth metrics.
type Stats struct {
	ReadyCount     int
	InFlightCount  int
	DLQCount       int
}

func (q *Queue) Stats() Stats {
	s := q.engine.Stats()
	return Stats{
		ReadyCount:    s.ReadyCount,
		InFlightCount: s.InFlightCount,
		DLQCount:      s.DLQCount,
	}
}

// Ack marks a message as completed by its ID.
func (q *Queue) Ack(msgID string) error {
	return q.engine.Ack(msgID)
}

// Nack rejects an in-flight message by its ID.
func (q *Queue) Nack(msgID string, requeue bool) error {
	return q.engine.Nack(msgID, requeue)
}

// Close closes the queue and stops background lease workers.
func (q *Queue) Close() error {
	return q.engine.Close()
}
