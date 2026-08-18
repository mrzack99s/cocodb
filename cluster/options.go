package cluster

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	internalCluster "github.com/mrzack99s/cocodb/internal/cluster"
)

// NodeConfig contains settings for starting a cluster node.
type NodeConfig struct {
	NodeID      string
	Addr        string
	Secret      string
	Peers       []string
	TLSConfig   *tls.Config
	ReadTimeout time.Duration
	Listener    net.Listener
}

// ClientConfig contains settings for dialing a cluster.
type ClientConfig struct {
	Addrs       []string
	Secret      string
	TLSConfig   *tls.Config
	DialTimeout time.Duration
	Dialer      func(ctx context.Context, network, addr string) (net.Conn, error)
}

// NodeOption applies an option to NodeConfig.
type NodeOption interface {
	applyNode(*NodeConfig)
}

// ClientOption applies an option to ClientConfig.
type ClientOption interface {
	applyClient(*ClientConfig)
}

// Option is a combined option that can configure both nodes and clients.
type Option interface {
	NodeOption
	ClientOption
}

type nodeOptFunc func(*NodeConfig)
func (f nodeOptFunc) applyNode(c *NodeConfig) { f(c) }

type clientOptFunc func(*ClientConfig)
func (f clientOptFunc) applyClient(c *ClientConfig) { f(c) }

// WithListener sets a custom network or in-memory listener for a cluster node.
func WithListener(l net.Listener) NodeOption {
	return nodeOptFunc(func(c *NodeConfig) {
		c.Listener = l
	})
}

// WithDialer sets a custom dialer function for a cluster client.
func WithDialer(d func(ctx context.Context, network, addr string) (net.Conn, error)) ClientOption {
	return clientOptFunc(func(c *ClientConfig) {
		c.Dialer = d
	})
}

type combinedOpt struct {
	nodeFunc   func(*NodeConfig)
	clientFunc func(*ClientConfig)
}
func (co combinedOpt) applyNode(c *NodeConfig)     { if co.nodeFunc != nil { co.nodeFunc(c) } }
func (co combinedOpt) applyClient(c *ClientConfig) { if co.clientFunc != nil { co.clientFunc(c) } }

// WithSecret sets the cluster authentication secret token for nodes or clients.
func WithSecret(secret string) Option {
	return combinedOpt{
		nodeFunc:   func(c *NodeConfig) { c.Secret = secret },
		clientFunc: func(c *ClientConfig) { c.Secret = secret },
	}
}

// WithPeers specifies the addresses of peer cluster nodes.
func WithPeers(peers ...string) NodeOption {
	return nodeOptFunc(func(c *NodeConfig) {
		c.Peers = append(c.Peers, peers...)
	})
}

// WithTLS configures TLS 1.3 encryption.
func WithTLS(tlsCfg *tls.Config) Option {
	return combinedOpt{
		nodeFunc:   func(c *NodeConfig) { c.TLSConfig = tlsCfg },
		clientFunc: func(c *ClientConfig) { c.TLSConfig = tlsCfg },
	}
}

// WithDevmTLS generates and applies zero-setup in-memory mTLS 1.3 certificates.
func WithDevmTLS(hosts ...string) (NodeOption, ClientOption) {
	cert, pool, _ := internalCluster.GenerateDevCert(hosts...)
	serverTLS := internalCluster.ServerTLSConfig(cert, pool, true)
	clientTLS := internalCluster.ClientTLSConfig(&cert, pool, "cocodb.cluster.local")

	nodeOpt := nodeOptFunc(func(c *NodeConfig) {
		c.TLSConfig = serverTLS
	})
	clientOpt := clientOptFunc(func(c *ClientConfig) {
		c.TLSConfig = clientTLS
	})
	return nodeOpt, clientOpt
}
