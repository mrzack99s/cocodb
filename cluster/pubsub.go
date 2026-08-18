package cluster

import (
	"context"

	internalCluster "github.com/mrzack99s/cocodb/internal/cluster"
	"github.com/mrzack99s/cocodb/pubsub"
)

// PubSub provides distributed event broadcasting across cluster nodes.
type PubSub struct {
	client *internalCluster.Client
}

// Publish broadcasts a message to a topic on the cluster with optional deduplication.
func (ps *PubSub) Publish(ctx context.Context, topic string, payload []byte, opts ...pubsub.Option) (int, error) {
	cfg := pubsub.DefaultOptions()
	for _, opt := range opts {
		opt(&cfg)
	}

	return ps.client.Publish(ctx, topic, payload, cfg.DedupID)
}
