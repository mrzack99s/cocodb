package storage

import (
	"github.com/mrzack99s/cocodb/internal/types"
)

// CacheEntry represents a cached page node in the LRU shard.
type CacheEntry struct {
	ID      types.PageID
	Page    *Page
	Pins    int32
	Dirty   bool
	PageLSN types.LSN

	prev *CacheEntry
	next *CacheEntry
}
