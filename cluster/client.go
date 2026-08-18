package cluster

import (
	"context"

	internalCluster "github.com/mrzack99s/cocodb/internal/cluster"
)

// Client is a thread-safe client connected to a CoCoDB cluster with consistent hash routing.
type Client struct {
	internal *internalCluster.Client
}

// Dial connects to a CoCoDB cluster.
func Dial(addrs []string, opts ...ClientOption) (*Client, error) {
	cfg := ClientConfig{
		Addrs: addrs,
	}
	for _, opt := range opts {
		if opt != nil {
			opt.applyClient(&cfg)
		}
	}

	cli := internalCluster.NewClient(internalCluster.ClientConfig{
		Addrs:       cfg.Addrs,
		Secret:      cfg.Secret,
		TLSConfig:   cfg.TLSConfig,
		DialTimeout: cfg.DialTimeout,
		Dialer:      cfg.Dialer,
	})

	return &Client{
		internal: cli,
	}, nil
}

// Ping verifies cluster connectivity.
func (c *Client) Ping(ctx context.Context) error {
	return c.internal.Ping(ctx)
}

// Queue returns a handle to a distributed task queue.
func (c *Client) Queue(name string) *Queue {
	return &Queue{
		name:   name,
		client: c.internal,
	}
}

// PubSub returns a handle to the distributed Pub/Sub broker.
func (c *Client) PubSub() *PubSub {
	return &PubSub{
		client: c.internal,
	}
}

// Close closes all pooled cluster node connections.
func (c *Client) Close() error {
	return c.internal.Close()
}
