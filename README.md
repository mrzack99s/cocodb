# CoCoDB

CoCoDB is a pure-Go embedded database that keeps application data in a single database file. It combines key/value storage, document collections, vector search, full-text search, queues, and pub/sub behind one transactional API.

[![Go](https://img.shields.io/badge/Go-1.26.5%2B-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache--2.0-blue)](LICENSE)
[![CGO](https://img.shields.io/badge/CGO-not_required-success)](#requirements)

## Highlights

- ACID transactions with crash recovery
- Key/value buckets and document collections
- HNSW vector similarity and BM25 full-text search
- Durable queues and in-process pub/sub
- Optional Admin Studio and metrics dashboard
- No CGO or external database service required

## Use cases

CoCoDB is intended for Go applications that need local, transactional storage without operating a separate database server. It can support desktop tools, edge services, application caches, search-enabled datasets, background jobs, and small standalone services.

## Components

| Component | Purpose |
| --- | --- |
| Key/value | Store and scan byte keys and values, with optional TTL |
| Documents | Work with structured records, indexes, and fluent queries |
| Vector search | Find similar vectors using HNSW indexes |
| Full-text search | Rank text matches using BM25 |
| Queues | Process durable background tasks with acknowledgements and retries |
| Pub/sub | Distribute events through topics and consumer groups |
| Admin Studio | Inspect and manage a database from a browser |
| Dashboard | View runtime metrics and expose Prometheus metrics |

## Requirements

- Go 1.26.5 or later
- No CGO toolchain or external database server

## Quick start

```go
package main

import (
	"log"

	coco "github.com/mrzack99s/cocodb"
)

func main() {
	db, err := coco.Open("app.coco", coco.Profile(coco.Balanced))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	users := db.Bucket("users")
	if err := users.Put([]byte("user:1"), []byte("Ada")); err != nil {
		log.Fatal(err)
	}
}
```

Opening the same path later reuses the existing database. Always close the database before the process exits so pending work can finish cleanly.

## Transactions and documents

Use `Update` for an atomic write transaction and `View` for a consistent read transaction:

```go
err := db.Update(func(tx *coco.Tx) error {
	accounts := tx.Bucket("accounts")
	if err := accounts.Put([]byte("alice"), []byte("800")); err != nil {
		return err
	}
	return accounts.Put([]byte("bob"), []byte("700"))
})
```

Document collections provide a higher-level API for structured data:

```go
users := db.Collection("users")

_, err := users.Insert(coco.Document{
	"name":   "Ada",
	"role":   "engineer",
	"active": true,
})

results, err := users.Query().
	Where("active").Eq(true).
	Limit(20).
	All()
```

## Configuration

Common options can be passed when opening a database:

```go
	db, err := coco.Open("app.coco",
	coco.Profile(coco.Balanced),
	coco.SyncMode(coco.SyncNormal),
	coco.MemoryLimit(128*1024*1024),
	coco.Background(true),
)
```

Profiles provide convenient defaults, while individual options allow applications to tune durability, memory use, encryption, background maintenance, and read-only access.

The storage used by Key/Value buckets (and all other models in the database)
is configurable. `StorageAuto` is the default: it uses RAM for `:memory:` and
disk for a file path. Use `StorageMemory` to force RAM, `StorageDisk` to force
a path-backed database, or `CustomStorage` to provide an implementation such
as encrypted or remote storage:

```go
db, err := coco.Open("cache.coco",
	coco.Storage(coco.StorageMemory),
	coco.CustomProfile(coco.ProfileConfig{
		MemoryLimit:   256 * 1024 * 1024,
		SyncMode:      coco.SyncOff,
		Background:    true,
		CleanInterval: time.Second,
	}),
)
```

`CustomProfile` lets applications define their own memory, durability, and
maintenance defaults; explicit options passed after it override those values.

### Per-model storage

Use `DefaultDisk()` or `DefaultMemory()` when every model should use one
storage kind. Override individual models when their data has a different
durability requirement:

```go
db, err := coco.Open("app.coco",
	coco.DefaultDisk(),
	coco.KVStorage(coco.StorageMemory),
	coco.DocumentStorage(coco.StorageDisk),
	coco.QueueStorage(coco.StorageDisk),
)
```

An overridden disk model uses its own sibling database file (for example,
`app.coco-kv`); memory-backed models are deliberately not retained after
`Close`. `KVCustomStorage`, `DocumentCustomStorage`, and `QueueCustomStorage`
accept a `BackendFactory` for other storage implementations. Because these are
separate engines, an `Update` spanning more than one configured model commits
each engine independently rather than as one cross-store ACID commit.

### Multiple writer processes

For more than one process writing the same database path, opt in to an
isolated write session per `Update`:

```go
db, err := coco.Open("app.coco",
	coco.DefaultDisk(),
	coco.MultiWriter(),
	coco.WriterTimeout(30*time.Second),
)
```

`MultiWriter` serializes writers by opening a fresh, exclusively locked engine
for each update, so a process commits against the latest recovered WAL state.
Use `Update` for every write; direct mutable collection/bucket handles are not
safe to share between processes. Background maintenance is disabled in this
mode because it would retain stale page state.

One open database instance supports many simultaneous read transactions and
serializes concurrent write transactions to preserve ACID guarantees. Opening
the same disk path with `MultiWriter` is supported; without it, a second writer
remains deliberately blocked.

## Examples

Runnable examples are available in [examples](examples/):

| Example | Demonstrates |
| --- | --- |
| `01_basic_kv` | Key/value operations and transactions |
| `02_document_query` | Collections, indexes, and queries |
| `03_vector_similarity` | Vector similarity search |
| `04_fulltext_bm25` | Full-text indexing and ranking |
| `05_admin_studio` | Browser-based database management |
| `06_queue_and_pubsub` | Background tasks and event delivery |
| `07_distributed_cluster` | Secure multi-node operation |
| `08_observability_dashboard` | Metrics dashboard and Prometheus endpoint |

```bash
go run examples/01_basic_kv/main.go
go run examples/05_admin_studio/main.go
```

## Optional web interfaces

The Admin Studio provides a browser-based interface for exploring data, running queries, viewing search results, and performing common maintenance tasks.

```bash
go run examples/05_admin_studio/main.go -db app.coco -addr :8787
```

Open `http://localhost:8787` after the server starts.

The separate observability dashboard focuses on database health and runtime metrics:

```bash
go run examples/08_observability_dashboard/main.go
```

It includes a Prometheus-compatible `/metrics` endpoint for integration with monitoring systems.

## Development

```bash
go test ./...
go test -race ./...
go test -bench=. -benchmem -run=^$ .
```

The frontend sources for the Admin Studio and dashboard are located under `studio/frontend` and `dashboard/frontend`.

## License

Licensed under the [Apache License 2.0](LICENSE).
