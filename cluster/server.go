package cluster

import (
	"github.com/mrzack99s/cocodb"
	internalCluster "github.com/mrzack99s/cocodb/internal/cluster"
)

// Node represents a running CoCoDB cluster server node.
type Node struct {
	server *internalCluster.Server
	ring   *internalCluster.HashRing
}

// StartNode starts a high-performance cluster server node backed by a local CoCoDB instance.
func StartNode(db *cocodb.DB, addr string, opts ...NodeOption) (*Node, error) {
	cfg := NodeConfig{
		Addr: addr,
	}
	for _, opt := range opts {
		if opt != nil {
			opt.applyNode(&cfg)
		}
	}

	ring := internalCluster.NewHashRing(internalCluster.DefaultVirtualNodes)
	ring.AddNode(addr)
	for _, p := range cfg.Peers {
		ring.AddNode(p)
	}

	srv := internalCluster.NewServer(db, ring, internalCluster.ServerConfig{
		NodeID:      cfg.NodeID,
		Addr:        cfg.Addr,
		Secret:      cfg.Secret,
		TLSConfig:   cfg.TLSConfig,
		ReadTimeout: cfg.ReadTimeout,
		Listener:    cfg.Listener,
	})

	if err := srv.Start(); err != nil {
		return nil, err
	}

	db.SetClusterStatusProvider(func() any {
		return srv.Status()
	})

	return &Node{
		server: srv,
		ring:   ring,
	}, nil
}

// Addr returns the network listening address of the node.
func (n *Node) Addr() string {
	if a := n.server.Addr(); a != nil {
		return a.String()
	}
	return ""
}

// Re-export cluster status types
type ClusterStatus = internalCluster.ClusterStatus
type PeerStatus = internalCluster.PeerStatus

// Status returns real-time cluster health and peer status.
func (n *Node) Status() ClusterStatus {
	if n.server == nil {
		return ClusterStatus{Enabled: false}
	}
	return n.server.Status()
}

// Close gracefully stops the cluster node server.
func (n *Node) Close() error {
	return n.server.Close()
}
