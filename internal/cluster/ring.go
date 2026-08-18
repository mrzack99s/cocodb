package cluster

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
)

const DefaultVirtualNodes = 256

// HashRing maps arbitrary keys/DedupIDs deterministically to cluster nodes.
type HashRing struct {
	mu          sync.RWMutex
	vNodes      int
	ring        []uint32
	ringNodeMap map[uint32]string
	members     map[string]bool
}

// NewHashRing creates a consistent hash ring with the specified virtual nodes per physical node.
func NewHashRing(vNodes int) *HashRing {
	if vNodes <= 0 {
		vNodes = DefaultVirtualNodes
	}
	return &HashRing{
		vNodes:      vNodes,
		ringNodeMap: make(map[uint32]string),
		members:     make(map[string]bool),
	}
}

func hashKey(key string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return h.Sum32()
}

// AddNode registers a node on the hash ring with its virtual token positions.
func (hr *HashRing) AddNode(node string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if hr.members[node] {
		return
	}
	hr.members[node] = true

	for i := 0; i < hr.vNodes; i++ {
		vKey := fmt.Sprintf("%s#vnode-%d", node, i)
		h := hashKey(vKey)
		hr.ring = append(hr.ring, h)
		hr.ringNodeMap[h] = node
	}

	sort.Slice(hr.ring, func(i, j int) bool {
		return hr.ring[i] < hr.ring[j]
	})
}

// Members returns a copy of all registered physical node addresses in the ring.
func (hr *HashRing) Members() []string {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	res := make([]string, 0, len(hr.members))
	for m := range hr.members {
		res = append(res, m)
	}
	sort.Strings(res)
	return res
}

// VirtualNodes returns the virtual tokens count per physical node.
func (hr *HashRing) VirtualNodes() int {
	return hr.vNodes
}

// RemoveNode removes a physical node and its virtual nodes from the ring.
func (hr *HashRing) RemoveNode(node string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if !hr.members[node] {
		return
	}
	delete(hr.members, node)

	newRing := make([]uint32, 0, len(hr.ring)-hr.vNodes)
	for _, h := range hr.ring {
		if hr.ringNodeMap[h] == node {
			delete(hr.ringNodeMap, h)
		} else {
			newRing = append(newRing, h)
		}
	}
	hr.ring = newRing
}

// GetNode routes a key or DedupID to its primary owner node in O(log N) time.
func (hr *HashRing) GetNode(key string) (string, bool) {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	if len(hr.ring) == 0 {
		return "", false
	}

	h := hashKey(key)
	idx := sort.Search(len(hr.ring), func(i int) bool {
		return hr.ring[i] >= h
	})

	// Wrap around ring if beyond the end
	if idx == len(hr.ring) {
		idx = 0
	}

	node := hr.ringNodeMap[hr.ring[idx]]
	return node, true
}
