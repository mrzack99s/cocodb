package queue

import (
	"sync"
	"time"
)

type dedupEntry struct {
	expireAt int64 // UnixNano
}

// Deduplicator manages sliding-window message deduplication to prevent duplicate processing.
type Deduplicator struct {
	mu      sync.RWMutex
	entries map[string]dedupEntry
}

// NewDeduplicator creates a new in-memory sliding window deduplicator.
func NewDeduplicator() *Deduplicator {
	return &Deduplicator{
		entries: make(map[string]dedupEntry),
	}
}

// CheckOrSet checks if dedupID has been recorded within window.
// Returns true if new (allowed to process/enqueue), false if duplicate (rejected).
func (d *Deduplicator) CheckOrSet(dedupID string, window time.Duration) bool {
	if dedupID == "" {
		return true // No deduplication requested
	}

	now := time.Now().UnixNano()
	expireAt := now + int64(window)

	d.mu.Lock()
	defer d.mu.Unlock()

	if entry, exists := d.entries[dedupID]; exists {
		if entry.expireAt > now {
			// Still within deduplication window -> duplicate!
			return false
		}
	}

	// Record or update deduplication entry
	d.entries[dedupID] = dedupEntry{expireAt: expireAt}

	// Opportunistic periodic cleanup if map grows
	if len(d.entries) > 1000 {
		d.cleanupLocked(now)
	}

	return true
}

// IsDuplicate checks whether dedupID is currently active without adding it.
func (d *Deduplicator) IsDuplicate(dedupID string) bool {
	if dedupID == "" {
		return false
	}

	now := time.Now().UnixNano()

	d.mu.RLock()
	defer d.mu.RUnlock()

	if entry, exists := d.entries[dedupID]; exists {
		return entry.expireAt > now
	}
	return false
}

// Remove manually clears a deduplication entry (e.g. on task rollback).
func (d *Deduplicator) Remove(dedupID string) {
	if dedupID == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.entries, dedupID)
}

func (d *Deduplicator) cleanupLocked(now int64) {
	for id, entry := range d.entries {
		if entry.expireAt <= now {
			delete(d.entries, id)
		}
	}
}
