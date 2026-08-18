<div align="center">

# 🥥 CoCo Database (CoCoDB)

### *High-Performance, Single-File, Embedded Multi-Model Database in Pure Go*

[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg?style=flat)](LICENSE)
[![Build Status](https://img.shields.io/badge/Build-Passing-brightgreen.svg?style=flat)]()
[![CGO](https://img.shields.io/badge/CGO-Zero_Dependencies-blue.svg?style=flat)]()
[![Storage](https://img.shields.io/badge/Storage-Single--File_Kernel-purple.svg?style=flat)]()
[![Admin Studio](https://img.shields.io/badge/Admin_Studio-React_19_%2B_Tailwind-teal.svg?style=flat)]()

**CoCo** is a modern, zero-dependency embedded database engine engineered from the ground up in **Pure Go**. It unites **Key/Value Storage**, **NoSQL Document Collections**, **HNSW Vector Similarity Search**, and **BM25 Full-Text Search** into a single crash-safe storage kernel with ACID transaction guarantees and an embedded web-based **Admin Studio**.

[Features](#-key-features) • [Architecture](#-architecture) • [Quickstart](#-quickstart) • [Multi-Model APIs](#-multi-model-apis) • [Admin Studio](#-embedded-admin-studio) • [Examples](#-runnable-examples) • [Configuration](#-configuration--profiles)

---

</div>

## 🌟 Key Features

- **⚡ Multi-Model Data Storage**:
  - **Key/Value Store**: Ordered B+Tree buckets, TTL expiration, prefix scanning, and atomic integer increments.
  - **Document Database**: Compact binary document format (zero-copy field projections), single/compound secondary indexes, schema validation, and fluent Volcano query optimizer.
  - **Vector Search (HNSW)**: Approximate Nearest Neighbor (ANN) search with multi-layer persistent HNSW graph supporting Cosine, L2 Euclidean, and Dot Product distance metrics.
  - **Full-Text BM25 Search**: Inverted index postings with Unicode-aware tokenization and BM25 relevance scoring.
  - **Transactional Task Queue**: Durable, crash-safe queues with message visibility leases, Ack/Nack, DLQ, delayed delivery, and **Distributed Exactly-Once Deduplication**.
  - **Real-Time Pub/Sub**: High-throughput topic broker supporting single/multi-level wildcards (`orders.*`, `events.>`) and **Consumer Groups** for competing workers.
- **🛡️ Crash-Safe ACID Storage Substrate**:
  - **Single-File Storage**: Compact 16 KiB slotted page format with 64-byte metadata header and page compaction.
  - **Dual Meta Pages (Meta A / Meta B)**: Alternating crash-safe generation switching ensuring atomic commits.
  - **Hardware CRC32C Checksums**: Castagnoli 32-bit CRC checksum calculated and validated on every page I/O.
  - **Write-Ahead Logging (WAL)**: Redo log records with 3-Phase Crash Recovery (Analysis, Redo, Undo).
  - **MVCC & Snapshot Isolation**: Non-blocking concurrent readers with single-writer coordinator and Active Reader Table.
  - **16-Partition Sharded LRU Page Cache**: Minimizes lock contention under high-concurrency read workloads.
- **🎨 Built-in Admin Studio (Web GUI)**:
  - Zero external runtime dependencies — the React 19 + TypeScript + Tailwind CSS SPA is compiled and embedded directly into the Go binary via `//go:embed`.
  - Real-time kernel telemetry, KV explorer, Document browser, visual Volcano execution plan tree (`Filter -> IndexScan -> Limit`), Vector playground, BM25 keyword tester, and one-click integrity validator (`db.Check()`).
  - **Light ☀️ and Dark 🌙 Theme** support with instant toggle and `localStorage` persistence.
- **🔒 Security & Enterprise Hardening**:
  - Optional **AES-256-GCM Encryption at Rest** with PageID Additional Authenticated Data (AAD).
  - Point-in-time snapshot file backup and online restore.
  - Background maintenance scheduler for WAL checkpointing and automatic TTL page eviction.

---

## 🏛 Architecture

```
+-----------------------------------------------------------------------------------+
|                        CoCo Admin Studio (React 19 + Tailwind)                    |
|  [Dashboard Metrics]  [Collection Explorer]  [KV Browser]  [Query Planner & AST]  |
|  [Vector Similarity Playground]  [BM25 Full-Text Search]  [Integrity & Backups]   |
+-----------------------------------------+-----------------------------------------+
                                          | Embedded HTTP REST API on :8787
+-----------------------------------------v-----------------------------------------+
|                       Go HTTP REST Server (package studio)                        |
|   /api/stats    /api/catalog    /api/kv/*    /api/doc/*    /api/vector/*    ...   |
+-----------------------------------------+-----------------------------------------+
|                         CoCo Database Public Interface                            |
|             coco.Open()  •  db.Bucket()  •  db.Collection()  •  db.Update()       |
+-----------------------------------------+-----------------------------------------+
|                        Execution & Query Engine Layer                             |
|  - Volcano Physical Operators (CollectionScan, IndexScan, Filter, Sort, Limit)    |
|  - Query AST Optimizer & Aggregate Computations (Count, Sum, Avg, Min, Max)       |
|  - Multi-Layer HNSW Vector Graph Index (Cosine / L2 / Dot Product)                |
|  - Inverted Index & Unicode Normalizer with BM25 Relevance Scoring                |
+-----------------------------------------+-----------------------------------------+
|                        Transaction & MVCC Coordinator                             |
|  - Snapshot Isolation MVCC with Version Chains & Record Directory                 |
|  - Single-Writer ACID Coordinator with Savepoints and Nested Rollbacks            |
|  - Active Reader Table tracking oldest active read transactions                   |
+-----------------------------------------+-----------------------------------------+
|                           Storage Kernel & Substrate                              |
|  - 16-Partition Sharded LRU Page Cache with Dirty Page Writeback                  |
|  - Slotted Page Storage (16 KiB pages, Downward slot arrays, Zero-copy views)     |
|  - Write-Ahead Log (WAL) with 3-Phase Crash Recovery (Analysis, Redo, Undo)       |
|  - Dual Meta Pages (Meta A / Meta B) with Monotonic Generation Switching          |
|  - CRC32C Castagnoli Hardware Checksums & AES-256-GCM Encryption at Rest          |
+-----------------------------------------------------------------------------------+
```

---

## 📦 Installation

Requires **Go 1.24+** (no CGO or external C compilers needed):

```bash
go get github.com/mrzack99s/cocodb
```

---

## 🚀 Quickstart

Create a database, store Key/Value data, query NoSQL Documents, and launch the Web Admin Studio in under 30 lines of code:

```go
package main

import (
    "fmt"
    "log"
    coco "cocodb"
    "cocodb/studio"
)

func main() {
    // 1. Open or create a database
    db, err := coco.Open("app.coco", coco.Profile(coco.Balanced))
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // 2. Key/Value operations
    sessions := db.Bucket("sessions")
    _ = sessions.Put([]byte("usr:101"), []byte("tok_live_99812"))

    // 3. Document Collection operations
    users := db.Collection("users")
    _, _ = users.Insert(coco.Document{
        "name":   "Alex Mercer",
        "role":   "engineer",
        "active": true,
        "age":    29,
    })

    // 4. Query documents
    results, _ := users.Query().
        Where("active").Eq(true).
        Where("age").Gte(25).
        All()

    fmt.Printf("Found %d active users:\n", len(results))
    for _, u := range results {
        fmt.Printf("  • %s (%s, age %v)\n", u["name"], u["role"], u["age"])
    }

    // 5. Start Embedded Admin Studio on http://localhost:8787
    srv := studio.NewServer(db, ":8787")
    _ = srv.Start()
    fmt.Printf("\n🚀 Admin Studio running at: %s\n", srv.URL())

    select {}
}
```

---

## 📚 Multi-Model APIs

### 1. Key/Value Engine

CoCo's Key/Value engine uses lexicographically-ordered B+Trees, supporting prefix scans, atomic increments, and automatic TTL expiration.

```go
bucket := db.Bucket("cache")

// Put with optional TTL (Time-To-Live)
_ = bucket.Put([]byte("session:usr_99"), []byte("tok_abc"), coco.TTL(15*time.Minute))

// Get key
val, err := bucket.Get([]byte("session:usr_99"))

// Atomic integer counter (+1, +5, -1)
newVal, _ := bucket.Increment([]byte("metrics:page_views"), 1)

// Prefix scan (iterating all keys starting with "session:")
it := bucket.Prefix([]byte("session:"))
for it.Valid() {
    fmt.Printf("Key: %s, Value: %s\n", it.Key(), it.Value())
    it.Next()
}
it.Close()

// Delete key
_ = bucket.Delete([]byte("session:usr_99"))
```

---

### 2. Document Collections & Queries

Documents are stored in a high-performance binary format providing zero-allocation byte projections and secondary indexing.

```go
products := db.Collection("products")

// Create secondary index on category
_ = products.CreateIndex(
    coco.Index("idx_category").On("category"),
)

// Insert document (auto-generates 16-byte hex _id if omitted)
docID, _ := products.Insert(coco.Document{
    "name":     "Mechanical Keyboard",
    "category": "electronics",
    "price":    129.99,
    "rating":   4.8,
    "in_stock": true,
})

// Query with predicates, ordering, and limits
results, err := products.Query().
    Where("category").Eq("electronics").
    Where("price").Lt(200.0).
    OrderBy("price", coco.Desc).
    Limit(10).
    All()

// Inspect Volcano Physical Execution Plan
plan, _ := products.Query().Where("category").Eq("electronics").Explain()
fmt.Println(plan)
// Output: Filter -> IndexScan(index=idx_category)

// Update document
_ = db.Update(func(tx *coco.Tx) error {
    col := tx.Collection("products")
    doc, _ := col.Get(docID)
    doc["price"] = 119.99
    return col.Replace(docID, doc)
})
```

---

### 3. Vector Similarity Search (HNSW)

CoCo embeds a persistent **Hierarchical Navigable Small World (HNSW)** graph index for approximate nearest neighbor similarity searches.

```go
import "cocodb/internal/vector"

// Configure HNSW graph with Cosine metric
cfg := vector.DefaultHNSWConfig(4) // 4 dimensions
cfg.Metric = vector.Cosine         // vector.Cosine, vector.L2, or vector.DotProduct
hnsw := vector.NewHNSW(cfg)

// Insert vector embeddings
_ = hnsw.Insert(1, []float32{0.92, 0.15, 0.22, 0.08}) // AI/ML Paper
_ = hnsw.Insert(2, []float32{0.14, 0.95, 0.82, 0.11}) // Database Book

// Search for Top-2 nearest neighbors
queryVec := []float32{0.95, 0.12, 0.15, 0.02}
matches := hnsw.Search(queryVec, 2)

for _, m := range matches {
    similarity := (1.0 - m.Distance) * 100.0
    fmt.Printf("Doc #%d | Distance: %.4f | Similarity: %.1f%%\n", m.ID, m.Distance, similarity)
}
```

---

### 4. Full-Text Search (BM25 Inverted Index)

```go
import "cocodb/search"

idx := search.NewInvertedIndex()

// Index documents
idx.IndexDoc(1, "CoCo is a modern high-performance embedded multi-model database in Pure Go.")
idx.IndexDoc(2, "PostgreSQL and SQLite are widely used relational databases.")

// Search with BM25 ranking
results := idx.Search("embedded database go", 5)
for _, res := range results {
    fmt.Printf("Doc #%d | BM25 Score: %.3f\n", res.RecordID, res.Score)
}
```

---

### 5. ACID Multi-Bucket / Multi-Collection Transactions

CoCo provides serializable multi-bucket updates and snapshot isolation reads:

```go
// Read-Write Transaction
err := db.Update(func(tx *coco.Tx) error {
    accounts := tx.Bucket("accounts")
    logs := tx.Collection("audit_logs")

    // Update balances
    _ = accounts.Put([]byte("acc:alice"), []byte("800"))
    _ = accounts.Put([]byte("acc:bob"), []byte("700"))

    // Log transaction
    _, _ = logs.Insert(coco.Document{
        "event":  "transfer",
        "amount": 200,
        "from":   "alice",
        "to":     "bob",
    })
    return nil // Commit automatically
})

// Read-Only Transaction (Snapshot Isolation)
err = db.View(func(tx *coco.Tx) error {
    accounts := tx.Bucket("accounts")
    bal, _ := accounts.Get([]byte("acc:alice"))
    fmt.Printf("Alice Balance: $%s\n", string(bal))
    return nil
})
```

---

### 6. Transactional Queue & Distributed Deduplication

CoCo features a durable, crash-safe task queue with **Distributed Exactly-Once Deduplication**, lease visibility timeouts, auto-failover, and Dead-Letter Queues (DLQ):

```go
queue := db.Queue("order_processing")
ctx := context.Background()

// Enqueue with Deduplication ID (Duplicate attempts from any worker are rejected)
msg, err := queue.Enqueue(ctx, []byte(`{"order_id": "998811", "amount": 149.99}`),
    coco.WithDedupID("order_998811", 10*time.Minute), // 10-min deduplication window
    coco.WithPriority(200),                          // Priority 0-255
    coco.WithMaxRetries(3),                          // Max failures before DLQ
)

// Worker Dequeue with Visibility Timeout Lease
task, err := queue.Dequeue(ctx, coco.WithVisibilityTimeout(30*time.Second))
if err == nil {
    // Process task...
    _ = task.Ack() // Complete and remove from queue
}
```

---

### 7. Real-Time Pub/Sub with Wildcards & Consumer Groups

High-throughput in-memory topic broker with single-segment (`*`) and multi-segment (`>`) pattern matching and **Consumer Groups** for competing worker load distribution:

```go
ps := db.PubSub()
ctx := context.Background()

// 1. Direct Broadcast Subscriber (Wildcard matching all sensor metrics)
auditSub := ps.Subscribe(ctx, "sensors.>")
defer auditSub.Unsubscribe()

// 2. Distributed Consumer Group (Each event delivered to only 1 worker in the group)
workerSub1 := ps.SubscribeGroup(ctx, "sensors.temperature", "temp_workers")
workerSub2 := ps.SubscribeGroup(ctx, "sensors.temperature", "temp_workers")

// 3. Publish with Deduplication Window
_, _ = ps.Publish(ctx, "sensors.temperature", []byte(`{"temp": 23.5}`),
    coco.WithPubDedupID("event_sn101_ts1", 1*time.Minute),
)
```

---

### 8. Secure Distributed Cluster & Multi-Node Deduplication

CoCoDB includes a high-performance **Distributed Cluster Subsystem** with Mutual TLS 1.3 (`mTLS`), Secret Token Authentication, Consistent Hash Routing, and multi-node task deduplication:

```go
import (
    coco "cocodb"
    "cocodb/cluster"
)

// 1. Start a Secure Cluster Node in 3 lines of code
nodeTLS, clientTLS := cluster.WithDevmTLS("127.0.0.1", "localhost") // Zero-setup mTLS

node, err := cluster.StartNode(db, "127.0.0.1:9001",
    cluster.WithSecret("cluster_secret_token_123"), // Token Authentication
    cluster.WithPeers("127.0.0.1:9002", "127.0.0.1:9003"),
    nodeTLS,
)
defer node.Close()

// 2. Connect Distributed Client
client, err := cluster.Dial([]string{"127.0.0.1:9001", "127.0.0.1:9002"},
    cluster.WithSecret("cluster_secret_token_123"),
    clientTLS,
)
defer client.Close()

// 3. Enqueue Task with Cross-Cluster Deduplication (Idempotency Guarantee)
queue := client.Queue("distributed_payments")
msg, err := queue.Enqueue(ctx, []byte(`{"order": 9988}`),
    coco.WithDedupID("order_9988", 10*time.Minute), // Duplicate attempts rejected across all nodes!
    coco.WithPriority(200),
)

// 4. Worker Dequeue & Acknowledge
task, _ := queue.Dequeue(ctx, coco.WithVisibilityTimeout(30*time.Second))
_ = task.Ack()
```

---

## 🎨 Embedded Admin Studio

CoCo includes a built-in web management interface built with **React 19**, **TypeScript**, and **Tailwind CSS v4**.

```go
import "cocodb/studio"

srv := studio.NewServer(db, ":8787")
_ = srv.Start()
```

Open **`http://localhost:8787`** in any modern web browser to access:

| View | Description |
| :--- | :--- |
| 📊 **Dashboard** | Live kernel metrics, allocated size, 16-partition LRU cache hit rate, and WAL LSN sequence counter. |
| 🗄️ **Collections** | NoSQL document data table, dynamic schema inspector, interactive filter builder, and JSON tree editor. |
| 🔑 **KV Buckets** | Real-time Key/Value browser, prefix scanner, TTL countdown chips, and atomic increment tester. |
| 📦 **Task Queues** | Durable task scheduler, visibility timeout lease tracker, worker dequeue simulator, and DLQ inspector. |
| 📢 **Pub/Sub Broker** | Real-time topic event publisher with deduplication keys, live broadcast event stream, and Consumer Group telemetry. |
| ⚡ **Query Console** | Interactive fluent query runner with live Go code generator and visual Volcano execution plan tree. |
| 🧠 **Vector Playground** | Embedding similarity search tester with Cosine/L2/Dot toggles and similarity percentage bars. |
| 🔍 **Full-Text Search** | Unicode token normalizer tester and BM25 relevance score breakdown. |
| 🛠️ **Integrity & Tools** | One-click kernel validator (`db.Check()`), force WAL checkpoint, and point-in-time snapshot backup creator. |
| ☀️ / 🌙 **Dual Theme** | Instant Light & Dark mode toggle with `localStorage` persistence. |

---

## 📈 Dedicated Observability & Metrics Dashboard

For production monitoring, real-time performance sparklines, and Prometheus scraping, CoCoDB provides a dedicated standalone Observability Dashboard:

```go
import "cocodb/dashboard"

srv := dashboard.NewServer(db, ":9090")
_ = srv.Start()
```

Open **`http://localhost:9090`** to view:
- ⚡ **Live Real-Time Charts (SSE 500ms Stream)**: Live Operations/sec (QPS), 16-partition LRU cache hit ratio, single-file storage footprint, and WAL progression.
- 📦 **Task Queues & Pub/Sub Telemetry**: Ready tasks, in-flight leases, and Dead-Letter Queue (DLQ) alerts.
- 🔥 **Synthetic Benchmark Probe**: Trigger 1-second in-memory latency benchmarks measuring P50 & P99 latencies in microseconds.
- 📡 **Prometheus Exporter**: Direct `/metrics` scrape endpoint compatible with Prometheus, Grafana Agent, and OpenTelemetry.

---

## 💡 Runnable Examples

Complete, standalone examples are located under [`examples/`](file:///Users/mrzack/SourceCodes/cocodb/examples/):

```bash
# 1. Key/Value Engine, TTL, Prefix Scans & Atomic Increments
go run examples/01_basic_kv/main.go

# 2. Document Collections, Secondary Indexes, Queries & Volcano Plans
go run examples/02_document_query/main.go

# 3. Persistent HNSW Vector Graph Similarity Search
go run examples/03_vector_similarity/main.go

# 4. Unicode Tokenizer & BM25 Full-Text Search
go run examples/04_fulltext_bm25/main.go

# 5. Interactive Live Admin Studio Web Interface
go run examples/05_admin_studio/main.go [optional_database_path]

# 6. Task Queue with Deduplication & Real-Time Pub/Sub with Consumer Groups
go run examples/06_queue_and_pubsub/main.go

# 7. Secure Distributed Cluster with 3 Nodes, mTLS 1.3 & Cross-Node Deduplication
go run examples/07_distributed_cluster/main.go

# 8. Standalone Real-Time Observability Dashboard & Prometheus Exporter
go run examples/08_observability_dashboard/main.go
```

---

## ⚙️ Configuration & Profiles

```go
db, err := coco.Open("production.coco",
    // Performance Profiles
    coco.Profile(coco.Performance), // Tiny (8MB), Balanced (64MB), Performance (256MB)

    // WAL Synchronization Modes
    coco.SyncMode(coco.SyncNormal),  // SyncFull (fsync each commit), SyncNormal, SyncOff

    // Memory Limit (LRU Page Cache)
    coco.MemoryLimit(128 * 1024 * 1024), // 128 MB cache

    // Read-Only Mode
    coco.ReadOnly(),

    // AES-256-GCM Encryption at Rest
    coco.EncryptionKey([]byte("32-byte-secret-encryption-key--")),
    coco.EncryptionKeyID("key-v1"),

    // Background Cleaner & Maintenance
    coco.Background(true),
)
```

---

## 🩺 Diagnostic & Maintenance Tools

```go
// 1. Storage Kernel Integrity Check
report, err := db.Check(context.Background())
if err == nil && report.Valid {
    fmt.Printf("Integrity OK: %d pages validated with CRC32C\n", report.PagesChecked)
}

// 2. Real-Time Telemetry Stats
stats := db.Stats()
fmt.Printf("Pages: %d | Hit Rate: %.1f%% | LSN: #%d\n",
    stats.PageCount, stats.CacheHitRate*100, stats.LastLSN)

// 3. Hot Snapshot Backup
_ = db.Backup(context.Background(), "backup-2026.coco")
```

---

## ⚡ Benchmarks & High Performance

Benchmarked on Apple Silicon (ARM64) running Go 1.24 with zero external dependencies:

| Benchmark Operation | Latency (`ns/op`) | Throughput (`ops/sec`) | Allocations | Memory / Op |
| :--- | :--- | :--- | :--- | :--- |
| **KV Sequential Get (B+Tree)** | **242.7 ns/op** | **~4,120,000 gets/sec** | **2 allocs/op** | **71 B/op** |
| **KV Parallel Get (B+Tree)** | **284.8 ns/op** | **~3,511,000 gets/sec** | **2 allocs/op** | **71 B/op** |
| **MVCC Concurrent Reads** | **416.0 ns/op** | **~2,403,000 reads/sec** | **7 allocs/op** | **416 B/op** |
| **Pub/Sub Publish & Broadcast** | **512.0 ns/op** | **~1,953,000 broadcasts/sec** | **4 allocs/op** | **164 B/op** |
| **Document Insert (Binary Slotted)** | **737.1 ns/op** | **~1,356,000 inserts/sec** | **30 allocs/op** | **1.6 KB/op** |
| **Document Query (Volcano Plan)** | **468.2 ns/op** | **~2,135,000 queries/sec** | **20 allocs/op** | **1.0 KB/op** |
| **Transactional Queue Enqueue/Dequeue** | **1,293 ns/op** | **~773,000 tasks/sec** | **23 allocs/op** | **1.7 KB/op** |
| **Vector Cosine Search (128D HNSW)** | **11,595 ns/op** | **~86,240 ANN searches/sec** | **14 allocs/op** | **3.9 KB/op** |
| **Full-Text BM25 Ranking** | **52,071 ns/op** | **~19,204 queries/sec** | **27 allocs/op** | **74.9 KB/op** |

To run the full benchmark suite on your own hardware:
```bash
go test -bench=. -benchmem -run=^$ .
```

---

## 🧪 Testing & Verification

CoCoDB is rigorously verified using Go's built-in race detector across all packages:

```bash
go test -race ./...
```

```text
ok      cocodb                   1.994s
ok      cocodb/document          1.492s
ok      cocodb/internal/btree   14.962s
ok      cocodb/internal/crypto   2.756s
ok      cocodb/internal/storage  3.194s
ok      cocodb/internal/text     5.413s
ok      cocodb/internal/txn      4.525s
ok      cocodb/internal/vector   3.659s
ok      cocodb/internal/wal      4.023s
ok      cocodb/kv                2.826s
ok      cocodb/studio            1.545s
```

---

## 📄 License
 
CoCo is open-source software licensed under the **Apache License, Version 2.0**. See the [LICENSE](LICENSE) file for more details.
