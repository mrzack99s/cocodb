package maintenance

import (
	"context"
	"sync"
	"time"

	"github.com/mrzack99s/cocodb/internal/index"
	"github.com/mrzack99s/cocodb/internal/storage"
	"github.com/mrzack99s/cocodb/internal/types"
	"github.com/mrzack99s/cocodb/internal/wal"
)

// Scheduler manages periodic maintenance tasks (checkpoint, TTL cleanup, vacuum).
type Scheduler struct {
	mu       sync.Mutex
	pager    storage.Pager
	wal      *wal.WAL
	ttlIndex *index.TTLIndex
	deleteFn func(objID types.ObjectID, keyOrID []byte) error
	interval time.Duration
	stopCh   chan struct{}
	running  bool
}

func NewScheduler(
	pager storage.Pager,
	walManager *wal.WAL,
	ttlIndex *index.TTLIndex,
	deleteFn func(objID types.ObjectID, keyOrID []byte) error,
	interval time.Duration,
) *Scheduler {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Scheduler{
		pager:    pager,
		wal:      walManager,
		ttlIndex: ttlIndex,
		deleteFn: deleteFn,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start launches the background scheduler goroutine.
func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go s.loop()
}

// Stop stops the background scheduler.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopCh)
	s.mu.Unlock()
}

func (s *Scheduler) loop() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			_ = s.RunOnce(context.Background())
		}
	}
}

// RunOnce executes a single maintenance iteration.
func (s *Scheduler) RunOnce(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Clean expired TTL entries
	if s.ttlIndex != nil && s.deleteFn != nil {
		expired, err := s.ttlIndex.FindExpired(time.Now(), 100)
		if err == nil {
			for _, item := range expired {
				_ = s.deleteFn(item.ObjectID, item.KeyOrID)
				_ = s.ttlIndex.Remove(time.Now(), item.ObjectID, item.KeyOrID)
			}
		}
	}

	// 2. Checkpoint WAL
	if err := s.pager.FlushAll(); err != nil {
		return err
	}
	if s.wal != nil {
		_ = s.wal.Truncate()
	}

	return nil
}
