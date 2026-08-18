package kv_test

import (
	"testing"
	"time"

	"cocodb/internal/btree"
	"cocodb/internal/file"
	"cocodb/internal/index"
	"cocodb/internal/storage"
	"cocodb/internal/txn"
	"cocodb/internal/types"
	"cocodb/internal/wal"
	"cocodb/kv"
)

func TestBucketKVOperations(t *testing.T) {
	memDB := file.NewMemoryBackend()
	pager, err := storage.OpenPager(memDB, 16*1024*1024, false)
	if err != nil {
		t.Fatalf("OpenPager failed: %v", err)
	}

	memWAL := file.NewMemoryBackend()
	walManager, err := wal.OpenWAL(memWAL, 0)
	if err != nil {
		t.Fatalf("OpenWAL failed: %v", err)
	}

	tm := txn.NewTxnManager(pager, walManager, txn.SyncOff)
	tx, _ := tm.Begin(false)

	tree := btree.NewBTree(pager, types.InvalidPageID)
	ttlIdx := index.NewTTLIndex(pager, types.InvalidPageID)
	bucket := kv.NewBucket("users", 1, tree, tx, ttlIdx)

	// Put & Get
	if err := bucket.Put([]byte("user:1"), []byte("Alice")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	val, err := bucket.Get([]byte("user:1"))
	if err != nil || string(val) != "Alice" {
		t.Fatalf("Get mismatch: %v, %s", err, string(val))
	}

	// PutIfAbsent
	inserted, err := bucket.PutIfAbsent([]byte("user:1"), []byte("Alice2"))
	if err != nil || inserted {
		t.Fatalf("PutIfAbsent should not have inserted existing key")
	}

	inserted, err = bucket.PutIfAbsent([]byte("user:2"), []byte("Bob"))
	if err != nil || !inserted {
		t.Fatalf("PutIfAbsent failed to insert new key")
	}

	// CAS
	swapped, err := bucket.CompareAndSwap([]byte("user:2"), []byte("Wrong"), []byte("Bob2"))
	if err != nil || swapped {
		t.Fatalf("CAS should have failed on wrong expected value")
	}

	swapped, err = bucket.CompareAndSwap([]byte("user:2"), []byte("Bob"), []byte("Bob2"))
	if err != nil || !swapped {
		t.Fatalf("CAS failed on matching expected value")
	}

	// Increment & Decrement
	count, err := bucket.Increment([]byte("counter"), 5)
	if err != nil || count != 5 {
		t.Fatalf("Increment failed: %v, count=%d", err, count)
	}
	count, err = bucket.Decrement([]byte("counter"), 2)
	if err != nil || count != 3 {
		t.Fatalf("Decrement failed: %v, count=%d", err, count)
	}

	// TTL Expiration
	_ = bucket.Put([]byte("temp"), []byte("tempval"), kv.WithTTL(10*time.Millisecond))
	time.Sleep(20 * time.Millisecond)
	expired, err := ttlIdx.FindExpired(time.Now(), 10)
	if err != nil || len(expired) == 0 {
		t.Fatalf("expected TTL index to find expired key")
	}

	// Batch
	batch := kv.NewBatch().
		Put([]byte("b1"), []byte("v1")).
		Put([]byte("b2"), []byte("v2")).
		Delete([]byte("user:1"))

	if err := bucket.Batch(batch); err != nil {
		t.Fatalf("Batch failed: %v", err)
	}

	exists, _ := bucket.Exists([]byte("user:1"))
	if exists {
		t.Fatalf("user:1 should have been deleted by batch")
	}

	// Range Iterator
	it := bucket.Range([]byte("b1"), []byte("b3"))
	defer it.Close()

	var keys []string
	for it.Valid() {
		keys = append(keys, string(it.Key()))
		it.Next()
	}
	if len(keys) != 2 || keys[0] != "b1" || keys[1] != "b2" {
		t.Fatalf("Range iterator mismatch: %v", keys)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
}
