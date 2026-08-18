package storage

import (
	"sync/atomic"

	"github.com/mrzack99s/cocodb/internal/types"
)

const DefaultShardCount = 16

// PageCache provides a sharded, thread-safe LRU page cache.
type PageCache struct {
	shards    []*CacheShard
	numShards uint64
	pageLimit int
	pageSize  uint32
}

// NewPageCache creates a PageCache with total capacity across shards.
func NewPageCache(maxBytes int64, pageSize uint32) *PageCache {
	if pageSize == 0 {
		pageSize = types.DefaultPageSize
	}
	if maxBytes <= 0 {
		maxBytes = 64 * 1024 * 1024 // 64 MB default
	}
	totalPages := int(maxBytes / int64(pageSize))
	if totalPages < DefaultShardCount {
		totalPages = DefaultShardCount
	}

	shardCap := totalPages / DefaultShardCount
	if shardCap < 1 {
		shardCap = 1
	}

	shards := make([]*CacheShard, DefaultShardCount)
	for i := 0; i < DefaultShardCount; i++ {
		shards[i] = newCacheShard(shardCap)
	}

	return &PageCache{
		shards:    shards,
		numShards: uint64(DefaultShardCount),
		pageLimit: totalPages,
		pageSize:  pageSize,
	}
}

func (c *PageCache) shardIndex(id types.PageID) int {
	return int(uint64(id) & (c.numShards - 1))
}

// Get returns the page from cache if present and pins it.
func (c *PageCache) Get(id types.PageID) *Page {
	idx := c.shardIndex(id)
	p := c.shards[idx].Get(id)
	if p != nil {
		atomic.AddUint64(&c.shards[idx].hits, 1)
	} else {
		atomic.AddUint64(&c.shards[idx].misses, 1)
	}
	return p
}

// Put inserts a page into the cache and pins it.
func (c *PageCache) Put(page *Page) *CacheEntry {
	idx := c.shardIndex(page.Header.ID)
	return c.shards[idx].Put(page)
}

// Pin pins a page in the cache.
func (c *PageCache) Pin(id types.PageID) bool {
	idx := c.shardIndex(id)
	return c.shards[idx].Pin(id)
}

// Unpin decrements pin count for page.
func (c *PageCache) Unpin(id types.PageID) {
	idx := c.shardIndex(id)
	c.shards[idx].Unpin(id)
}

// MarkDirty marks a page dirty with given LSN.
func (c *PageCache) MarkDirty(id types.PageID, lsn types.LSN) {
	idx := c.shardIndex(id)
	c.shards[idx].MarkDirty(id, lsn)
}

// Remove evicts a page from cache.
func (c *PageCache) Remove(id types.PageID) *CacheEntry {
	idx := c.shardIndex(id)
	return c.shards[idx].Remove(id)
}

// GetAllDirty returns all dirty entries across all shards.
func (c *PageCache) GetAllDirty() []*CacheEntry {
	var all []*CacheEntry
	for _, s := range c.shards {
		all = append(all, s.GetDirtyEntries()...)
	}
	return all
}

// Stats returns hit, miss, and hit-rate.
func (c *PageCache) Stats() (hits, misses uint64, hitRate float64) {
	for _, s := range c.shards {
		hits += atomic.LoadUint64(&s.hits)
		misses += atomic.LoadUint64(&s.misses)
	}
	total := hits + misses
	if total == 0 {
		return 0, 0, 0.0
	}
	return hits, misses, float64(hits) / float64(total)
}
