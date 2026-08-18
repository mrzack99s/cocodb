package storage

import (
	"sync"

	"cocodb/internal/types"
)

// CacheShard is an isolated, locked LRU cache partition.
type CacheShard struct {
	mu       sync.RWMutex
	capacity int
	entries  map[types.PageID]*CacheEntry
	head     *CacheEntry // Most recently used
	tail     *CacheEntry // Least recently used
	hits     uint64
	misses   uint64
}

func newCacheShard(capacity int) *CacheShard {
	return &CacheShard{
		capacity: capacity,
		entries:  make(map[types.PageID]*CacheEntry),
	}
}

// Get returns the cached page and pins it if found.
func (s *CacheShard) Get(id types.PageID) *Page {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entries[id]
	if !ok {
		return nil
	}
	e.Pins++
	s.moveToHead(e)
	return e.Page
}

// Put inserts or updates a page in the shard and pins it.
func (s *CacheShard) Put(page *Page) *CacheEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := page.Header.ID
	if e, ok := s.entries[id]; ok {
		e.Page = page
		e.Pins++
		e.Dirty = page.Dirty
		e.PageLSN = page.Header.LSN
		s.moveToHead(e)
		return nil
	}

	e := &CacheEntry{
		ID:      id,
		Page:    page,
		Pins:    1,
		Dirty:   page.Dirty,
		PageLSN: page.Header.LSN,
	}
	s.entries[id] = e
	s.addToHead(e)

	// Check if over capacity
	if len(s.entries) > s.capacity {
		return s.evictOne()
	}
	return nil
}

// Pin increments the pin count for page id.
func (s *CacheShard) Pin(id types.PageID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[id]; ok {
		e.Pins++
		return true
	}
	return false
}

// Unpin decrements pin count for page id.
func (s *CacheShard) Unpin(id types.PageID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[id]; ok {
		if e.Pins > 0 {
			e.Pins--
		}
	}
}

// MarkDirty marks a page dirty in cache.
func (s *CacheShard) MarkDirty(id types.PageID, lsn types.LSN) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[id]; ok {
		e.Dirty = true
		e.PageLSN = lsn
		e.Page.Dirty = true
		e.Page.Header.LSN = lsn
	}
}

// Remove removes a page from the cache completely.
func (s *CacheShard) Remove(id types.PageID) *CacheEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[id]; ok {
		s.removeEntry(e)
		delete(s.entries, id)
		return e
	}
	return nil
}

// GetDirtyEntries returns all dirty entries across the shard without removing them.
func (s *CacheShard) GetDirtyEntries() []*CacheEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*CacheEntry
	for _, e := range s.entries {
		if e.Dirty || e.Page.Dirty {
			result = append(result, e)
		}
	}
	return result
}

func (s *CacheShard) evictOne() *CacheEntry {
	cur := s.tail
	for cur != nil {
		if cur.Pins == 0 {
			s.removeEntry(cur)
			delete(s.entries, cur.ID)
			return cur
		}
		cur = cur.prev
	}
	return nil
}

func (s *CacheShard) addToHead(e *CacheEntry) {
	e.prev = nil
	e.next = s.head
	if s.head != nil {
		s.head.prev = e
	}
	s.head = e
	if s.tail == nil {
		s.tail = e
	}
}

func (s *CacheShard) removeEntry(e *CacheEntry) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		s.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		s.tail = e.prev
	}
	e.prev = nil
	e.next = nil
}

func (s *CacheShard) moveToHead(e *CacheEntry) {
	if s.head == e {
		return
	}
	s.removeEntry(e)
	s.addToHead(e)
}
