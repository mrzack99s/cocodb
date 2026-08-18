package pubsub_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mrzack99s/cocodb/pubsub"
)

func TestPubSub_Broadcast(t *testing.T) {
	ps := pubsub.New(64)
	ctx := context.Background()

	sub1 := ps.Subscribe(ctx, "news.tech")
	defer sub1.Unsubscribe()

	sub2 := ps.Subscribe(ctx, "news.tech")
	defer sub2.Unsubscribe()

	count, err := ps.Publish(ctx, "news.tech", []byte("Go 1.24 released"))
	if err != nil || count != 2 {
		t.Fatalf("expected 2 subscribers delivered, got count=%d (err: %v)", count, err)
	}

	select {
	case m1 := <-sub1.Channel():
		if string(m1.Payload) != "Go 1.24 released" {
			t.Fatalf("sub1 received unexpected payload: %s", string(m1.Payload))
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("sub1 timeout waiting for message")
	}

	select {
	case m2 := <-sub2.Channel():
		if string(m2.Payload) != "Go 1.24 released" {
			t.Fatalf("sub2 received unexpected payload: %s", string(m2.Payload))
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("sub2 timeout waiting for message")
	}
}

func TestPubSub_WildcardTopics(t *testing.T) {
	ps := pubsub.New(64)
	ctx := context.Background()

	// Single-segment wildcard: sensors.*.temp
	subSingle := ps.Subscribe(ctx, "sensors.*.temp")
	defer subSingle.Unsubscribe()

	// Multi-segment wildcard: events.>
	subMulti := ps.Subscribe(ctx, "events.>")
	defer subMulti.Unsubscribe()

	// Publish matching single-segment
	_, _ = ps.Publish(ctx, "sensors.kitchen.temp", []byte("22.5C"))

	select {
	case m := <-subSingle.Channel():
		if string(m.Payload) != "22.5C" {
			t.Fatalf("unexpected payload: %s", string(m.Payload))
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("single wildcard sub did not receive message")
	}

	// Publish multi-segment
	_, _ = ps.Publish(ctx, "events.user.account.created", []byte("user_101"))

	select {
	case m := <-subMulti.Channel():
		if string(m.Payload) != "user_101" {
			t.Fatalf("unexpected payload: %s", string(m.Payload))
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("multi wildcard sub did not receive message")
	}
}

func TestPubSub_ConsumerGroup(t *testing.T) {
	ps := pubsub.New(64)
	ctx := context.Background()

	// 3 worker instances subscribing under the same consumer group "payment_workers"
	subWorker1 := ps.SubscribeGroup(ctx, "payments", "payment_workers")
	defer subWorker1.Unsubscribe()

	subWorker2 := ps.SubscribeGroup(ctx, "payments", "payment_workers")
	defer subWorker2.Unsubscribe()

	subWorker3 := ps.SubscribeGroup(ctx, "payments", "payment_workers")
	defer subWorker3.Unsubscribe()

	var worker1Count, worker2Count, worker3Count atomic.Int64
	var wg sync.WaitGroup

	numMessages := 30
	wg.Add(numMessages)

	// Worker 1 listener
	go func() {
		for range subWorker1.Channel() {
			worker1Count.Add(1)
			wg.Done()
		}
	}()

	// Worker 2 listener
	go func() {
		for range subWorker2.Channel() {
			worker2Count.Add(1)
			wg.Done()
		}
	}()

	// Worker 3 listener
	go func() {
		for range subWorker3.Channel() {
			worker3Count.Add(1)
			wg.Done()
		}
	}()

	// Publish 30 payment tasks
	for i := 0; i < numMessages; i++ {
		_, err := ps.Publish(ctx, "payments", []byte(fmt.Sprintf("pay_%d", i)))
		if err != nil {
			t.Fatalf("Publish %d failed: %v", i, err)
		}
	}

	wg.Wait()

	totalProcessed := worker1Count.Load() + worker2Count.Load() + worker3Count.Load()
	if totalProcessed != int64(numMessages) {
		t.Fatalf("expected exactly %d total tasks processed across group, got %d", numMessages, totalProcessed)
	}

	// Verify all 3 workers shared the workload
	if worker1Count.Load() == 0 || worker2Count.Load() == 0 || worker3Count.Load() == 0 {
		t.Fatalf("load distribution failed: w1=%d w2=%d w3=%d", worker1Count.Load(), worker2Count.Load(), worker3Count.Load())
	}
}

func TestPubSub_Deduplication(t *testing.T) {
	ps := pubsub.New(64)
	ctx := context.Background()

	sub := ps.Subscribe(ctx, "orders")
	defer sub.Unsubscribe()

	dedupID := "order_event_123"

	// First publish succeeds
	count, err := ps.Publish(ctx, "orders", []byte("order_data"), pubsub.WithDedupID(dedupID, 500*time.Millisecond))
	if err != nil || count != 1 {
		t.Fatalf("first publish failed: count=%d (err: %v)", count, err)
	}

	// Immediate duplicate publish with same dedupID should be rejected with ErrDuplicateMessage
	_, err = ps.Publish(ctx, "orders", []byte("duplicate_order_data"), pubsub.WithDedupID(dedupID, 500*time.Millisecond))
	if err != pubsub.ErrDuplicateMessage {
		t.Fatalf("expected ErrDuplicateMessage, got %v", err)
	}

	// Verify subscriber only received 1 message
	select {
	case <-sub.Channel():
		// Received 1 message as expected
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("subscriber did not receive first message")
	}

	select {
	case m := <-sub.Channel():
		t.Fatalf("subscriber received unexpected duplicate message: %v", m)
	case <-time.After(50 * time.Millisecond):
		// Good, no duplicate delivered
	}
}

func TestPubSub_BackpressureDropOldest(t *testing.T) {
	ps := pubsub.New(2)
	ctx := context.Background()

	sub := ps.Subscribe(ctx, "metrics", pubsub.WithBufferSize(2), pubsub.WithBackpressure(pubsub.DropOldest))
	defer sub.Unsubscribe()

	// Fill buffer with 3 items (capacity 2)
	_, _ = ps.Publish(ctx, "metrics", []byte("val_1"))
	_, _ = ps.Publish(ctx, "metrics", []byte("val_2"))
	_, _ = ps.Publish(ctx, "metrics", []byte("val_3"))

	// Should receive val_2 and val_3 (val_1 was dropped)
	m1 := <-sub.Channel()
	if string(m1.Payload) != "val_2" {
		t.Fatalf("expected val_2, got %s", string(m1.Payload))
	}

	m2 := <-sub.Channel()
	if string(m2.Payload) != "val_3" {
		t.Fatalf("expected val_3, got %s", string(m2.Payload))
	}
}
