package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	coco "cocodb"
	"cocodb/studio"
)

func main() {
	var dbPath string
	var addr string
	var seed bool
	var readOnly bool

	flag.StringVar(&dbPath, "db", "app.coco", "Path to CoCo database file")
	flag.StringVar(&addr, "addr", ":8787", "HTTP listen address for Admin Studio")
	flag.BoolVar(&seed, "seed", false, "Seed initial sample data if database is newly created")
	flag.BoolVar(&readOnly, "readonly", false, "Open database in read-only mode")
	flag.Parse()

	// Allow positional argument for DB path: go run main.go [path]
	if flag.NArg() > 0 {
		dbPath = flag.Arg(0)
	}

	fmt.Println("==================================================")
	fmt.Println("   CoCo Embedded Database — Admin Studio          ")
	fmt.Println("==================================================")
	fmt.Printf("Database File: %s\n", dbPath)
	fmt.Printf("Listen Address: %s\n", addr)
	fmt.Printf("Read-Only Mode: %v\n", readOnly)

	// 1. Open Real CoCo Database
	var opts []coco.Option
	opts = append(opts, coco.Profile(coco.Balanced))
	if readOnly {
		opts = append(opts, coco.ReadOnly())
	}

	db, err := coco.Open(dbPath, opts...)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// 2. Optional sample data seeding (only if --seed flag was explicitly passed)
	if seed && !readOnly {
		seedData(db)
	}

	// 3. Start Admin Studio HTTP Server
	srv := studio.NewServer(db, addr)
	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start Admin Studio: %v", err)
	}

	fmt.Println()
	fmt.Println("🚀 CoCo Admin Studio is running with LIVE database data!")
	fmt.Printf("👉 Open your browser at: %s\n\n", srv.URL())
	fmt.Println("Available Real-Time Features:")
	fmt.Println("  • 📊 Dashboard: Live kernel metrics, LRU hit rate, allocated pages, LSN")
	fmt.Println("  • 🗄️ Collections: Document CRUD, filter queries, binary schema viewer")
	fmt.Println("  • 🔑 KV Buckets: Key/Value browser, prefix scanner, TTL manager")
	fmt.Println("  • 📦 Task Queues: Durable task scheduling, visibility timeout leases, DLQ")
	fmt.Println("  • 📢 Pub/Sub Broker: Real-time event broadcasting, wildcards, consumer groups")
	fmt.Println("  • ⚡ Query Console: Physical Volcano execution planner & AST analyzer")
	fmt.Println("  • 🧠 Vector Search: Real HNSW vector similarity testing")
	fmt.Println("  • 🔍 Full-Text: Real Unicode BM25 keyword relevance ranking")
	fmt.Println("  • 🛠️ Integrity & Tools: Instant CRC32C storage kernel validation & backup")
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop.")

	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down CoCo Admin Studio...")
}

func seedData(db *coco.DB) {
	fmt.Println("Seeding sample documents, KV buckets, and task queues...")
	_ = db.Update(func(tx *coco.Tx) error {
		settings := tx.Bucket("system_settings")
		_ = settings.Put([]byte("server:cluster"), []byte("prod-ap-southeast-1"))
		_ = settings.Put([]byte("server:env"), []byte("production"))

		users := tx.Collection("users")
		_, _ = users.Insert(coco.Document{
			"_id":       "u_01",
			"name":      "Alice Johnson",
			"email":     "alice@example.com",
			"role":      "admin",
			"active":    true,
			"embedding": []float32{0.91, 0.12, 0.35, 0.08},
		})
		_, _ = users.Insert(coco.Document{
			"_id":       "u_02",
			"name":      "Bob Smith",
			"email":     "bob@example.com",
			"role":      "engineer",
			"active":    true,
			"embedding": []float32{0.15, 0.88, 0.72, 0.10},
		})
		return nil
	})

	// Seed queue tasks
	q := db.Queue("order_processing")
	_, _ = q.Enqueue(nil, []byte(`{"order_id": "9901", "action": "charge"}`), coco.WithPriority(200))
	_, _ = q.Enqueue(nil, []byte(`{"order_id": "9902", "action": "fulfill"}`), coco.WithPriority(150))
	fmt.Println("✓ Seed completed.")
}
