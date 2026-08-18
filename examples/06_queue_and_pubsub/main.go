package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	coco "cocodb"
)

func main() {
	dbPath := "example_queue_pubsub.coco"
	_ = os.Remove(dbPath)
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-lock")
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-wal")
	defer os.Remove(dbPath + "-lock")

	fmt.Println("🚀 Initializing CoCoDB Multi-Model Storage Engine...")
	db, err := coco.Open(dbPath, coco.Profile(coco.Performance))
	if err != nil {
		panic(err)
	}
	defer db.Close()

	ctx := context.Background()

	// =========================================================================
	// 1. Transactional Queue with Distributed Deduplication (Exactly-Once Job)
	// =========================================================================
	fmt.Println("\n--- [1] Transactional Queue with Distributed Deduplication ---")
	orderQueue := db.Queue("order_processing")

	orderID := "order_checkout_TXN_998811"

	// Producer 1 enqueues task with Deduplication ID
	fmt.Printf("📦 Producer 1 enqueuing order: %s\n", orderID)
	msg1, err := orderQueue.Enqueue(ctx, []byte(`{"order_id": "998811", "amount": 149.99}`),
		coco.WithDedupID(orderID, 5*time.Minute),
		coco.WithPriority(200), // High priority
	)
	if err != nil {
		panic(err)
	}
	fmt.Printf("   ✅ Task successfully enqueued (Message ID: %s)\n", msg1.ID)

	// Producer 2 (e.g. from another API instance/replica) attempts to enqueue the same order concurrently
	fmt.Printf("📦 Producer 2 attempting to enqueue duplicate order: %s\n", orderID)
	_, err = orderQueue.Enqueue(ctx, []byte(`{"order_id": "998811", "amount": 149.99}`),
		coco.WithDedupID(orderID, 5*time.Minute),
	)
	if err == coco.ErrDuplicateMessage {
		fmt.Println("   🛑 Duplicate rejected! Exactly-once deduplication window active.")
	} else {
		fmt.Printf("   Unexpected error: %v\n", err)
	}

	// =========================================================================
	// 2. Worker Dequeue, Visibility Timeout & Acknowledgment (Ack)
	// =========================================================================
	fmt.Println("\n--- [2] Worker Processing with Lease & Acknowledgment ---")
	task, err := orderQueue.Dequeue(ctx, coco.WithVisibilityTimeout(2*time.Second))
	if err != nil {
		panic(err)
	}
	fmt.Printf("👷 Worker received task: %s (Payload: %s)\n", task.ID, string(task.Payload))
	fmt.Printf("   Status: %s, In-Flight Visibility Leased until: %v\n", task.State, time.Unix(0, task.VisibleAt).UTC())

	// Acknowledge task completion
	if err := task.Ack(); err != nil {
		panic(err)
	}
	fmt.Println("   ✅ Task completed and acknowledged (Ack) -> removed from queue.")

	// =========================================================================
	// 3. Real-Time Pub/Sub with Wildcards & Consumer Groups
	// =========================================================================
	fmt.Println("\n--- [3] Real-Time Pub/Sub with Consumer Groups ---")
	ps := db.PubSub()

	// Direct broadcast subscriber for audit logging (wildcard matching all sensor events)
	auditSub := ps.Subscribe(ctx, "sensors.>")
	defer auditSub.Unsubscribe()

	// Distributed Consumer Group: 2 competing workers sharing the "temperature_processors" group
	workerGroupSub1 := ps.SubscribeGroup(ctx, "sensors.iot.temperature", "temperature_processors")
	defer workerGroupSub1.Unsubscribe()

	workerGroupSub2 := ps.SubscribeGroup(ctx, "sensors.iot.temperature", "temperature_processors")
	defer workerGroupSub2.Unsubscribe()

	var wg sync.WaitGroup
	wg.Add(4) // 2 from audit logger + 2 from consumer group workers (SN-01 and SN-02)

	// Audit logger receiver
	go func() {
		for msg := range auditSub.Channel() {
			fmt.Printf("   📡 [Audit Logger] Received on topic %q: %s\n", msg.Topic, string(msg.Payload))
			wg.Done()
		}
	}()

	// Worker 1 receiver
	go func() {
		for msg := range workerGroupSub1.Channel() {
			fmt.Printf("   ⚙️  [Consumer Group Worker 1] Processing: %s\n", string(msg.Payload))
			wg.Done()
		}
	}()

	// Worker 2 receiver
	go func() {
		for msg := range workerGroupSub2.Channel() {
			fmt.Printf("   ⚙️  [Consumer Group Worker 2] Processing: %s\n", string(msg.Payload))
			wg.Done()
		}
	}()

	time.Sleep(50 * time.Millisecond)

	// Publish 2 events
	fmt.Println("📢 Publishing Event 1: sensors.iot.temperature (23.4°C)")
	_, _ = ps.Publish(ctx, "sensors.iot.temperature", []byte(`{"sensor_id": "SN-01", "temp": 23.4}`))

	fmt.Println("📢 Publishing Event 2: sensors.iot.temperature (24.1°C)")
	_, _ = ps.Publish(ctx, "sensors.iot.temperature", []byte(`{"sensor_id": "SN-02", "temp": 24.1}`))

	wg.Wait()

	fmt.Println("\n🎉 All Queue & Pub/Sub demonstration flows completed successfully!")
}
