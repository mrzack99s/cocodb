package cocodb_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	coco "cocodb"
	"cocodb/internal/vector"
	"cocodb/search"
)

func BenchmarkKV_Put(b *testing.B) {
	dbPath := "bench_kv_put.coco"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-wal")

	db, err := coco.Open(dbPath, coco.Profile(coco.Performance), coco.SyncMode(coco.SyncOff))
	if err != nil {
		b.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("user:key:%08d", i))
		val := []byte("value_payload_data_for_high_throughput_testing_1234567890")
		_ = db.Bucket("bench_bucket").Put(key, val)
	}
}

func BenchmarkKV_Get(b *testing.B) {
	dbPath := "bench_kv_get.coco"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-wal")

	db, err := coco.Open(dbPath, coco.Profile(coco.Performance), coco.SyncMode(coco.SyncOff))
	if err != nil {
		b.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	bucket := db.Bucket("bench_bucket")
	const numKeys = 10000
	for i := 0; i < numKeys; i++ {
		_ = bucket.Put(
			[]byte(fmt.Sprintf("key:%06d", i)),
			[]byte("cached_payload_high_performance_data"),
		)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("key:%06d", i%numKeys))
		_, _ = bucket.Get(key)
	}
}

func BenchmarkKV_ParallelGet(b *testing.B) {
	dbPath := "bench_kv_par_get.coco"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-wal")

	db, err := coco.Open(dbPath, coco.Profile(coco.Performance), coco.SyncMode(coco.SyncOff))
	if err != nil {
		b.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	bucket := db.Bucket("bench_bucket")
	const numKeys = 10000
	for i := 0; i < numKeys; i++ {
		_ = bucket.Put(
			[]byte(fmt.Sprintf("key:%06d", i)),
			[]byte("cached_payload_high_performance_data"),
		)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := []byte(fmt.Sprintf("key:%06d", i%numKeys))
			_, _ = bucket.Get(key)
			i++
		}
	})
}

func BenchmarkDocument_Insert(b *testing.B) {
	dbPath := "bench_doc_insert.coco"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-wal")

	db, err := coco.Open(dbPath, coco.Profile(coco.Performance), coco.SyncMode(coco.SyncOff))
	if err != nil {
		b.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	col := db.Collection("bench_users")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = col.Insert(coco.Document{
			"name":   "Alex Mercer",
			"active": true,
			"age":    30,
			"score":  99.5,
			"tags":   []any{"go", "database", "embedded"},
		})
	}
}

func BenchmarkDocument_Query(b *testing.B) {
	dbPath := "bench_doc_query.coco"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-wal")

	db, err := coco.Open(dbPath, coco.Profile(coco.Performance), coco.SyncMode(coco.SyncOff))
	if err != nil {
		b.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	col := db.Collection("products")
	for i := 0; i < 1000; i++ {
		cat := "electronics"
		if i%2 == 0 {
			cat = "audio"
		}
		_, _ = col.Insert(coco.Document{
			"name":     fmt.Sprintf("Product #%d", i),
			"category": cat,
			"price":    float64(i * 10),
			"active":   true,
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = col.Query().
			Where("category").Eq("electronics").
			Where("price").Lt(5000.0).
			Limit(20).
			All()
	}
}

func BenchmarkVector_CosineSearch(b *testing.B) {
	cfg := vector.DefaultHNSWConfig(128)
	cfg.Metric = vector.Cosine
	hnsw := vector.NewHNSW(cfg)

	// Pre-populate with 1000 128-dimensional vectors
	for i := 1; i <= 1000; i++ {
		vec := make([]float32, 128)
		for j := range vec {
			vec[j] = float32(i+j) * 0.001
		}
		_ = hnsw.Insert(uint64(i), vec)
	}

	query := make([]float32, 128)
	for j := range query {
		query[j] = 0.5
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = hnsw.Search(query, 10)
	}
}

func BenchmarkText_BM25Search(b *testing.B) {
	idx := search.NewInvertedIndex()
	for i := 1; i <= 1000; i++ {
		idx.IndexDoc(
			uint64(i),
			fmt.Sprintf("High performance embedded multi-model storage engine in pure Go doc number %d", i),
		)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = idx.Search("performance database go", 10)
	}
}

func BenchmarkMVCC_ConcurrentReads(b *testing.B) {
	dbPath := "bench_mvcc.coco"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-wal")

	db, err := coco.Open(dbPath, coco.Profile(coco.Performance), coco.SyncMode(coco.SyncOff))
	if err != nil {
		b.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	bucket := db.Bucket("counter")
	_ = bucket.Put([]byte("key"), []byte("payload_val"))

	var wg sync.WaitGroup
	workers := 8
	opsPerWorker := b.N / workers
	if opsPerWorker == 0 {
		opsPerWorker = 1
	}

	b.ResetTimer()
	b.ReportAllocs()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				_ = db.View(func(tx *coco.Tx) error {
					_, err := tx.Bucket("counter").Get([]byte("key"))
					return err
				})
			}
		}()
	}
	wg.Wait()
}

func BenchmarkQueue_EnqueueDequeue(b *testing.B) {
	dbPath := "bench_queue.coco"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-wal")

	db, err := coco.Open(dbPath, coco.Profile(coco.Performance), coco.SyncMode(coco.SyncOff))
	if err != nil {
		b.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	q := db.Queue("bench_tasks")
	ctx := context.Background()
	payload := []byte("high_throughput_queue_task_payload_data_123")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		msg, _ := q.Enqueue(ctx, payload)
		m, _ := q.Dequeue(ctx)
		_ = m.Ack()
		_ = msg
	}
}

func BenchmarkPubSub_PublishBroadcast(b *testing.B) {
	ps := dbPubSub()
	ctx := context.Background()

	sub1 := ps.Subscribe(ctx, "events.live")
	defer sub1.Unsubscribe()
	sub2 := ps.Subscribe(ctx, "events.live")
	defer sub2.Unsubscribe()

	payload := []byte("realtime_event_streaming_payload_message")

	go func() {
		for range sub1.Channel() {
		}
	}()
	go func() {
		for range sub2.Channel() {
		}
	}()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = ps.Publish(ctx, "events.live", payload)
	}
}

func dbPubSub() *coco.PubSub {
	db, _ := coco.Open(":memory:")
	return db.PubSub()
}
