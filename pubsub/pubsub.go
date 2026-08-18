package pubsub

import (
	"context"

	internalPubSub "github.com/mrzack99s/cocodb/internal/pubsub"
	internalQueue "github.com/mrzack99s/cocodb/internal/queue"
)

type Message = internalPubSub.Message
type Subscription = internalPubSub.Subscription

var (
	ErrDuplicateMessage = internalQueue.ErrDuplicateMessage
)

// PubSub represents a high-performance topic publish/subscribe broker.
type PubSub struct {
	engine *internalPubSub.Engine
}

// New creates a new PubSub broker.
func New(defaultBufSize int) *PubSub {
	return &PubSub{
		engine: internalPubSub.NewEngine(defaultBufSize),
	}
}

// Topic returns a scoped helper for publishing and subscribing to a specific topic.
func (ps *PubSub) Topic(name string) *Topic {
	return &Topic{
		ps:   ps,
		name: name,
	}
}

// Publish broadcasts a payload to all matching topic subscribers with optional deduplication.
func (ps *PubSub) Publish(ctx context.Context, topic string, payload []byte, opts ...Option) (int, error) {
	cfg := DefaultOptions()
	for _, o := range opts {
		o(&cfg)
	}

	return ps.engine.Publish(topic, payload, cfg.DedupID, cfg.DedupWindow)
}

// Subscribe subscribes to a topic or wildcard pattern (e.g. "orders.*", "events.>").
func (ps *PubSub) Subscribe(ctx context.Context, topicPattern string, opts ...Option) *Subscription {
	cfg := DefaultOptions()
	for _, o := range opts {
		o(&cfg)
	}

	if cfg.ConsumerGroup != "" {
		return ps.engine.SubscribeGroup(ctx, topicPattern, cfg.ConsumerGroup, cfg.BufferSize, cfg.Strategy)
	}
	return ps.engine.Subscribe(ctx, topicPattern, cfg.BufferSize, cfg.Strategy)
}

// SubscribeGroup creates a load-sharing subscription where each message is delivered to only one worker in the group.
func (ps *PubSub) SubscribeGroup(ctx context.Context, topic string, groupName string, opts ...Option) *Subscription {
	opts = append(opts, WithConsumerGroup(groupName))
	return ps.Subscribe(ctx, topic, opts...)
}

// Topic represents a scoped topic handle.
type Topic struct {
	ps   *PubSub
	name string
}

func (t *Topic) Name() string {
	return t.name
}

func (t *Topic) Publish(ctx context.Context, payload []byte, opts ...Option) (int, error) {
	return t.ps.Publish(ctx, t.name, payload, opts...)
}

func (t *Topic) Subscribe(ctx context.Context, opts ...Option) *Subscription {
	return t.ps.Subscribe(ctx, t.name, opts...)
}

func (t *Topic) SubscribeGroup(ctx context.Context, groupName string, opts ...Option) *Subscription {
	return t.ps.SubscribeGroup(ctx, t.name, groupName, opts...)
}

// Stats represents real-time PubSub metrics.
type Stats = internalPubSub.Stats

// Stats returns real-time Pub/Sub broker metrics.
func (ps *PubSub) Stats() Stats {
	return ps.engine.Stats()
}
