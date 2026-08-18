package queue

import (
	"time"
)

// Option configures queue behavior or enqueue parameters.
type Option func(*Options)

type Options struct {
	VisibilityTimeout time.Duration
	MaxRetries        int
	DedupID           string
	DedupWindow       time.Duration
	Priority          uint8
	Delay             time.Duration
	TTL               time.Duration
	EnableDLQ         bool
}

func DefaultOptions() Options {
	return Options{
		VisibilityTimeout: 30 * time.Second,
		MaxRetries:        5,
		DedupWindow:       10 * time.Minute,
		Priority:          128,
		EnableDLQ:         true,
	}
}

// WithVisibilityTimeout sets how long a dequeued task remains invisible before failover.
func WithVisibilityTimeout(d time.Duration) Option {
	return func(o *Options) {
		o.VisibilityTimeout = d
	}
}

// WithMaxRetries sets the maximum number of failed deliveries before routing to DLQ.
func WithMaxRetries(n int) Option {
	return func(o *Options) {
		o.MaxRetries = n
	}
}

// WithDedupID sets an idempotent deduplication key for the task.
func WithDedupID(id string, window ...time.Duration) Option {
	return func(o *Options) {
		o.DedupID = id
		if len(window) > 0 {
			o.DedupWindow = window[0]
		}
	}
}

// WithPriority sets task priority (0 = lowest, 255 = highest).
func WithPriority(p uint8) Option {
	return func(o *Options) {
		o.Priority = p
	}
}

// WithDelay schedules the task to be delivered only after the specified duration.
func WithDelay(d time.Duration) Option {
	return func(o *Options) {
		o.Delay = d
	}
}

// WithTTL sets the message time-to-live before expiration.
func WithTTL(ttl time.Duration) Option {
	return func(o *Options) {
		o.TTL = ttl
	}
}

// WithDLQ enables or disables automatic Dead-Letter Queue routing.
func WithDLQ(enabled bool) Option {
	return func(o *Options) {
		o.EnableDLQ = enabled
	}
}
