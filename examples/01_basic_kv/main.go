package main

import (
	"fmt"
	"log"
	"os"
	"time"

	coco "github.com/mrzack99s/cocodb"
)

func main() {
	dbPath := "kv_example.coco"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-wal")

	// 1. Open CoCo Database
	db, err := coco.Open(dbPath, coco.Profile(coco.Balanced))
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	fmt.Println("=== 1. Direct Bucket Operations ===")
	bucket := db.Bucket("user_sessions")

	// Put keys with optional TTL
	_ = bucket.Put([]byte("session:usr_101"), []byte("tok_abc123"), coco.TTL(5*time.Minute))
	_ = bucket.Put([]byte("session:usr_102"), []byte("tok_def456"))
	_ = bucket.Put([]byte("session:usr_103"), []byte("tok_ghi789"))
	_ = bucket.Put([]byte("config:theme"), []byte("dark"))

	// Get a key
	val, err := bucket.Get([]byte("session:usr_101"))
	if err != nil {
		log.Fatalf("Get error: %v", err)
	}
	fmt.Printf("Fetched session:usr_101 -> %s\n", string(val))

	// Atomic counter increment
	newCount, _ := bucket.Increment([]byte("metrics:login_count"), 1)
	fmt.Printf("Login counter: %d\n", newCount)
	newCount, _ = bucket.Increment([]byte("metrics:login_count"), 5)
	fmt.Printf("Login counter after +5: %d\n", newCount)

	fmt.Println("\n=== 2. Prefix Scan ===")
	it := bucket.Prefix([]byte("session:"))
	for it.Valid() {
		fmt.Printf("  Key: %-20s Value: %s\n", string(it.Key()), string(it.Value()))
		it.Next()
	}
	it.Close()

	fmt.Println("\n=== 3. ACID Transaction (Update & View) ===")
	err = db.Update(func(tx *coco.Tx) error {
		b := tx.Bucket("accounts")
		_ = b.Put([]byte("acc:alice"), []byte("1000"))
		_ = b.Put([]byte("acc:bob"), []byte("500"))

		// Transfer 200 from Alice to Bob
		_ = b.Put([]byte("acc:alice"), []byte("800"))
		_ = b.Put([]byte("acc:bob"), []byte("700"))
		return nil
	})
	if err != nil {
		log.Fatalf("Transaction failed: %v", err)
	}

	_ = db.View(func(tx *coco.Tx) error {
		b := tx.Bucket("accounts")
		aliceBal, _ := b.Get([]byte("acc:alice"))
		bobBal, _ := b.Get([]byte("acc:bob"))
		fmt.Printf("Alice Balance: $%s, Bob Balance: $%s\n", string(aliceBal), string(bobBal))
		return nil
	})

	stats := db.Stats()
	fmt.Printf("\nDB Stats: %d pages allocated, Cache Hit Rate: %.1f%%\n", stats.PageCount, stats.CacheHitRate*100)
}
