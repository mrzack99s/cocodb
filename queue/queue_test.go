package queue_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"cocodb/internal/file"
	internalQueue "cocodb/internal/queue"
	"cocodb/internal/storage"
	"cocodb/queue"
)

func createTestQueue(t *testing.T, name string) *queue.Queue {
	memBackend := file.NewMemoryBackend()
	pager, err := storage.OpenPager(memBackend, 4*1024*1024, false)
	if err != nil {
		t.Fatalf("OpenPager failed: %v", err)
	}

	cfg := internalQueue.DefaultConfig()
	cfg.DefaultVisibilityTimeout = 50 * time.Millisecond
	cfg.DefaultMaxRetries = 2
	engine := internalQueue.NewQueueEngine(name, cfg, nil, pager, false)
	return queue.New(engine)
}

func TestQueue_BasicFIFOAndAck(t *testing.T) {
	q := createTestQueue(t, "tasks")
	defer q.Close()

	ctx := context.Background()

	// Enqueue 3 items
	for i := 1; i <= 3; i++ {
		_, err := q.Enqueue(ctx, []byte(fmt.Sprintf("task_%d", i)))
		if err != nil {
			t.Fatalf("Enqueue %d failed: %v", i, err)
		}
	}

	stats := q.Stats()
	if stats.ReadyCount != 3 {
		t.Fatalf("expected 3 ready, got %d", stats.ReadyCount)
	}

	// Dequeue item 1
	msg1, err := q.Dequeue(ctx)
	if err != nil || string(msg1.Payload) != "task_1" {
		t.Fatalf("expected task_1, got %v (err: %v)", msg1, err)
	}
	if err := msg1.Ack(); err != nil {
		t.Fatalf("Ack failed: %v", err)
	}

	// Dequeue item 2
	msg2, err := q.Dequeue(ctx)
	if err != nil || string(msg2.Payload) != "task_2" {
		t.Fatalf("expected task_2, got %v (err: %v)", msg2, err)
	}
	if err := msg2.Ack(); err != nil {
		t.Fatalf("Ack failed: %v", err)
	}

	// Dequeue item 3
	msg3, err := q.Dequeue(ctx)
	if err != nil || string(msg3.Payload) != "task_3" {
		t.Fatalf("expected task_3, got %v (err: %v)", msg3, err)
	}
	if err := msg3.Ack(); err != nil {
		t.Fatalf("Ack failed: %v", err)
	}

	stats = q.Stats()
	if stats.ReadyCount != 0 || stats.InFlightCount != 0 {
		t.Fatalf("expected empty queue, got ready=%d inFlight=%d", stats.ReadyCount, stats.InFlightCount)
	}
}

func TestQueue_Deduplication(t *testing.T) {
	q := createTestQueue(t, "orders")
	defer q.Close()

	ctx := context.Background()
	dedupID := "order_998877"

	// First enqueue should succeed
	msg1, err := q.Enqueue(ctx, []byte("process order 998877"), queue.WithDedupID(dedupID, 500*time.Millisecond))
	if err != nil || msg1 == nil {
		t.Fatalf("first enqueue failed: %v", err)
	}

	// Concurrent enqueues with identical dedupID from multiple workers should be rejected
	var wg sync.WaitGroup
	duplicateRejectionCount := 0
	var countMu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			_, err := q.Enqueue(ctx, []byte(fmt.Sprintf("duplicate from worker %d", workerID)), queue.WithDedupID(dedupID, 500*time.Millisecond))
			if err == queue.ErrDuplicateMessage {
				countMu.Lock()
				duplicateRejectionCount++
				countMu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if duplicateRejectionCount != 10 {
		t.Fatalf("expected exactly 10 duplicate rejections, got %d", duplicateRejectionCount)
	}

	// Exactly 1 message in queue
	stats := q.Stats()
	if stats.ReadyCount != 1 {
		t.Fatalf("expected exactly 1 message in ready queue, got %d", stats.ReadyCount)
	}
}

func TestQueue_VisibilityTimeoutAndFailover(t *testing.T) {
	q := createTestQueue(t, "failover")
	defer q.Close()

	ctx := context.Background()
	_, _ = q.Enqueue(ctx, []byte("flaky_task"))

	// Worker 1 dequeues with 40ms visibility timeout
	msg1, err := q.Dequeue(ctx, queue.WithVisibilityTimeout(40*time.Millisecond))
	if err != nil || string(msg1.Payload) != "flaky_task" {
		t.Fatalf("Worker 1 Dequeue failed: %v", err)
	}

	// Worker 1 simulates crash / does not Ack
	// Wait for visibility timeout to expire
	time.Sleep(150 * time.Millisecond)

	// Worker 2 should now be able to pick up the failed task
	ctxTimeout, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	msg2, err := q.Dequeue(ctxTimeout)
	if err != nil || msg2 == nil {
		t.Fatalf("Worker 2 failed to recover unacknowledged message: %v", err)
	}

	if msg2.RetryCount != 2 {
		t.Fatalf("expected RetryCount=2, got %d", msg2.RetryCount)
	}

	// Worker 2 successfully Acks
	if err := msg2.Ack(); err != nil {
		t.Fatalf("Ack failed: %v", err)
	}
}

func TestQueue_DeadLetterQueue(t *testing.T) {
	q := createTestQueue(t, "poison_pills")
	defer q.Close()

	ctx := context.Background()
	_, _ = q.Enqueue(ctx, []byte("bad_payload"), queue.WithMaxRetries(2))

	dlq := q.DLQ()
	if dlq == nil {
		t.Fatalf("expected DLQ to be enabled")
	}

	// Attempt 1: Nack
	msg1, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue 1 failed: %v", err)
	}
	_ = msg1.Nack(true)

	// Attempt 2: Nack -> reaches max retries 2 -> should move to DLQ
	msg2, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue 2 failed: %v", err)
	}
	err = msg2.Nack(true)
	if err != queue.ErrMaxRetriesExceeded {
		t.Fatalf("expected ErrMaxRetriesExceeded, got %v", err)
	}

	// Main queue should now be empty
	stats := q.Stats()
	if stats.ReadyCount != 0 {
		t.Fatalf("main queue should be empty, got ready=%d", stats.ReadyCount)
	}

	// DLQ should contain the dead letter message
	dlqCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	deadMsg, err := dlq.Dequeue(dlqCtx)
	if err != nil || string(deadMsg.Payload) != "bad_payload" {
		t.Fatalf("DLQ Dequeue failed: %v", err)
	}
	_ = deadMsg.Ack()
}

func TestQueue_Priority(t *testing.T) {
	q := createTestQueue(t, "priority")
	defer q.Close()

	ctx := context.Background()

	// Enqueue Low priority (50), High priority (200), Medium priority (128)
	_, _ = q.Enqueue(ctx, []byte("low"), queue.WithPriority(50))
	_, _ = q.Enqueue(ctx, []byte("high"), queue.WithPriority(200))
	_, _ = q.Enqueue(ctx, []byte("medium"), queue.WithPriority(128))

	// Should dequeue High first
	m1, _ := q.Dequeue(ctx)
	if string(m1.Payload) != "high" {
		t.Fatalf("expected high first, got %s", string(m1.Payload))
	}
	_ = m1.Ack()

	// Then Medium
	m2, _ := q.Dequeue(ctx)
	if string(m2.Payload) != "medium" {
		t.Fatalf("expected medium second, got %s", string(m2.Payload))
	}
	_ = m2.Ack()

	// Then Low
	m3, _ := q.Dequeue(ctx)
	if string(m3.Payload) != "low" {
		t.Fatalf("expected low third, got %s", string(m3.Payload))
	}
	_ = m3.Ack()
}

func TestQueue_DelayedDelivery(t *testing.T) {
	q := createTestQueue(t, "delayed")
	defer q.Close()

	ctx := context.Background()

	// Schedule 100ms in future
	_, _ = q.Enqueue(ctx, []byte("future_task"), queue.WithDelay(100*time.Millisecond))

	// Immediate attempt with short timeout should fail/timeout
	ctxShort, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	_, err := q.Dequeue(ctxShort)
	cancel()
	if err == nil {
		t.Fatalf("expected timeout because task is delayed")
	}

	// Wait for delay to pass
	time.Sleep(120 * time.Millisecond)

	ctxWait, cancelWait := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancelWait()
	msg, err := q.Dequeue(ctxWait)
	if err != nil || string(msg.Payload) != "future_task" {
		t.Fatalf("failed to dequeue delayed task after timer: %v", err)
	}
	_ = msg.Ack()
}
