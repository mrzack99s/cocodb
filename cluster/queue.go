package cluster

import (
	"context"
	"time"

	internalCluster "github.com/mrzack99s/cocodb/internal/cluster"
	"github.com/mrzack99s/cocodb/queue"
)

// Queue provides access to a distributed task queue with consistent hash routing.
type Queue struct {
	name   string
	client *internalCluster.Client
}

// Message represents a task received from the distributed queue.
type Message struct {
	ID         string
	Queue      string
	Payload    []byte
	RetryCount int
	Priority   uint8
	State      string
	client     *internalCluster.Client
}

// Ack marks the distributed task as successfully completed.
func (m *Message) Ack() error {
	if m.client == nil {
		return nil
	}
	return m.client.Ack(context.Background(), m.Queue, m.ID)
}

// Nack rejects the distributed task and optionally re-enqueues or routes to DLQ.
func (m *Message) Nack(requeue bool) error {
	if m.client == nil {
		return nil
	}
	return m.client.Nack(context.Background(), m.Queue, m.ID, requeue)
}

// Enqueue adds a task to the distributed queue with deduplication and priority options.
func (q *Queue) Enqueue(ctx context.Context, payload []byte, opts ...queue.Option) (*Message, error) {
	cfg := queue.DefaultOptions()
	for _, opt := range opts {
		opt(&cfg)
	}

	req := internalCluster.EnqueueReq{
		Queue:    q.name,
		Payload:  payload,
		DedupID:  cfg.DedupID,
		Priority: cfg.Priority,
	}
	if cfg.Delay > 0 {
		req.DelayMs = cfg.Delay.Milliseconds()
	}

	resp, err := q.client.Enqueue(ctx, req)
	if err != nil {
		return nil, err
	}

	return &Message{
		ID:       resp.MessageID,
		Queue:    resp.Queue,
		Payload:  payload,
		Priority: cfg.Priority,
		State:    resp.State,
		client:   q.client,
	}, nil
}

// Dequeue pulls an available task from the distributed queue.
func (q *Queue) Dequeue(ctx context.Context, opts ...queue.Option) (*Message, error) {
	cfg := queue.DefaultOptions()
	for _, opt := range opts {
		opt(&cfg)
	}

	req := internalCluster.DequeueReq{
		Queue:   q.name,
		AutoAck: false,
	}
	if cfg.VisibilityTimeout > 0 {
		req.VisibilityMs = cfg.VisibilityTimeout.Milliseconds()
	} else {
		req.VisibilityMs = (30 * time.Second).Milliseconds()
	}

	resp, err := q.client.Dequeue(ctx, req)
	if err != nil {
		return nil, err
	}
	if !resp.Found {
		return nil, queue.ErrQueueEmpty
	}

	return &Message{
		ID:         resp.MessageID,
		Queue:      resp.Queue,
		Payload:    resp.Payload,
		RetryCount: resp.RetryCount,
		Priority:   resp.Priority,
		State:      resp.State,
		client:     q.client,
	}, nil
}
