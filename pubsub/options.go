package pubsub

import (
	"time"

	internalPubSub "github.com/mrzack99s/cocodb/internal/pubsub"
)

type BackpressureStrategy = internalPubSub.BackpressureStrategy

const (
	BlockOnFull = internalPubSub.BlockOnFull
	DropOldest  = internalPubSub.DropOldest
	DropNewest  = internalPubSub.DropNewest
)

type Option func(*Options)

type Options struct {
	BufferSize   int
	Strategy     BackpressureStrategy
	DedupID      string
	DedupWindow  time.Duration
	ConsumerGroup string
}

func DefaultOptions() Options {
	return Options{
		BufferSize:  256,
		Strategy:    BlockOnFull,
		DedupWindow: 5 * time.Minute,
	}
}

// WithBufferSize sets the subscriber channel buffer capacity.
func WithBufferSize(size int) Option {
	return func(o *Options) {
		o.BufferSize = size
	}
}

// WithBackpressure sets how the subscription handles a full channel buffer.
func WithBackpressure(strategy BackpressureStrategy) Option {
	return func(o *Options) {
		o.Strategy = strategy
	}
}

// WithDedupID sets an idempotent deduplication key for published events.
func WithDedupID(id string, window ...time.Duration) Option {
	return func(o *Options) {
		o.DedupID = id
		if len(window) > 0 {
			o.DedupWindow = window[0]
		}
	}
}

// WithConsumerGroup subscribes under a consumer group name for distributed load sharing.
func WithConsumerGroup(groupName string) Option {
	return func(o *Options) {
		o.ConsumerGroup = groupName
	}
}
