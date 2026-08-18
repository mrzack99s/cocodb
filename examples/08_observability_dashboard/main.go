package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	coco "github.com/mrzack99s/cocodb"
	"github.com/mrzack99s/cocodb/dashboard"
)

func main() {
	fmt.Println("🚀 Initializing CoCoDB Observability & Metrics Dashboard...")

	dbPath := "demo_observability.coco"
	db, err := coco.Open(dbPath,
		coco.Profile(coco.Balanced),
	)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// 1. Seed some initial collections and task queues
	users := db.Collection("users")
	_, _ = users.Insert(coco.Document{"name": "Alice", "role": "admin", "active": true})
	_, _ = users.Insert(coco.Document{"name": "Bob", "role": "engineer", "active": true})

	orderQueue := db.Queue("order_processing")
	_, _ = orderQueue.Enqueue(context.Background(), []byte(`{"order_id": "init_01", "amount": 199.00}`))

	// 2. Start the Standalone Observability Dashboard Server on :9090
	port := ":9090"
	srv := dashboard.NewServer(db, port)
	if err := srv.Start(); err != nil {
		panic(err)
	}

	fmt.Println("\n=======================================================")
	fmt.Printf("📊 CoCoDB Observability Dashboard is LIVE at:\n")
	fmt.Printf("   👉 Web Dashboard UI : %s\n", srv.URL())
	fmt.Printf("   👉 Prometheus Metrics: %s/metrics\n", srv.URL())
	fmt.Println("=======================================================")

	// 3. Background Workload Simulator (Generates continuous live QPS, Cache hits, and Events)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		bucket := db.Bucket("analytics_stream")
		ps := db.PubSub()
		r := rand.New(rand.NewSource(time.Now().UnixNano()))

		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		var counter int
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				counter++
				key := fmt.Sprintf("metric_key_%d", r.Intn(100))
				val := fmt.Sprintf("val_%d_%d", counter, time.Now().UnixNano())

				// Perform KV Put & Get
				_ = bucket.Put([]byte(key), []byte(val))
				_, _ = bucket.Get([]byte(key))

				// Periodically enqueue a task and broadcast an event
				if counter%5 == 0 {
					_, _ = orderQueue.Enqueue(ctx, []byte(fmt.Sprintf(`{"task_id": %d, "timestamp": %d}`, counter, time.Now().UnixNano())),
						coco.WithVisibilityTimeout(10*time.Second),
					)
				}
				if counter%10 == 0 {
					_, _ = ps.Publish(ctx, "system.heartbeat", []byte(fmt.Sprintf(`{"tick": %d}`, counter)))
				}
				if counter%15 == 0 {
					// Simulate worker task processing
					if msg, err := orderQueue.Dequeue(ctx); err == nil && msg != nil {
						_ = msg.Ack()
					}
				}
			}
		}
	}()

	fmt.Println("⚡ Background workload generator active (Simulating continuous reads, writes, and queues)")
	fmt.Println("👉 Press Ctrl+C to stop the dashboard server.")

	// Wait for terminate signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\n🛑 Shutting down Observability Dashboard gracefully...")
	_ = srv.Stop(context.Background())
	fmt.Println("👋 Server stopped. Goodbye!")
}
