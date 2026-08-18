# CoCo Embedded Multi-Model Database
## Engineering Handoff & Implementation Plan

> **Project codename:** CoCo  
> **Primary language:** Go  
> **Primary deployment models:** Embedded / In-process, and Distributed Edge (optional modules)  
> **Core philosophy:** Pure Go, no CGO, minimal dependencies, predictable performance, ACID, single-file persistent storage, multi-model on one transactional kernel.

---

# 1. Executive Summary

CoCo is a **high-performance embedded multi-model database engine written in Go**.

The database must be usable directly inside an application without requiring:

- a daemon
- a database server process
- a network port
- an external service
- an external storage engine
- CGO
- a JVM
- a cluster

CoCo provides four first-class data models on top of the same storage and transaction kernel:

1. Ordered Key/Value
2. Document / NoSQL
3. Vector Search
4. Full-Text Search

All models share:

- Pager
- Page Cache
- B+Tree
- Record Store
- WAL
- Transactions
- MVCC
- Snapshot Isolation
- Secondary Indexes
- Backup
- Recovery
- Integrity Checking
- Maintenance
- Query Planner

The intended positioning is:

> **A lightweight transactional embedded multi-model database for Go applications, with optional distributed Edge execution over gRPC.**

A shorter product description:

> **Document + KV + Vector + Full-Text in one embedded ACID engine.**

---

# 2. Product Goals

## 2.1 Primary Goals

CoCo MUST be:

- Embedded-first
- Pure Go
- No CGO
- Lightweight
- Crash-safe
- ACID
- Encrypted at rest when configured
- Fast for point reads
- Fast for indexed range queries
- Efficient for local application workloads
- Low-maintenance
- Easy to embed
- Easy to distribute as part of a Go application
- Single database file where practical
- Predictable under mixed read/write workloads
- Capable of metadata + vector hybrid search
- Scale read and vector-search workloads across multiple Edge nodes without changing the embedded kernel
- Expose a single idiomatic Go API for embedded use; no separate client packages or language bindings are in scope
- Use GPU acceleration for vector workloads when an Edge advertises a compatible runtime, with a correct CPU fallback

## 2.2 Multi-Model Goals

The engine MUST expose:

### Key/Value

- Get
- Put
- Delete
- Prefix Scan
- Range Scan
- Iteration
- Atomic batch
- Compare-and-swap
- Increment/decrement
- TTL

### Document / NoSQL

- Schemaless documents
- Optional schema validation
- Nested documents
- Arrays
- CRUD
- Secondary indexes
- Compound indexes
- Unique indexes
- Partial indexes
- Sparse indexes
- Range queries
- Sort
- Projection
- Aggregation
- Explain

### Vector

- float32 vector type
- exact nearest-neighbor search
- cosine distance
- L2 distance
- inner product
- persistent HNSW
- metadata-filtered vector search
- vector quantization in a later milestone
- optional GPU-accelerated exact search, ANN candidate generation, and reranking
- deterministic CPU fallback for every GPU-accelerated operation

### Full-Text

- tokenization
- normalization
- inverted index
- BM25
- full-text + metadata filtering
- hybrid text/vector search in a later milestone

---

# 3. Scope Boundaries and Non-Goals for the Embedded Core

The embedded core MUST NOT contain network or device-runtime concerns. It MUST NOT contain:

- Distributed consensus implementation
- Raft or Paxos implementation
- Cluster membership implementation
- Leader election implementation
- Shard-placement policy
- Cross-region replication implementation
- Distributed transactions
- Multi-master conflict resolution
- HTTP or gRPC server implementation
- SQL wire protocol
- PostgreSQL wire protocol
- Authentication server
- RBAC server
- Service discovery
- GPU driver bindings or GPU vector runtime implementation
- Remote query coordinator implementation

Distributed execution, replication, server mode, and GPU acceleration MUST be separate modules. The distributed v1 architecture is intentionally **single-writer per shard**: it provides local ACID transactions at an Edge, idempotent routed writes, and federated reads/search. Atomic transactions across shards are deferred until a separately designed consensus/transaction protocol exists.

Example:

```text
coco/
coco-server/
coco-edge/
coco-coordinator/
coco-proto/
coco-vector-gpu/
coco-sync/
coco-replication/
```

The embedded database kernel must remain usable without them.

---

# 4. Core Architecture Decisions

## 4.1 Primary Storage Structure

Use:

```text
Paged B+Tree
+
Slotted Pages
+
Write-Ahead Log
+
MVCC
+
Snapshot Isolation
+
Managed Page Cache
```

Do NOT use an LSM Tree as the primary v1 storage engine.

### Rationale

B+Tree better matches the initial embedded goals:

- single database file
- ordered KV
- range queries
- secondary indexes
- predictable latency
- no compaction spikes
- easier page-level recovery model
- lower operational complexity
- natural support for sorted indexes

---

# 5. High-Level Architecture

```text
Application
     |
     v
+--------------------------------------------------+
|                    CoCo API                      |
|                                                  |
| KV | Document | Vector | Search | Transaction    |
+--------------------------+-----------------------+
                           |
                           v
+--------------------------------------------------+
|                 Query Engine                     |
|                                                  |
| Expressions                                      |
| Logical Plan                                     |
| Optimizer                                        |
| Physical Plan                                    |
| Executor                                         |
| Aggregation                                      |
+--------------------------+-----------------------+
                           |
          +----------------+----------------+
          |                |                |
          v                v                v
      B+Tree Index      HNSW Index      Text Index
          |                |                |
          +----------------+----------------+
                           |
                           v
+--------------------------------------------------+
|                 Record Layer                     |
|                                                  |
| RecordID                                         |
| CSON                                             |
| Version Chain                                    |
| Overflow / Blob                                  |
+--------------------------+-----------------------+
                           |
                           v
+--------------------------------------------------+
|               Transaction Engine                 |
|                                                  |
| MVCC                                             |
| Snapshot Isolation                               |
| Reader Table                                     |
| Single Writer Coordinator                        |
| Savepoints                                       |
+--------------------------+-----------------------+
                           |
                           v
+--------------------------------------------------+
|                 Storage Kernel                   |
|                                                  |
| Pager                                            |
| Page Cache                                       |
| Slotted Pages                                    |
| B+Tree                                           |
| Allocator                                        |
| Free List                                        |
| Overflow Pages                                   |
| Checksums                                        |
+--------------------------+-----------------------+
                           |
                           v
+--------------------------------------------------+
|                        WAL                       |
|                                                  |
| Append                                            |
| Recovery                                          |
| Checkpoint                                        |
| Group Commit                                      |
+--------------------------+-----------------------+
                           |
                           v

                 application.coco
                 application.coco-wal
                 application.coco-lock
```

---

# 6. Deployment Model

Typical application:

```go
db, err := coco.Open("app.coco")
if err != nil {
    panic(err)
}
defer db.Close()
```

In-memory:

```go
db, err := coco.Open(":memory:")
```

Read-only:

```go
db, err := coco.Open(
    "catalog.coco",
    coco.ReadOnly(),
)
```

The standard deployment MUST require no background service.

---

# 7. File Layout

Default persistent files:

```text
app.coco
app.coco-wal
app.coco-lock
```

The main `.coco` file contains fixed-size pages.

Default page size:

```text
16 KiB
```

v1 should initially lock the page size to 16 KiB.

Future format versions may support:

```text
4 KiB
8 KiB
16 KiB
32 KiB
```

---

# 8. Persistent Primitive Types

Persistent identifiers MUST use explicit-width types.

```go
type PageID uint64
type SlotID uint16
type RecordID uint64
type TxnID uint64
type LSN uint64
type ObjectID uint64
type FieldID uint32
```

Do NOT persist Go `int`.

---

# 9. Database Meta Pages

Use two metadata pages:

```text
Page 0 = Meta A
Page 1 = Meta B
```

This provides crash-safe metadata switching.

Proposed structure:

```go
type MetaPage struct {
    Magic             [8]byte
    FormatVersion     uint32
    PageSize          uint32

    Generation        uint64

    DatabaseID        [16]byte

    CatalogRoot       PageID
    RecordDirRoot     PageID
    FreeListRoot      PageID

    NextPageID        PageID
    NextRecordID      RecordID
    LastTxnID         TxnID
    LastLSN           LSN
    LastCheckpointLSN LSN

    Flags             uint64

    Checksum          uint32
}
```

Magic:

```text
COCODB01
```

Open logic:

```text
Read Meta A
Read Meta B

Validate:
- magic
- version
- page size
- checksum

Select valid page with highest Generation.
```

Metadata update:

```text
write inactive meta page
sync
switch generation
```

---

# 10. Page Types

```go
type PageType uint8

const (
    PageMeta PageType = iota

    PageBTreeInternal
    PageBTreeLeaf

    PageRecord
    PageOverflow

    PageFreeList

    PageVector
    PageText
)
```

---

# 11. Page Header

Proposed:

```go
type PageHeader struct {
    ID        PageID
    Type      PageType
    Flags     uint16

    LSN       LSN

    SlotCount uint16
    FreeStart uint16
    FreeEnd   uint16

    Checksum  uint32
}
```

Persistent page serialization MUST NOT use direct Go struct memory layout.

Always encode fields explicitly using `encoding/binary`.

---

# 12. Slotted Page

Page layout:

```text
+--------------------------------+
| Page Header                    |
+--------------------------------+
| Slot 0                         |
| Slot 1                         |
| Slot 2                         |
| ...                            |
|                                |
|          Free Space            |
|                                |
| Record C                       |
| Record B                       |
| Record A                       |
+--------------------------------+
```

Slot:

```go
type Slot struct {
    Offset uint16
    Length uint16
}
```

Required operations:

```text
Insert
Get
Update
Delete
Compact
FreeSpace
SlotCount
```

Deletion SHOULD mark a slot unused before compaction.

Compaction MUST preserve logical slot references where required or update the owning page's slot metadata atomically.

---

# 13. Backend Abstraction

No upper database layer may access `os.File` directly.

Internal interface:

```go
type Backend interface {
    ReadAt(p []byte, off int64) (int, error)
    WriteAt(p []byte, off int64) (int, error)

    Sync() error
    Truncate(size int64) error
    Size() (int64, error)

    Close() error
}
```

Implementations:

```text
OSBackend
MemoryBackend
```

This allows:

```text
persistent DB
in-memory DB
```

to use the exact same storage kernel.

---

# 14. Pager

Proposed internal API:

```go
type Pager interface {
    Get(PageID) (*Page, error)

    Allocate(PageType) (*Page, error)
    Free(PageID) error

    MarkDirty(*Page)

    Flush(PageID) error
    FlushAll() error
}
```

Responsibilities:

- page address calculation
- backend I/O
- checksum validation
- cache integration
- allocation
- dirty tracking
- WAL ordering enforcement

---

# 15. Page Allocation

Initial implementation:

```text
Free List
  |
  +--> reuse free page
  |
  +--> otherwise allocate NextPageID
```

Do not implement complicated extent allocation in v1.

Later:

- extents
- free-page batching
- locality-aware allocation

---

# 16. Page Cache

Implement a managed cache to control database memory independently from the OS cache.

Use a sharded architecture.

Recommended initial shard count:

```text
16
```

Potential cache entry:

```go
type CacheEntry struct {
    ID PageID

    Page *Page

    Pins int32

    Dirty bool

    PageLSN LSN
}
```

Required operations:

```text
Pin
Unpin
Get
Insert
MarkDirty
Evict
Flush
```

Initial eviction policy:

```text
LRU
```

Do not over-optimize initially.

Potential later replacements:

```text
CLOCK
CLOCK-Pro
2Q
```

only after benchmarks prove a need.

---

# 17. Memory Management

Public option:

```go
coco.MemoryLimit(128 * coco.MB)
```

Suggested starting budget:

```text
Page Cache   70%
Vector       20%
Query        10%
```

Budget MUST be dynamic.

Unused vector budget can return to the page cache.

Profiles:

```text
Tiny
Balanced
Performance
```

Example:

```go
db, err := coco.Open(
    "app.coco",
    coco.Profile(coco.Balanced),
)
```

Suggested initial profile defaults:

### Tiny

```text
Page cache: 4-8 MB
Workers: 1
Vector cache: minimum
Background work: conservative
```

### Balanced

```text
Page cache: 64 MB
Workers: 2
```

### Performance

```text
Cache: derived from configured memory limit
Workers: derived from runtime.NumCPU()
```

---

# 18. B+Tree

The B+Tree must be written in-house.

It is used by:

- KV data
- Primary indexes
- Secondary indexes
- Catalog
- Record directory
- TTL index
- Text term dictionary
- statistics metadata

Node classes:

```text
Internal
Leaf
```

Leaf pages SHOULD be doubly linked:

```text
PrevLeaf
NextLeaf
```

This enables efficient:

- forward iteration
- reverse iteration
- range scans
- prefix scans

---

# 19. Required B+Tree Features

Implementation is not considered complete until all of the following work:

```text
Search
Insert
Delete

Leaf split
Internal split

Borrow left
Borrow right

Merge left
Merge right

Root split
Root collapse

Seek
SeekFirst
SeekLast

Next
Prev

Range Scan
Reverse Range Scan
Prefix Scan
```

---

# 20. B+Tree Cursor

Suggested API:

```go
type Cursor struct {
    // internal state
}

func (c *Cursor) Seek(key []byte) bool
func (c *Cursor) First() bool
func (c *Cursor) Last() bool
func (c *Cursor) Next() bool
func (c *Cursor) Prev() bool

func (c *Cursor) Key() []byte
func (c *Cursor) Value() []byte
func (c *Cursor) Err() error
```

Cursor allocations should be minimized.

---

# 21. Key Encoding

A central key encoding system is mandatory.

Lexicographic byte order MUST match logical sort order.

Do not encode ordered integers as raw little-endian values.

Supported values:

```text
Null
Bool

Int
Uint

Float32
Float64

String
Bytes

Timestamp
UUID

Compound keys
```

Suggested component encoding:

```text
[type][payload]
[type][payload]
[type][payload]
```

Escaping or length encoding MUST preserve component boundaries.

Compound index example:

```text
country
age
created_at
record_id
```

A RecordID SHOULD be appended to non-unique secondary index keys to guarantee uniqueness.

---

# 22. Record Identity

Every stored document/value has a stable internal identity:

```go
type RecordID uint64
```

Indexes SHOULD point to RecordID rather than physical PageID/SlotID.

Benefits:

- records may move
- vacuum may relocate records
- secondary index references remain valid
- HNSW nodes remain stable
- full-text postings remain stable

---

# 23. Record Directory

Map:

```text
RecordID -> PageID + SlotID
```

Structure:

```go
type RecordLocation struct {
    Page PageID
    Slot SlotID
}
```

Store the directory in a B+Tree.

Hot RecordID locations MAY later be cached.

Do not implement a complex record directory cache before profiling.

---

# 24. Record Header

Proposed:

```go
type RecordHeader struct {
    RecordID RecordID

    BeginTxn TxnID
    EndTxn   TxnID

    PrevVersion RecordID

    Flags uint16

    PayloadLength uint32
}
```

Potential flags:

```text
Deleted
Overflow
Compressed
Encrypted
```

---

# 25. Large Records and Overflow Pages

If a payload is too large for a normal record page, store it through overflow pages.

Logical structure:

```text
Record
  |
  v
Overflow Page 1
  |
  v
Overflow Page 2
```

Initial overflow threshold:

```text
~25% of page size
```

This MUST be benchmark-tuned later.

---

# 26. Streaming Blob API

Large binary objects should not require loading the entire object into memory.

Potential API:

```go
w, err := collection.CreateBlob("object-key")
if err != nil {
    return err
}

_, err = io.Copy(w, src)
if err != nil {
    return err
}

return w.Close()
```

Read:

```go
r, err := collection.OpenBlob("object-key")
```

Blob support may be implemented after core document storage but the underlying overflow format should support it.

---

# 27. Write-Ahead Log

The WAL is a mandatory v1 component.

Suggested record header:

```go
type WALRecordHeader struct {
    Magic   uint32
    Version uint16
    Type    WALRecordType

    Length uint32

    LSN   LSN
    TxnID TxnID

    CRC uint32
}
```

Initial WAL record types:

```text
TxnBegin
PageAlloc
PageFree
PageUpdate
TxnCommit
TxnAbort
Checkpoint
```

---

# 28. WAL Safety Rule

The engine MUST enforce:

> WAL records corresponding to a dirty page must be durable before the dirty page is written to the main database file.

Equivalent:

```text
DurableWAL >= PageLSN
```

before page flush.

This rule MUST be represented explicitly in the pager/WAL integration rather than relying on call ordering conventions.

---

# 29. WAL Strategy

Start with a redo-oriented WAL.

A page update record can initially contain:

```text
PageID
offset
length
new bytes
```

or, during initial implementation, a full page image if simplicity is needed.

Recommended progression:

### Stage 1

```text
Full-page redo record
```

Advantages:

- simple recovery
- simple correctness
- easier tests

Disadvantage:

- large WAL

### Stage 2

Introduce delta/page-fragment records after correctness is stable.

Do not prematurely optimize WAL size.

---

# 30. Recovery

Open sequence:

```text
Read Meta Pages
      |
      v
Validate Meta
      |
      v
Detect Existing WAL
      |
      v
Scan WAL
      |
      v
Validate CRC
      |
      v
Identify committed transactions
      |
      v
Redo committed changes
      |
      v
Ignore incomplete transactions
      |
      v
Checkpoint if necessary
      |
      v
Database Ready
```

The recovery path MUST be deterministic and idempotent.

Running recovery twice on the same valid state must produce the same database.

---

# 31. Checkpoint

Checkpoint moves durable state from WAL into main database pages.

High-level:

```text
Flush WAL
   |
   v
Flush eligible dirty pages
   |
   v
Write checkpoint record
   |
   v
Update Meta
   |
   v
Recycle/truncate WAL
```

Checkpoint MUST respect active snapshots and WAL requirements.

---

# 32. Sync Modes

Expose:

```go
type SyncMode int

const (
    SyncFull SyncMode = iota
    SyncNormal
    SyncOff
)
```

### SyncFull

Durability priority.

```text
WAL append
fsync
commit success
```

### SyncNormal

Group/batched durability.

### SyncOff

No crash durability guarantee.

Must be documented as unsafe for durable storage.

---

# 33. Writer Coordinator

The public API can accept writes from many goroutines.

Internally commit through one writer coordinator:

```text
goroutine A ----\
goroutine B -----+--> Writer Coordinator --> WAL --> Storage
goroutine C ----/
```

v1 concurrency model:

```text
Multiple concurrent readers
Single serialized writer
```

This is an intentional embedded-first design.

---

# 34. Group Commit

The writer coordinator SHOULD support group commit.

Example:

```text
Txn 101
Txn 102
Txn 103
   |
   v
single WAL batch
   |
   v
single fsync
```

Each transaction still receives an individual commit result.

Implement group commit only after basic commit correctness is complete.

---

# 35. Transactions

Public APIs:

```go
err := db.Update(func(tx *coco.Tx) error {
    return nil
})
```

```go
err := db.View(func(tx *coco.Tx) error {
    return nil
})
```

Manual:

```go
tx, err := db.Begin(coco.ReadWrite)
if err != nil {
    return err
}

defer tx.Rollback()

// operations...

return tx.Commit()
```

---

# 36. Isolation Model

v1 uses:

```text
Snapshot Isolation
```

Readers obtain a stable snapshot.

Read transactions should not observe uncommitted state.

---

# 37. MVCC Visibility

A version is visible when conceptually:

```text
BeginTxn <= SnapshotTxn
AND
(
    EndTxn == 0
    OR
    EndTxn > SnapshotTxn
)
```

The actual implementation MUST distinguish committed and uncommitted TxnIDs.

Do not infer commit state purely by numerical TxnID ordering.

Transaction manager metadata is the authority.

---

# 38. Version Chains

Update:

```text
RecordID 10 v1
   |
   v
RecordID 11 v2
   |
   v
RecordID 12 v3
```

or stable logical ID + version object.

The implementation may choose separate RecordIDs for physical versions while preserving one logical primary identity.

The important requirement:

- old snapshot can read old value
- new snapshot can read new value
- deleted versions remain visible until safe to reclaim

---

# 39. Reader Table

Transaction manager tracks active read snapshots.

Required operation:

```text
OldestActiveSnapshot()
```

This controls safe reclamation of:

- deleted versions
- obsolete versions
- pages no longer visible

---

# 40. Savepoints

Do not implement nested transactions.

Expose savepoints instead:

```go
err := tx.Savepoint("before-payment")
err = tx.RollbackTo("before-payment")
```

Savepoint implementation may initially track transaction-local mutation/WAL offsets.

---

# 41. Catalog

Catalog stores metadata for:

- buckets
- collections
- indexes
- schemas
- field dictionaries
- vector indexes
- full-text indexes

Proposed object:

```go
type CatalogObject struct {
    ID   ObjectID
    Type ObjectType
    Name string

    Root PageID

    Flags uint64
}
```

Catalog updates MUST be transactional.

---

# 42. Ordered Key/Value API

Example:

```go
bucket := db.Bucket("sessions")
```

Required operations:

```text
Put
Get
Delete
Exists

PutIfAbsent
CompareAndSwap

Increment
Decrement

BatchGet
BatchPut
BatchDelete

PrefixScan
RangeScan
ReverseScan

First
Last

Iterator
ReverseIterator

TTL
Expire
Persist
```

Example:

```go
err := bucket.Put(
    []byte("user:1001"),
    value,
)
```

Range iterator:

```go
it := bucket.Range(start, end)
defer it.Close()

for it.Next() {
    key := it.Key()
    value := it.Value()

    _ = key
    _ = value
}
```

---

# 43. Atomic KV Operations

Implement using transaction primitives.

### PutIfAbsent

```go
inserted, err := bucket.PutIfAbsent(key, value)
```

### CompareAndSwap

```go
swapped, err := bucket.CompareAndSwap(
    key,
    expected,
    replacement,
)
```

### Increment

```go
value, err := bucket.Increment(key, 1)
```

Stored integer encoding MUST be deterministic and validated.

---

# 44. TTL

Support TTL for KV and documents.

KV:

```go
bucket.Put(
    key,
    value,
    coco.TTL(30*time.Minute),
)
```

Implement TTL using an ordered expiration index.

Key:

```text
expiration timestamp
+
object ID
+
record ID/key
```

Background cleanup:

```text
seek earliest expiration
delete while expiration <= now
stop
```

Never periodically scan all data.

---

# 45. Document Model

Example:

```go
users := db.Collection("users")
```

Insert:

```go
id, err := users.Insert(coco.Document{
    "_id":    "u1001",
    "name":   "John",
    "age":    30,
    "active": true,
})
```

Required CRUD:

```text
Insert
InsertMany

Get
GetView

Replace

Update

Delete
DeleteMany

Find
FindOne

Count
Exists

Iterator
```

---

# 46. Document Types

Initial persistent types:

```text
Null
Bool

Int64
Uint64

Float32
Float64

String
Bytes

DateTime
Duration

UUID

Array
Object

Decimal128

VectorFloat32
VectorInt8
```

Avoid exposing dozens of integer widths in storage initially unless necessary.

Go values may be normalized into canonical persistent types.

---

# 47. CSON

CoCo should use its own binary document representation:

```text
CSON = CoCo Serialized Object Notation
```

Goals:

- compact
- deterministic
- versioned
- partial decoding
- low allocation
- direct field lookup
- nested object support
- array support

Do NOT persist documents as JSON strings.

---

# 48. CSON Layout

Suggested logical format:

```text
+------------------------+
| Header                 |
+------------------------+
| Field Directory        |
|                        |
| field id               |
| type                   |
| offset                 |
| length                 |
| ...                    |
+------------------------+
| Payload                |
+------------------------+
```

This allows access to a specific field without decoding the entire document.

---

# 49. Field Dictionary

Each collection maintains:

```text
1 -> _id
2 -> name
3 -> email
4 -> age
5 -> created_at
```

CSON stores FieldID rather than field names repeatedly.

Field IDs MUST NOT be reused after being retired.

This simplifies compatibility and old-record decoding.

---

# 50. DocumentView

A zero/low-allocation view API is a critical performance path.

Example:

```go
view, err := users.GetView("u1001")
if err != nil {
    return err
}

name, ok := view.String("name")
age, ok := view.Int64("age")
```

The query executor SHOULD operate on `DocumentView` where possible.

Avoid:

```text
[]byte
-> decode all
-> map[string]any
-> filter
```

for every query row.

---

# 51. Struct Mapping

Convenience API:

```go
type User struct {
    ID    string `coco:"_id"`
    Name  string `coco:"name"`
    Age   int64  `coco:"age"`
}
```

Reflection-based struct encoding is allowed as a convenience layer.

However:

```text
Document/CSON path = performance path
Reflection path = convenience path
```

Do not let reflection become a core storage dependency.

---

# 52. Document Updates

Support full replace:

```go
users.Replace("u1001", doc)
```

And partial update:

```go
users.Update(
    "u1001",
    coco.Set("age", 31),
    coco.Set("active", true),
)
```

Initial update operators:

```text
Set
Unset

Increment
Decrement

Min
Max

Push
Pop

AddToSet
Remove

Rename
SetIfMissing
```

Update execution must maintain indexes transactionally.

---

# 53. Optional Schema

Default collections are schemaless.

Optional validation:

```go
users.SetSchema(
    coco.Schema{
        "email": coco.String().Required(),
        "age":   coco.Int().Min(0),
    },
)
```

Validation happens before commit.

Potential constraints:

```text
Required
Type
Min
Max
Length
Enum
Pattern
Nested
Array element type
```

Regex validation MAY use Go stdlib `regexp`.

---

# 54. Secondary Indexes

Example:

```go
users.CreateIndex(
    coco.Index("email").
        Name("idx_email").
        Unique(),
)
```

Required:

```text
Single-field
Compound
Unique
Sparse
Partial
Nested-field
Array/multikey later
```

---

# 55. Compound Index

Example:

```go
users.CreateIndex(
    coco.Index(
        "country",
        "age",
        "created_at",
    ),
)
```

Physical key:

```text
encoded(country)
+
encoded(age)
+
encoded(created_at)
+
RecordID
```

For unique indexes, RecordID is not part of uniqueness semantics.

---

# 56. Partial Index

Example:

```go
users.CreateIndex(
    coco.Index("email").
        Where(
            coco.Eq("active", true),
        ),
)
```

Only matching documents enter the index.

---

# 57. Sparse Index

Example:

```go
users.CreateIndex(
    coco.Index("email").Sparse(),
)
```

Documents missing `email` are omitted.

---

# 58. Covering Index

Later v1.x capability:

```go
coco.Index("country", "age").
    Include("name", "email")
```

This permits index-only query execution.

Do not implement before normal secondary index correctness is complete.

---

# 59. Index States

Index metadata:

```text
BUILDING
READY
FAILED
DROPPING
```

Planner MUST only use indexes in READY state.

Initial v1 index creation may be blocking/offline.

Online background index build can be a later milestone.

---

# 60. Query API

Preferred Go-native fluent API:

```go
result, err := users.
    Query().
    Where("active").Eq(true).
    Where("age").Gte(18).
    OrderBy("created_at", coco.Desc).
    Limit(100).
    All()
```

Alternative logical expression:

```go
q := coco.And(
    coco.Eq("active", true),
    coco.Gte("age", 18),
)
```

---

# 61. Query Operators

Initial comparison operators:

```text
Eq
Ne

Gt
Gte

Lt
Lte

Between

In
NotIn
```

Logical:

```text
And
Or
Not
```

Existence:

```text
Exists
Missing
IsNull
IsNotNull
```

String:

```text
Contains
StartsWith
EndsWith
EqualFold
```

Array support:

```text
ArrayContains
ArrayAny
ArrayAll
ArraySize
```

Array operators may be phased after basic scalar query support.

---

# 62. Projection

Example:

```go
users.
    Query().
    Select(
        "name",
        "email",
    )
```

Exclude:

```go
Exclude("password")
```

Projection execution SHOULD read only requested CSON fields when possible.

---

# 63. Sorting

Example:

```go
OrderBy("created_at", coco.Desc)
```

Multi-field:

```go
OrderBy(
    coco.Desc("score"),
    coco.Asc("name"),
)
```

If index order satisfies requested ordering:

```text
do not execute an explicit sort
```

---

# 64. Query Architecture

Pipeline:

```text
Expression
    |
    v
Logical Plan
    |
    v
Planner
    |
    v
Optimizer
    |
    v
Physical Plan
    |
    v
Executor
```

---

# 65. Logical Operators

Potential nodes:

```text
Scan
Filter
Project
Sort
Limit
Aggregate
Distinct
VectorSearch
TextSearch
Lookup
```

---

# 66. Physical Operators

Initial operator interface:

```go
type Operator interface {
    Next() bool
    Row() Row
    Err() error
    Close() error
}
```

Implement:

```text
CollectionScan
PrimaryLookup
IndexSeek
IndexRangeScan
Fetch
Filter
Projection
Sort
Limit
Distinct
Aggregate
VectorScan
HNSWScan
TextScan
```

Use an iterator/Volcano-style execution model in v1.

Do not implement a bytecode VM initially.

---

# 67. Query Planner

Planner inputs:

- predicate
- available indexes
- requested sort
- projection
- limit
- vector clause
- text clause
- statistics

Example:

```text
WHERE country = "TH"
AND age >= 18
ORDER BY created_at DESC
LIMIT 20
```

Potential plan:

```text
IndexRangeScan(idx_country_age_created)
    |
    v
Fetch
    |
    v
Filter residual predicates
    |
    v
Limit
```

---

# 68. Statistics

Initial statistics:

```text
collection row count
index row count
distinct approximation
min
max
null count
```

Later:

```text
histograms
sampling
```

Do not implement a complex PostgreSQL-style estimator initially.

---

# 69. Explain

Required public feature:

```go
plan, err := users.
    Query().
    Where("email").Eq("john@example.com").
    Explain()
```

Example output:

```text
IndexSeek
  index: idx_email
  lookup: john@example.com
  expected rows: 1
  fetch: true
```

Later:

```text
ExplainAnalyze
```

with runtime metrics.

---

# 70. Aggregation

Initial:

```text
Count
Sum
Avg
Min
Max
Distinct
GroupBy
Having
```

Example:

```go
orders.
    Query().
    Where("status").Eq("paid").
    GroupBy("customer_id").
    Sum("amount")
```

Aggregation should stream where possible.

If memory exceeds query memory limit:

```text
spill to temporary storage
```

Spill implementation may be delayed until after in-memory aggregation works.

---

# 71. Query Memory Limits

Public configuration:

```go
coco.QueryMemoryLimit(32 * coco.MB)
```

Used by:

- sort
- group
- distinct
- hybrid search candidate sets

The database must avoid uncontrolled application memory use.

---

# 72. Temporary Storage

Potential file:

```text
app.coco-tmp
```

or OS-managed temporary file.

Use for:

```text
external sort
aggregation spill
large intermediate results
```

Temporary files MUST be cleaned up automatically.

---

# 73. Vector Data Model

Vectors live inside documents.

Example logical document:

```json
{
  "_id": "product-1",
  "name": "Laptop",
  "category": "electronics",
  "embedding": "<vector>"
}
```

API:

```go
products.CreateVectorIndex(
    coco.VectorIndex("embedding").
        Dimensions(768).
        Metric(coco.Cosine).
        HNSW(),
)
```

---

# 74. Vector Distance Functions

Implement in pure Go:

```text
Cosine
L2 / Euclidean
Squared L2
Dot Product / Inner Product
```

Performance baseline:

```go
for i := range a {
    dot += a[i] * b[i]
}
```

Do not write assembly/SIMD until benchmarks identify distance functions as a real bottleneck.

---

# 75. Exact Vector Search

Exact search MUST exist before HNSW.

Pipeline:

```text
candidate documents
      |
      v
read vector
      |
      v
distance(query, vector)
      |
      v
bounded top-k heap
```

A max-heap of K elements avoids sorting all vectors.

Exact search enables:

- correctness oracle for ANN tests
- small datasets
- highly selective metadata filtering
- fallback if vector index is unavailable

---

# 76. Persistent HNSW

Implement HNSW after exact search.

Node concept:

```go
type HNSWNode struct {
    RecordID RecordID
    Level    uint8

    // persisted neighbor lists
}
```

Index configuration:

```text
Dimensions
Metric
M
EfConstruction
```

Search configuration:

```text
EfSearch
TopK
```

Graph persistence MUST survive close/open.

HNSW changes MUST be recoverable or rebuildable.

---

# 77. Vector Index Consistency Strategy

Primary document record is the source of truth.

The vector index is a derived structure.

Recommended policy:

```text
document = authoritative
HNSW = rebuildable
```

After crash, if HNSW cannot be proven consistent:

```text
mark index NEEDS_REBUILD
```

and use:

```text
exact vector fallback
```

until rebuilt.

This is safer than risking incorrect ANN results.

---

# 78. Vector Quantization

Later milestone.

Target options:

```text
Float32
Float16
Int8 scalar quantization
```

Primary reason:

Embedded memory/storage efficiency.

Example:

```text
768 dimensions * float32
= 3072 bytes/vector

768 dimensions * int8
= 768 bytes/vector
```

Quantization must not be implemented before persistent exact/HNSW correctness.

---

# 79. Hybrid Vector + Metadata Query

Critical feature.

Example:

```go
products.
    Query().
    Where("category").Eq("laptop").
    Where("price").Lt(50000).
    VectorNear("embedding", queryVector).
    TopK(10)
```

Planner strategies:

### Metadata-first

```text
B+Tree metadata filter
      |
      v
candidate RecordIDs
      |
      v
exact/vector evaluation
```

Best when metadata is very selective.

### Vector-first

```text
HNSW
  |
  v
candidate top-N
  |
  v
metadata filter
  |
  v
top-K
```

Best when vector search is selective.

Planner may initially use simple heuristics.

---

# 80. Full-Text Search

Core components:

```text
Tokenizer
Normalizer
Inverted Index
Posting Lists
BM25
```

Example:

```go
articles.CreateTextIndex(
    "title",
    "body",
)
```

Query:

```go
articles.
    Query().
    Text("embedded database").
    TopK(20)
```

---

# 81. Text Index

Logical:

```text
term
 |
 +--> doc 10
 +--> doc 25
 +--> doc 40
```

Posting entries may include:

```text
RecordID
term frequency
field
position later
```

Start without positional search if necessary.

Phrase search can be later.

---

# 82. BM25

Implement BM25 in-house.

Required statistics:

```text
document count
document length
average document length
document frequency
term frequency
```

Do not start with language-specific stemming.

Start with:

```text
Unicode-aware tokenization
lowercase normalization
configurable stop words later
```

---

# 83. Hybrid Text + Vector

Later milestone.

Potential query:

```go
docs.
    Query().
    Text("golang embedded database").
    VectorNear("embedding", queryVector).
    Where("language").Eq("en").
    TopK(20)
```

Do not hard-code one scoring formula into storage format.

Expose a ranking/planner layer.

---

# 84. Full-Text and Vector Derived Index Principle

Source of truth:

```text
Document Record
```

Derived:

```text
Secondary Index
HNSW
Inverted Index
Statistics
```

Derived structures SHOULD be rebuildable from primary records.

This dramatically improves repairability.

---

# 85. Background Scheduler

Do not start many independent maintenance goroutines.

Implement one scheduler.

Jobs:

```text
Checkpoint
TTL cleanup
Vacuum
Statistics
Index maintenance
Optional vector maintenance
```

Configuration:

```go
coco.BackgroundWorkers(2)
```

For constrained environments:

```go
coco.Background(false)
```

Then expose:

```go
db.Maintenance(ctx)
```

---

# 86. Vacuum / MVCC Garbage Collection

Vacuum responsibilities:

- reclaim obsolete record versions
- reclaim deleted records
- reclaim overflow pages
- compact fragmented record pages when useful
- return unused pages to free list

Never reclaim data visible to the oldest active snapshot.

Initial mode:

```text
incremental vacuum
```

Avoid full-database rewrite.

---

# 87. Checksum

Use stdlib:

```go
hash/crc32
```

Prefer CRC32C table.

Checksum:

- meta page
- normal pages
- WAL records

Integrity checker validates these.

---

# 88. Integrity Checking

API:

```go
report, err := db.Check(ctx)
```

Validate:

```text
Meta pages
Page checksums
Page types
B+Tree ordering
B+Tree child links
Leaf links
Record directory
Free list
Secondary index references
HNSW references
Text index references
```

Derived index corruption SHOULD allow:

```text
RebuildIndex
```

Primary record corruption must produce a clear error.

---

# 89. Repair Philosophy

Do not silently invent or reconstruct primary user data.

Safe repair operations:

```text
Recover WAL
Rebuild secondary index
Rebuild HNSW
Rebuild text index
Rebuild statistics
Rebuild free list when possible
```

Primary record checksum failure:

```text
report corruption
```

unless a valid WAL/snapshot copy exists.

---

# 90. Backup

Public API:

```go
err := db.Backup(ctx, "backup.coco")
```

Backup MUST represent one consistent snapshot.

Initial implementation may:

1. acquire a snapshot
2. checkpoint
3. copy stable database pages/file
4. include required WAL if necessary
5. finalize backup

A blocking writer pause is acceptable for an early milestone if brief and clearly documented.

Later optimize to online backup.

---

# 91. Snapshot API

```go
snap, err := db.Snapshot()
if err != nil {
    return err
}
defer snap.Close()
```

Snapshot provides consistent read-only view.

Potential use cases:

- backup
- export
- long-running analytics
- application-level consistent read

---

# 92. Export / Import

Support later:

```text
NDJSON
binary dump
```

NDJSON is preferred over one huge JSON array.

KV dump can use binary framing.

---

# 93. Encryption

CoCo MUST support opt-in encryption at rest for an embedded database. Encryption is implemented entirely in Pure Go and does not require a daemon, CGO, a KMS, or an external service. The application supplies key material and remains responsible for protecting, rotating, and recovering it.

Use Go standard-library primitives:

```text
crypto/aes
crypto/cipher
AES-256-GCM
```

The initial public API accepts a 32-byte data-encryption key and an optional stable key identifier:

```go
db, err := coco.Open("app.coco",
    coco.EncryptionKey(key),       // exactly 32 bytes; never persisted
    coco.EncryptionKeyID("key-2026-01"),
)
```

Rules:

- Encryption is disabled only when the caller does not supply `EncryptionKey`; an existing encrypted database MUST fail to open without the correct key.
- Encrypt all persisted confidential database content: main database pages, WAL records, overflow pages, temporary/spill files, and backup output.
- Each encrypted page/WAL record MUST use a unique 96-bit AES-GCM nonce. Its envelope MUST include an encryption format version, key ID, nonce, ciphertext, and authentication tag. Bind the database ID, page or WAL-record identity, generation/LSN, and format version as AEAD associated data to prevent ciphertext swapping or replay.
- Validate the GCM authentication tag before parsing page/WAL contents. Authentication failure, missing key, wrong key, or unsupported encryption format MUST fail closed with a typed error; recovery MUST never treat unauthenticated bytes as valid data.
- Never store plaintext keys, passwords, derived keys, or raw key material in the database, WAL, backups, logs, metrics, traces, errors, or `String` output. Minimize key lifetime in memory and zero temporary mutable key buffers when practical.
- Do not implement weak password-to-key derivation. If password-derived keys are later required, specify an interoperable memory-hard KDF and its versioned parameters separately before implementation.
- Backup/restore preserves encryption metadata but never copies keys. Restoring an encrypted backup requires the matching key or a supplied destination re-encryption key.
- Support key rotation through an explicit online/offline rewrite operation: write new pages and WAL records under the new key ID, retain old keys only until all reachable encrypted data is rewritten, then verify and retire the old key. Rotation MUST be crash-recoverable and resumable.

Database encryption protects stored bytes, not a running process's memory, an application that has the key, or filesystem metadata such as database filename and file size. TLS/mTLS remains mandatory for optional networked Edge/Coordinator transport and protects data in transit.

Suggested configuration for deployments that construct options from configuration:

```yaml
storage:
  encryption:
    enabled: true
    key_id: key-2026-01
    key_source: environment-or-secret-provider # key bytes are never written to this file
```

The configuration loader MUST reject `enabled: true` without exactly 32 bytes of resolved key material, and it MUST avoid accepting keys as command-line arguments.

---

# 94. In-Memory Mode

Use `MemoryBackend`.

Do not create an entirely separate in-memory engine.

Same:

```text
Pager
B+Tree
Record
Txn
Query
```

must be exercised in memory mode.

This ensures test coverage of the real engine.

---

# 95. Read-Only / Immutable Modes

Read-only:

```go
coco.ReadOnly()
```

Immutable:

```go
coco.Immutable()
```

Immutable mode can later skip:

- writer locks
- WAL recovery checks after initial validation
- maintenance
- change tracking

Useful for bundled catalogs and offline search indexes.

---

# 96. Locking

v1 process-level policy:

```text
one process owns a RW database
```

Use a `.lock` file or OS advisory locking.

Cross-process multiple read-only support can be added later.

Do not attempt complex multi-process writer coordination in v1.

---

# 97. Lock Ordering

Define and document lock hierarchy.

Example:

```text
1. Database lifecycle
2. Catalog
3. Transaction manager
4. B+Tree
5. Page cache shard
6. Page
```

Never acquire locks in reverse order.

Avoid callbacks while holding internal locks.

---

# 98. Context Cancellation

Long operations MUST accept `context.Context`.

Examples:

```text
query
backup
check
vacuum
index build
vector rebuild
text rebuild
```

Executor should check cancellation periodically, not on every scalar comparison.

---

# 99. Limits

Database options should protect the host application.

Examples:

```text
MaxKeySize
MaxDocumentSize
MaxFieldDepth
MaxArrayDepth
MaxVectorDimensions
MaxQueryMemory
MaxReaders
```

Initial suggested defaults:

```text
MaxKeySize       64 KiB
MaxDocumentSize  16 MiB
MaxDepth         128
```

Tune later.

---

# 100. Observability

Expose:

```go
stats := db.Stats()
```

Useful metrics:

```text
DatabaseSize
WALSize

PageCount
FreePageCount

CacheHits
CacheMisses
CacheHitRate

DirtyPages

ActiveReaders
WriteQueueDepth

Commits
Rollbacks

CheckpointCount

IndexScans
CollectionScans

VectorQueries
TextQueries

VacuumPagesReclaimed
```

Do not embed Prometheus.

Applications can export stats themselves.

---

# 101. Query Profiling

Later:

```go
profile, err := query.Profile(ctx)
```

Example:

```text
Planning:       0.10 ms
IndexSeek:      0.50 ms
Fetch:          1.70 ms
Filter:         0.20 ms
Total:          2.50 ms

Rows scanned:   100
Rows returned:  10
Cache hit rate: 98.4%
```

---

# 102. Slow Query Callback

Potential API:

```go
db.SetSlowQueryThreshold(
    100 * time.Millisecond,
)
```

```go
db.OnSlowQuery(func(q coco.QueryProfile) {
})
```

Callbacks MUST execute outside internal storage locks.

---

# 103. Package Structure

Recommended repository:

```text
coco/
|
|-- coco.go
|-- db.go
|-- tx.go
|-- options.go
|-- errors.go
|-- stats.go
|
|-- kv/
|   |-- bucket.go
|   |-- iterator.go
|   `-- batch.go
|
|-- document/
|   |-- collection.go
|   |-- document.go
|   |-- update.go
|   |-- query.go
|   |-- schema.go
|   `-- iterator.go
|
|-- vector/
|   |-- vector.go
|   |-- index.go
|   `-- query.go
|
|-- search/
|   |-- text.go
|   `-- hybrid.go
|
`-- internal/
    |
    |-- file/
    |   |-- backend.go
    |   |-- osfile.go
    |   |-- memfile.go
    |   `-- lock.go
    |
    |-- storage/
    |   |-- page.go
    |   |-- pager.go
    |   |-- slotted.go
    |   |-- overflow.go
    |   |-- allocator.go
    |   |-- freelist.go
    |   `-- checksum.go
    |
    |-- cache/
    |   |-- cache.go
    |   |-- shard.go
    |   `-- entry.go
    |
    |-- btree/
    |   |-- tree.go
    |   |-- node.go
    |   |-- internal.go
    |   |-- leaf.go
    |   |-- cursor.go
    |   |-- search.go
    |   |-- insert.go
    |   |-- delete.go
    |   |-- split.go
    |   |-- merge.go
    |   `-- verify.go
    |
    |-- wal/
    |   |-- wal.go
    |   |-- header.go
    |   |-- record.go
    |   |-- writer.go
    |   |-- reader.go
    |   |-- recovery.go
    |   `-- checkpoint.go
    |
    |-- txn/
    |   |-- manager.go
    |   |-- transaction.go
    |   |-- snapshot.go
    |   |-- mvcc.go
    |   |-- reader_table.go
    |   `-- savepoint.go
    |
    |-- record/
    |   |-- record.go
    |   |-- directory.go
    |   |-- version.go
    |   `-- blob.go
    |
    |-- cson/
    |   |-- types.go
    |   |-- header.go
    |   |-- encode.go
    |   |-- decode.go
    |   |-- view.go
    |   `-- dictionary.go
    |
    |-- catalog/
    |   |-- catalog.go
    |   |-- object.go
    |   |-- collection.go
    |   |-- index.go
    |   `-- schema.go
    |
    |-- index/
    |   |-- primary.go
    |   |-- secondary.go
    |   |-- compound.go
    |   |-- sparse.go
    |   |-- partial.go
    |   `-- ttl.go
    |
    |-- query/
    |   |-- expression.go
    |   |-- logical.go
    |   |-- physical.go
    |   |-- planner.go
    |   |-- optimizer.go
    |   |-- stats.go
    |   `-- executor/
    |       |-- operator.go
    |       |-- collection_scan.go
    |       |-- index_scan.go
    |       |-- fetch.go
    |       |-- filter.go
    |       |-- projection.go
    |       |-- sort.go
    |       |-- limit.go
    |       |-- aggregate.go
    |       `-- lookup.go
    |
    |-- vector/
    |   |-- distance.go
    |   |-- exact.go
    |   |-- quantize.go
    |   `-- hnsw/
    |       |-- graph.go
    |       |-- node.go
    |       |-- insert.go
    |       |-- delete.go
    |       |-- search.go
    |       `-- persist.go
    |
    |-- text/
    |   |-- tokenizer.go
    |   |-- normalize.go
    |   |-- postings.go
    |   |-- inverted.go
    |   `-- bm25.go
    |
    |-- backup/
    |   |-- backup.go
    |   |-- restore.go
    |   `-- snapshot.go
    |
    |-- maintenance/
    |   |-- scheduler.go
    |   |-- vacuum.go
    |   |-- ttl.go
    |   |-- checkpoint.go
    |   `-- analyze.go
    |
    `-- integrity/
        |-- check.go
        `-- repair.go
```

---

# 104. Dependency Policy

Core package target:

```text
stdlib-only
```

Expected stdlib packages:

```text
os
io
bytes
bufio
encoding/binary
hash/crc32

sync
sync/atomic

context
time

math
sort
container/heap

crypto/aes
crypto/cipher

unicode
unicode/utf8
regexp
```

No dependency should be added merely for convenience.

Dependency approval rule:

> A dependency is allowed only if implementing and maintaining the feature internally provides little architectural value and the dependency is small, stable, pure Go, and isolated behind an optional module.

---

# 105. Public API Design Principles

The API should be:

- Go-native
- explicit
- context-aware where needed
- easy for basic use
- capable of advanced control
- not tightly coupled to internal representation

Avoid exposing:

```text
PageID
SlotID
BTree node
WAL record
MVCC internals
```

Public types should not lock future file-format evolution.

---

# 106. Error Model

Define sentinel errors where useful:

```go
var (
    ErrNotFound       = errors.New("coco: not found")
    ErrExists         = errors.New("coco: already exists")
    ErrReadOnly       = errors.New("coco: database is read-only")
    ErrTxnClosed      = errors.New("coco: transaction closed")
    ErrConflict       = errors.New("coco: transaction conflict")
    ErrCorrupt        = errors.New("coco: database corruption")
    ErrInvalidFormat  = errors.New("coco: invalid database format")
    ErrUnsupported    = errors.New("coco: unsupported operation")
)
```

Wrap errors using `%w`.

Persistent corruption errors should include:

- page
- record
- index
- LSN

when known.

---

# 107. Implementation Strategy

Implementation MUST be vertical and correctness-first.

Do not build Document, HNSW, or Query Engine before the storage substrate has crash/recovery tests.

Development order:

```text
File Format
   |
   v
Pager
   |
   v
Slotted Pages
   |
   v
B+Tree
   |
   v
WAL
   |
   v
Transactions
   |
   v
MVCC
   |
   v
KV
   |
   v
CSON
   |
   v
Document
   |
   v
Indexes
   |
   v
Query
   |
   v
Vector Exact
   |
   v
HNSW
   |
   v
Full-Text
   |
   v
Hybrid Search
   |
   v
Operational Features
```

---

# 108. Milestone 0 — Repository & Engineering Foundation

## Deliverables

- Go module
- internal package structure
- coding conventions
- benchmark package
- fuzz testing setup
- CI
- race detector job
- file-format version constants
- standard errors

## Required commands

```bash
go test ./...
go test -race ./...
go vet ./...
```

## Acceptance

- empty repository builds
- package architecture finalized
- no external dependencies

---

# 109. Milestone 1 — File Backend and Meta Format

## Implement

```text
OSBackend
MemoryBackend

Meta A
Meta B

CRC32C
database creation
database open
database close
```

## Tests

- create database
- reopen database
- invalid magic
- corrupt meta A
- corrupt meta B
- choose latest generation
- both meta pages invalid
- memory backend behavior

## Acceptance

Database can:

```text
Create
Close
Reopen
Validate Meta
```

without B+Tree.

---

# 110. Milestone 2 — Pager and Page Allocator

## Implement

```text
Page encode/decode
Read page
Write page
Allocate page
Free page
Free list
Dirty tracking
Checksum
```

## Tests

- allocate 1 page
- allocate 10,000 pages
- free/reuse pages
- corrupt page checksum
- truncate file
- reopen and validate allocation metadata

## Acceptance

Pager is deterministic across reopen.

---

# 111. Milestone 3 — Slotted Pages

## Implement

```text
Insert
Read
Update
Delete
Compact
FreeSpace
```

## Mandatory fuzz tests

Random sequence:

```text
Insert
Update
Delete
Compact
Read
```

Compare against an in-memory reference model.

## Acceptance

At least millions of randomized operations run without invariant failure.

---

# 112. Milestone 4 — B+Tree Read Path

## Implement

```text
Leaf node
Internal node
Search
Cursor
First
Last
Seek
Next
Prev
```

Use a temporary simple builder if necessary before insert logic is finished.

## Acceptance

Read-only manually-created trees behave correctly.

---

# 113. Milestone 5 — B+Tree Insert/Split

## Implement

```text
Insert
Leaf split
Internal split
Root split
```

## Tests

Insert:

```text
ascending keys
descending keys
random keys
duplicate keys
large variable keys
```

Compare against:

```text
map + sorted keys reference
```

## Acceptance

At least 1,000,000 randomized inserts pass.

---

# 114. Milestone 6 — B+Tree Delete/Merge

## Implement

```text
Delete
Borrow sibling
Merge sibling
Root collapse
```

## Tests

Random insert/delete workload against reference model.

Validate invariants after every N operations.

## Acceptance

No ordering, child, or leaf-chain invariant violations.

---

# 115. Milestone 7 — WAL Basic

## Implement

```text
WAL append
WAL CRC
Txn begin
Page update
Commit
Abort
```

Use full-page redo records initially if needed.

## Tests

- append/read
- torn final WAL record
- bad CRC
- committed transaction
- uncommitted transaction

---

# 116. Milestone 8 — Recovery

## Implement

```text
dirty shutdown detection
WAL scan
committed transaction identification
redo
checkpoint
```

## Crash testing

Create a test binary that:

1. writes transactions
2. exits abruptly at injected failpoints
3. reopens database
4. validates model

Failpoints:

```text
after WAL append
before fsync
after fsync
during page flush
before meta update
after meta update
during checkpoint
```

## Acceptance

Crash recovery is correct across all injected failure points.

This milestone is a hard gate.

Do not continue to document/vector work if crash correctness is unstable.

---

# 117. Milestone 9 — Transactions

## Implement

```text
View
Update
Begin
Commit
Rollback

transaction-local changes
writer coordinator
```

Initially:

```text
one writer
multiple readers
```

## Tests

- rollback leaves no visible changes
- atomic multi-key commit
- error inside Update rolls back
- closed Tx rejects new operations

---

# 118. Milestone 10 — MVCC and Snapshot Isolation

## Implement

```text
Txn IDs
read snapshot
version visibility
reader table
oldest snapshot
version chain
```

## Tests

Scenario:

```text
Reader R1 opens
Writer W updates key
Reader R2 opens

R1 sees old
R2 sees new
```

Deletion scenario:

```text
R1 sees value
W deletes
R2 sees missing
R1 still sees old value
```

## Acceptance

Snapshot semantics are deterministic.

---

# 119. Milestone 11 — Embedded KV MVP

At this point expose the first public usable database.

## Implement

```text
Bucket
Get
Put
Delete
Exists
Iterator
Prefix
Range
Batch
```

## MVP Release

Potential version:

```text
v0.1.0
```

At this milestone CoCo is already useful as an embedded ordered KV database.

---

# 120. Milestone 12 — KV Atomic + TTL

## Implement

```text
PutIfAbsent
CAS
Increment
Decrement
TTL index
expiration cleanup
```

## Tests

- concurrent goroutines submitting CAS
- expiration ordering
- restart with expired keys
- snapshot visibility around deletion

---

# 121. Milestone 13 — CSON

## Implement

```text
types
encoder
decoder
field dictionary
nested object
array
DocumentView
```

## Benchmark against

```text
encoding/json
```

Benchmarks:

```text
encode
decode full
read one integer
read one nested string
projection of 3 fields
```

Primary performance goal:

`DocumentView` field access must avoid full document allocation.

---

# 122. Milestone 14 — Document CRUD

## Implement

```text
Collection
Insert
Get
GetView
Replace
Update
Delete
Find by primary ID
```

## Acceptance

Document CRUD is fully transactional.

Potential release:

```text
v0.2.0
```

---

# 123. Milestone 15 — Secondary Indexes

## Implement

```text
single field
unique
compound
sparse
partial
```

Index updates occur in the same transaction as record changes.

## Tests

- insert updates index
- update moves index key
- delete removes index key
- rollback leaves index unchanged
- unique conflict
- crash recovery

---

# 124. Milestone 16 — Query Expression + Executor

## Implement

```text
Expression tree
CollectionScan
Filter
Projection
Limit
Sort
```

No optimizer required yet.

Goal:

correct query semantics.

---

# 125. Milestone 17 — Planner + Index Scans

## Implement

```text
IndexSeek
IndexRangeScan
basic selectivity
sort satisfaction
projection planning
Explain
```

Planner initial rules:

```text
exact unique index > exact index > range index > collection scan
```

Then incorporate:

```text
sort compatibility
limit
statistics
```

Potential release:

```text
v0.3.0
```

---

# 126. Milestone 18 — Aggregation

## Implement

```text
Count
Sum
Avg
Min
Max
Distinct
GroupBy
Having
```

Start memory-only.

Add spill after correctness and profiling.

---

# 127. Milestone 19 — Exact Vector Search

## Implement

```text
VectorFloat32
distance functions
TopK heap
metadata + exact vector
```

Use exact vector search as correctness baseline for ANN.

Potential release:

```text
v0.4.0
```

---

# 128. Milestone 20 — Persistent HNSW

## Implement

```text
graph
node persistence
insert
search
delete semantics
rebuild
```

Tests MUST compare ANN results against exact search.

Metrics:

```text
recall@K
latency
memory
index size
build time
```

Do not claim performance without these benchmarks.

---

# 129. Milestone 21 — Vector Hybrid Planner

## Implement

```text
metadata-first
vector-first
candidate oversampling
filter-after-ANN
fallback exact
```

Initial heuristic:

```text
if estimated metadata candidate count is small:
    metadata-first
else:
    vector-first
```

---

# 130. Milestone 22 — Full-Text Search

## Implement

```text
tokenizer
normalizer
inverted index
posting list
BM25
text query
```

Potential release:

```text
v0.5.0
```

---

# 131. Milestone 23 — Text + Vector Hybrid

Implement combination of:

```text
Metadata
Text
Vector
```

Ranking layer should remain extensible.

---

# 132. Milestone 24 — Maintenance

## Implement

```text
Incremental vacuum
Checkpoint scheduler
TTL scheduler
Analyze/statistics
Index rebuild
```

Allow:

```go
coco.Background(false)
```

---

# 133. Milestone 25 — Backup / Integrity

## Implement

```text
Snapshot
Backup
Restore
Check
Rebuild derived indexes
```

Potential release:

```text
v0.6.0
```

---

# 134. Milestone 26 — Encryption

Implement only after:

```text
WAL
backup
recovery
```

are stable, because encryption affects all of them.

Acceptance: encrypted main-file/WAL/backup round trips succeed with the correct key; missing, incorrect, altered, and swapped ciphertext fails closed; crash recovery and key rotation pass fault-injection tests; no plaintext key material appears in diagnostic output.

---

# 135. Milestone 27 — Performance Hardening

Profile:

```text
CPU
allocations
mutex contention
disk I/O
WAL fsync
cache hit ratio
query plans
vector search
```

Only now consider:

```text
custom cache policy
SIMD
mmap
direct I/O
compressed pages
delta WAL
specialized allocators
```

No optimization should be introduced without benchmark evidence.

---

# 136. Testing Strategy

Testing layers:

```text
Unit
Property
Fuzz
Integration
Crash
Concurrency
Race
Benchmark
Long-running soak
Corruption
Compatibility
```

---

# 137. Property Tests

Maintain simple reference models.

For B+Tree:

```text
map + sorted slice
```

For KV:

```text
map[string][]byte
```

For MVCC:

small logical version model.

Generate randomized operations and compare behavior.

---

# 138. Go Fuzz Targets

Mandatory fuzz targets:

```text
slotted page decode
CSON decode
key decoder
WAL decoder
B+Tree operation sequence
query expression parser/encoder if any
```

Never trust persistent binary decoders without fuzzing.

---

# 139. Crash Test Harness

The database project MUST have an automated crash harness.

Pattern:

```text
Parent process
  |
  +--> spawn child
          |
          +--> execute workload
          |
          +--> crash at failpoint
  |
  +--> reopen database
  |
  +--> verify reference state
```

Failpoint infrastructure should support:

```go
failpoint.Hit("wal.after-sync")
```

The final implementation can compile failpoints out of production builds.

---

# 140. Race Testing

CI must run:

```bash
go test -race ./...
```

Focus on:

```text
reader table
page cache
writer coordinator
scheduler
stats
change feed
```

---

# 141. Soak Tests

Run randomized workloads for hours.

Example mix:

```text
50% reads
20% inserts
15% updates
10% deletes
5% range scans
```

With periodic:

```text
checkpoint
vacuum
close/open
```

Compare final state to reference model.

---

# 142. Corruption Tests

Randomly corrupt:

```text
page byte
page header
slot
meta
WAL byte
free list
secondary index page
```

Expected behavior:

```text
detect corruption
never silently return invented data
```

---

# 143. File Format Compatibility Tests

Once a format version is released, preserve fixtures:

```text
testdata/v1/basic.coco
testdata/v1/index.coco
testdata/v1/vector.coco
```

New engine versions must open old supported formats.

Never change persistent layout silently.

---

# 144. Benchmark Suite

Create reproducible benchmarks for:

### KV

```text
PointGet
Put
Delete
PrefixScan
RangeScan
BatchPut
```

### Document

```text
Insert
Get
CSON field lookup
Indexed equality
Indexed range
Collection scan
Projection
Sort
Aggregation
```

### Transactions

```text
Commit Full Sync
Commit Normal Sync
Group Commit
Read Snapshot
```

### Vector

```text
Exact TopK
HNSW TopK
Filtered ANN
Recall@10
Recall@100
```

### Text

```text
Index build
Single term
multi-term
BM25 TopK
```

---

# 145. Benchmark Dataset Sizes

At least:

```text
1K
10K
100K
1M
10M where practical
```

Vector:

```text
10K
100K
1M
```

with dimensions:

```text
128
384
768
1536
```

---

# 146. Initial Performance Targets

These are engineering targets, NOT guarantees.

For a modern desktop/server system with data in cache:

```text
simple KV Get:
target low-microsecond to tens-of-microseconds range

in-memory simple Get:
hundreds of thousands ops/s or better

range iteration:
close to sequential memory/page traversal cost

CSON single-field read:
significantly cheaper than full JSON unmarshal
```

Durable write performance depends heavily on fsync latency.

Measure separately:

```text
SyncFull
SyncNormal
SyncOff
```

Never publish one write benchmark without sync mode.

---

# 147. Embedded Memory Targets

Idle engine should be configurable to operate within approximately:

```text
single-digit to low tens of MB
```

depending on enabled features.

Tiny mode should permit:

```text
~4-8 MB page cache
```

Vector indexes naturally require more memory/storage and should not force large allocations when unused.

---

# 148. Performance Rules

Never:

- optimize without benchmark
- benchmark with debug logging
- compare SyncOff against durable databases
- hide cache state
- hide page size
- hide dataset size
- hide vector recall

All benchmark results should record:

```text
CPU
RAM
disk
OS
Go version
sync mode
page size
cache size
dataset
```

---

# 149. Implementation Rules for AI Coding Agents

An AI agent working on CoCo MUST follow these rules.

## Rule 1

Do not replace internal components with third-party storage engines.

Forbidden:

```text
Badger
Pebble
Bolt
RocksDB
SQLite
LevelDB
```

## Rule 2

Do not bypass abstractions.

Examples:

```text
query layer must not read os.File
document layer must not manipulate pages
B+Tree must not know about Document
```

## Rule 3

Persistent formats must use explicit encoding.

Never persist raw Go structs.

## Rule 4

Every persistent structure requires:

```text
version
validation
bounds checks
```

## Rule 5

Storage correctness beats performance.

## Rule 6

Any optimization changing recovery semantics requires crash tests.

## Rule 7

New index types must be rebuildable.

## Rule 8

Avoid external dependencies unless approved.

## Rule 9

Every new package needs unit tests.

## Rule 10

Every bug involving corruption/recovery gets a permanent regression test.

---

# 150. Definition of Done — Storage Feature

A storage feature is not done unless:

- implementation complete
- tests complete
- corruption paths handled
- race-safe
- reopen tested
- recovery tested when persistent
- benchmark exists when performance-sensitive
- docs updated

---

# 151. Definition of Done — Public API Feature

A public API is not done unless:

- API is documented
- context behavior defined
- transaction semantics defined
- errors defined
- concurrency behavior defined
- example exists
- tests exist
- no internal type leaks

---

# 152. Recommended Initial Public API

```go
package coco
```

Open:

```go
db, err := coco.Open(
    "app.coco",
    coco.Profile(coco.Balanced),
    coco.SyncMode(coco.SyncNormal),
)
```

Close:

```go
defer db.Close()
```

---

# 153. KV Usage Example

```go
sessions := db.Bucket("sessions")

err := sessions.Put(
    []byte("token:abc"),
    []byte("user-100"),
    coco.TTL(time.Hour),
)
```

```go
value, err := sessions.Get(
    []byte("token:abc"),
)
```

---

# 154. Document Usage Example

```go
users := db.Collection("users")

_, err := users.Insert(coco.Document{
    "_id":    "u100",
    "name":   "Alice",
    "age":    int64(30),
    "active": true,
})
```

Query:

```go
rows, err := users.
    Query().
    Where("active").Eq(true).
    Where("age").Gte(18).
    OrderBy("age", coco.Desc).
    Limit(20).
    All()
```

---

# 155. Index Usage Example

```go
err := users.CreateIndex(
    coco.Index("email").
        Name("idx_users_email").
        Unique(),
)
```

Compound:

```go
err := users.CreateIndex(
    coco.Index(
        "country",
        "age",
    ).
    Name("idx_country_age"),
)
```

---

# 156. Vector Usage Example

```go
err := products.CreateVectorIndex(
    coco.VectorIndex("embedding").
        Dimensions(768).
        Metric(coco.Cosine).
        HNSW().
        M(16).
        EfConstruction(200),
)
```

Search:

```go
rows, err := products.
    Query().
    Where("category").Eq("laptop").
    VectorNear("embedding", queryVector).
    TopK(10).
    All()
```

---

# 157. Transaction Usage Example

```go
err := db.Update(func(tx *coco.Tx) error {
    users := tx.Collection("users")
    counters := tx.Bucket("counters")

    if err := users.Update(
        "u100",
        coco.Set("active", true),
    ); err != nil {
        return err
    }

    _, err := counters.Increment(
        []byte("active-users"),
        1,
    )

    return err
})
```

Document + KV commit atomically because both use the same transaction engine.

---

# 158. Release Strategy

Suggested phases:

## v0.1

```text
Storage
B+Tree
WAL
Recovery
Transactions
MVCC
KV
```

## v0.2

```text
CSON
Document CRUD
Secondary indexes
```

## v0.3

```text
Query planner
Index query
Aggregation
Explain
```

## v0.4

```text
Exact vector
Persistent HNSW
Filtered vector search
```

## v0.5

```text
Full-text
BM25
Hybrid search
```

## v0.6

```text
Vacuum
Backup
Integrity
Statistics
Operational hardening
```

## v0.9 Embedded

```text
File format freeze candidate
Performance hardening
Compatibility
```

## v1.0 Embedded

```text
Stable embedded API
Stable file format
Document + KV + Vector + Text
ACID
Recovery
Backup
Integrity
```

---

# 159. Critical Architecture Invariants

These MUST remain true throughout development.

### Invariant 1

```text
Document, KV, Vector, Text
share one transaction kernel.
```

### Invariant 2

```text
Derived indexes are rebuildable.
```

### Invariant 3

```text
RecordID is stable across physical relocation.
```

### Invariant 4

```text
No dirty page reaches main DB before its WAL is durable.
```

### Invariant 5

```text
Readers see a stable snapshot.
```

### Invariant 6

```text
No persistent binary structure relies on Go memory layout.
```

### Invariant 7

```text
Embedded use never requires a network/server process.
```

### Invariant 8

```text
Core storage has no mandatory third-party storage dependency.
```

---

# 160. What Must Be Implemented First

The first engineering iteration should focus ONLY on:

```text
1. File backend
2. Meta pages
3. Page codec
4. Pager
5. Slotted page
6. Page allocator
7. B+Tree
8. WAL
9. Recovery
10. Transactions
```

Do NOT start:

```text
HNSW
Full text
Complex query planner
Encryption
Compression
Replication
```

during this stage.

The project succeeds or fails on the correctness of the storage kernel.

---

# 161. First Development Sprint Suggested Tasks

## Task 1

Create repository/package structure.

## Task 2

Implement persistent primitive types.

## Task 3

Implement `Backend`.

## Task 4

Implement OS and memory backends.

## Task 5

Implement page codec.

## Task 6

Implement CRC32C.

## Task 7

Implement dual meta pages.

## Task 8

Implement create/open/close DB.

## Task 9

Implement pager read.

## Task 10

Implement page allocation.

## Task 11

Implement page free/reuse.

## Task 12

Implement slotted page.

## Task 13

Add fuzz tests.

## Task 14

Add microbenchmarks.

Sprint exit criteria:

```text
Database file can be created,
pages allocated,
records written to slotted pages,
closed,
reopened,
and validated.
```

---

# 162. Second Development Sprint

Focus:

```text
B+Tree
```

Tasks:

```text
node layout
leaf search
internal search
insert
split
cursor
range
delete
merge
verification
property tests
```

Sprint exit:

```text
1M randomized insert/delete operations
match reference model.
```

---

# 163. Third Development Sprint

Focus:

```text
WAL + crash recovery
```

Tasks:

```text
WAL format
append
CRC
commit
page redo
recovery
checkpoint
crash failpoints
```

Sprint exit:

```text
random crash testing repeatedly recovers
to a valid committed state.
```

---

# 164. Fourth Development Sprint

Focus:

```text
Transactions + MVCC + KV
```

Exit:

```text
CoCo can be released internally as an embedded KV database.
```

---

# 165. Fifth Development Sprint

Focus:

```text
CSON + Documents
```

Exit:

```text
Document CRUD with MVCC and crash safety.
```

---

# 166. Sixth Development Sprint

Focus:

```text
Secondary indexes + query execution
```

Exit:

```text
Indexed NoSQL query support.
```

---

# 167. Seventh Development Sprint

Focus:

```text
Exact vector + HNSW
```

Exit:

```text
Persistent filtered vector search.
```

---

# 168. Eighth Development Sprint

Focus:

```text
Full text + hybrid search
```

---

# 169. Final Engineering Principle

CoCo should not be built as:

```text
Document DB
+
separate KV engine
+
separate vector engine
```

It must be built as:

```text
             One Embedded Kernel
                     |
        +------------+------------+
        |            |            |
        v            v            v
       KV         Document      Vector
                                  |
                               Full Text
```

All models must share:

```text
Transactions
MVCC
WAL
RecordID
Pager
Page Cache
B+Tree
Backup
Recovery
Integrity
```

This is the core architectural advantage of CoCo.

---

# 170. Final Target Architecture

```text
+-----------------------------------------------------------+
|                         CoCo                              |
+-----------------------------------------------------------+
| Public API                                                |
| KV | Document | Vector | Full Text | Transaction          |
+-----------------------------------------------------------+
| Query                                                     |
| Expressions | Planner | Optimizer | Executor | Aggregate  |
+--------------------------+--------------------------------+
| Ordered Index            | Search Index                   |
| B+Tree                    | HNSW | Inverted Index          |
+--------------------------+--------------------------------+
| CSON / Records / Blob / Version Chains                    |
+-----------------------------------------------------------+
| MVCC / Snapshot / Transaction Manager                     |
+-----------------------------------------------------------+
| B+Tree / Record Directory                                 |
+-----------------------------------------------------------+
| Page Cache                                                |
+-----------------------------------------------------------+
| Pager / Slotted Page / Allocator / Free List / Checksum   |
+-----------------------------------------------------------+
| WAL / Recovery / Checkpoint                               |
+-----------------------------------------------------------+
| Backend                                                   |
| OS File | Memory                                          |
+-----------------------------------------------------------+
```

Final product goal:

> **CoCo should feel as simple to embed as SQLite, expose flexible NoSQL/document semantics, provide ordered KV performance, and add native vector/full-text search without requiring a separate service.**

The priority order is always:

```text
Correctness
>
Crash Safety
>
Predictability
>
API Quality
>
Performance
>
Advanced Features
```

Once the kernel is proven correct, optimization can be aggressive without compromising reliability.

---

# 171. Distributed Edge Architecture (Optional Deployment Module)

Distributed mode extends CoCo without changing the embedded storage contract. Each Edge owns one or more local `.coco` databases and retains the existing WAL, MVCC, recovery, and local ACID guarantees.

```text
                         Control Plane
              (membership, shard map, capabilities)
                              |
                              v
Go application ---> Coordinator / Gateway ---- gRPC ----+--> Edge A (CPU)
                                                     +--> Edge B (GPU)
                                                     +--> Edge C (GPU)
                                                               |
                                                               v
                                                        Embedded CoCo kernel
```

Components:

```text
coco-core            Pure-Go embedded kernel; no network or GPU dependency
coco-proto           Versioned protobuf API, buf/protoc configuration, compatibility checks
coco-edge            gRPC server, shard host, local admission control, GPU runtime selection
coco-coordinator     Routing, scatter/gather, merge, retry policy, query cancellation
coco-control-plane   Membership, shard map, health, capability and rollout metadata
```

The coordinator MAY run in an application process or be deployed as a service. An Edge MUST be independently restartable and recoverable from its own local files.

## 171.1 Data Ownership and Consistency

Use a stable `ShardID` derived from a collection's partition key. The control plane maps each shard to exactly one writable primary Edge in distributed v1.

```text
write -> resolve ShardID -> primary Edge -> local ACID transaction -> acknowledgement
read  -> resolve target shard(s) -> one or more Edges -> merge at coordinator
```

Rules:

- Every write MUST include an idempotency key and a client request ID.
- An Edge MUST deduplicate completed write requests for a bounded, configurable retention window.
- A request addressed to a stale shard epoch MUST fail with `STALE_SHARD_MAP`, including the current epoch when safe to return.
- A coordinator MUST retry only operations explicitly marked retry-safe and only before the deadline.
- A query that spans shards has **per-shard snapshot consistency**, not a global serializable snapshot, in distributed v1.
- Cross-shard transactions, joins requiring transactional consistency, and multi-primary writes are out of scope for distributed v1.
- Replicas, if added later, MUST be asynchronous read replicas until replication ordering and failover semantics are separately specified.

## 171.2 Distributed Query and Vector Merge

The coordinator compiles a logical query once, pushes filters and vector work to Edges, then merges Edge result streams.

```text
client query
   -> route relevant shards
   -> Edge-local filter + vector/text candidate search
   -> stream sorted candidates with score, shard, record version
   -> coordinator k-way merge, deduplicate, apply final top-K
```

For global `topK = K`, each Edge MUST receive an oversampled candidate limit: `K * oversampleFactor`, where the factor is planner-configurable and reported in `Explain`. The coordinator MUST expose `partial_results`, `failed_shards`, and `shard_map_epoch`; it MUST never silently return a partial result as complete.

Initial distributed query support is limited to point operations, shard-key routed scans, fan-out filtered vector search, and fan-out text/vector search. Global aggregation, globally ordered pagination, and distributed joins require explicit merge semantics and are deferred.

---

# 172. gRPC and Protobuf Contract

gRPC is the required transport between optional distributed Go modules. HTTP/JSON, if introduced, is an optional gateway and MUST NOT be the canonical contract. The embedded `coco` package remains the only public application API; no remote client package is provided.

## 172.1 Protocol Layout

```text
api/proto/coco/v1/common.proto
api/proto/coco/v1/kv.proto
api/proto/coco/v1/document.proto
api/proto/coco/v1/vector.proto
api/proto/coco/v1/query.proto
api/proto/coco/v1/cluster.proto
api/proto/coco/v1/admin.proto
```

Required services:

```text
KVService           unary CRUD, batches, transactions scoped to one shard
DocumentService     document CRUD and index administration
QueryService        server-streaming query/search results and Explain
VectorService       vector index administration and vector-search requests
ClusterService      Edge registration, health, shard-map and capability discovery
AdminService        protected maintenance and diagnostics operations
```

Use unary RPCs for bounded point operations and server streaming for scans, queries, backups, and large result sets. Every request MUST carry `request_id`, `deadline` through the normal gRPC context, and an API version. Mutating requests MUST additionally carry an `idempotency_key`.

## 172.2 Vector Wire Format

The canonical vector payload is packed bytes, not `repeated float`, to reduce allocation and protocol overhead:

```proto
message Vector {
  uint32 dimensions = 1;
  VectorEncoding encoding = 2; // F32_LE in distributed v1
  bytes data = 3;              // dimensions * 4 bytes for F32_LE
}
```

The server MUST validate dimensions, byte length, finite values, metric/index compatibility, and request limits before scheduling CPU or GPU work. Protocol fields use explicit encodings and endianness; they never rely on a language runtime's in-memory representation.

## 172.3 Compatibility, Errors, and Security

- Follow protobuf evolution rules: never reuse field numbers; reserve removed fields and enum values.
- CI MUST run breaking-change checks against the last released protobuf descriptor set.
- Map stable CoCo error codes to gRPC status codes and include structured details for `SHARD_MOVED`, `STALE_SHARD_MAP`, `PARTIAL_RESULT`, `INDEX_NOT_READY`, and `GPU_UNAVAILABLE`.
- Remote transport MUST support TLS (often called “SSL” in deployment configuration). SSLv2, SSLv3, TLS 1.0, and TLS 1.1 MUST be refused; production defaults require TLS 1.2 or newer and SHOULD prefer TLS 1.3.
- An Edge and Coordinator MUST support a configurable server certificate/key pair, a trusted CA bundle, SNI, and certificate-chain delivery. Private keys, CA bundles, and certificate contents MUST never be emitted in logs, metrics, traces, or diagnostic endpoints.
- TLS is mandatory by default for every remote listener and client dial. An explicitly named development-only insecure mode MAY be available, but it MUST require an affirmative configuration flag, bind only to loopback by default, and emit a prominent startup warning. It MUST NOT be enabled by an empty or invalid TLS configuration.
- Use mutual TLS by default between Coordinator, Edge, and Control Plane. The server MUST validate client certificate chains and identities against the configured CA; peer identity allow-lists/SAN rules MUST be configurable per internal role.
- Any Go module that dials a remote CoCo component MUST validate the server chain and hostname/SNI, and accept explicit trust-store/CA, client certificate/key, server name, minimum TLS version, and connection-timeout configuration. Disabling verification is permitted only through an explicitly unsafe development option.
- Certificate and CA rotation MUST support overlap: accept the old and new trust roots during the transition, reload changed certificate material without dropping healthy connections where the Go runtime permits it, and report the active certificate expiry as a health/metric signal. A failed reload MUST retain the last known-good configuration and surface a structured error.
- TLS handshake failures, expired/not-yet-valid certificates, untrusted peers, unsupported protocol versions, and hostname mismatches MUST fail closed with actionable, non-secret error codes and telemetry.
- Authenticate end-user calls at the gateway/Edge boundary and authorize tenant, database, collection, and admin scopes.
- Propagate trace context, request IDs, deadlines, and cancellation to every fan-out RPC.

Minimum remote transport configuration:

```yaml
transport:
  listen: "0.0.0.0:7443"
  tls:
    enabled: true
    min_version: "1.2"       # 1.3 preferred when available
    cert_file: /run/coco/tls/server.crt
    key_file: /run/coco/tls/server.key
    client_ca_file: /run/coco/tls/clients-ca.crt # enables/requires mTLS when set
    reload_interval: "1m"    # optional file-watch/poll implementation
```

The configuration schema MUST reject a TLS-enabled listener with missing/unreadable certificate material, a key that does not match its certificate, an insecure protocol minimum, or `client_ca_file` combined with a mode that does not verify client certificates. File paths are examples only; deployments MAY supply the same material through a secret manager or in-memory credential provider.

---

# 173. GPU Vector Runtime

GPU support is an optional acceleration layer behind a narrow runtime interface. The persistent document record and durable vector index remain authoritative; GPU memory is always rebuildable cache/state.

```go
type VectorRuntime interface {
    Capabilities(ctx context.Context) (VectorCapabilities, error)
    SearchExact(ctx context.Context, req ExactSearchRequest) (SearchResult, error)
    SearchANN(ctx context.Context, req ANNSearchRequest) (SearchResult, error)
    WarmIndex(ctx context.Context, index IndexHandle) error
    EvictIndex(ctx context.Context, index IndexHandle) error
    Close() error
}
```

Required implementations:

```text
cpu     mandatory pure-Go reference runtime
gpu     optional runtime adapter, isolated from coco-core
auto    chooses a healthy compatible GPU runtime, otherwise cpu
```

The GPU adapter MAY use a vendor runtime or an isolated sidecar process; its API boundary MUST prevent driver/library dependencies from entering `coco-core`. Initial GPU scope is float32 cosine/L2/inner-product exact search, batch scoring, and reranking. GPU ANN/HNSW acceleration is a later optimization after the CPU HNSW implementation has correctness and recall baselines.

Rules:

- CPU exact search is the correctness oracle and mandatory fallback.
- GPU result ordering uses deterministic tie-breaks: score, then `RecordID`.
- GPU device loss, OOM, timeout, unsupported dimensions, or index-not-warm conditions MUST either fall back to CPU within the client deadline or return a typed unavailable error; no request may hang waiting for a device.
- Index warm-up is asynchronous, observable, capacity-bounded, and evictable. It MUST not block WAL recovery or a local write commit.
- Edges advertise device model, runtime version, supported metrics/encodings, available memory, load, and warm-index state. The coordinator uses these only as scheduling hints, never as a correctness dependency.
- GPU and CPU paths MUST be compared in conformance tests using tolerances declared per metric and encoding.

---

# 174. Distributed Delivery Milestones

These milestones begin only after the local embedded vector milestones are correct and benchmarked. They may proceed in parallel with non-invasive embedded maintenance work, but MUST NOT weaken the local engine's dependency policy.

## Milestone 28 — Protocol Foundation

```text
Versioned protobuf modules
buf/protoc generation and breaking-change CI
common error/status details
Go gRPC transport integration tests
TLS/mTLS development and production configurations
```

Acceptance: a Go integration test performs idempotent KV/document operations over the internal transport, and cancellation propagates to the Edge.

## Milestone 29 — Single Edge Server

```text
coco-edge lifecycle and configuration
gRPC KV, document, query, and vector adapters
server-streaming scans/search
admission limits, deadlines, tracing, metrics
```

Acceptance: remote operations have the same observable local semantics for a single database/shard.

## Milestone 30 — GPU Runtime Baseline

```text
CPU runtime interface and auto selector
GPU exact-search/batch-rerank adapter
capability advertisement, warm-up, eviction, and fallback telemetry
CPU/GPU numerical conformance and failure-injection tests
```

Acceptance: a GPU-less Edge remains fully functional; a GPU-equipped Edge improves benchmarked workloads without changing query correctness.

## Milestone 31 — Control Plane and Shard Routing

```text
Edge registration and health
versioned shard map with epochs
single-primary shard ownership
stale-map rejection and coordinator refresh
idempotency ledger and routed writes
```

Acceptance: a shard move/restart neither acknowledges an ambiguous write nor loses a confirmed local commit.

## Milestone 32 — Multi-Edge Query Coordinator

```text
scatter/gather RPCs
deadline budget propagation
global top-K k-way merge with oversampling
partial-result policy and Explain output
GPU-aware but CPU-safe Edge scheduling
```

Acceptance: multi-Edge vector results match the defined merged exact-search baseline; failed shards are explicitly represented.

## Milestone 33 — Distributed Hardening and Release

```text
chaos tests: Edge loss, GPU loss, stale maps, duplicate writes, slow streams
load tests across mixed CPU/GPU Edges
security review: mTLS, authorization, secret rotation
protobuf compatibility and upgrade/downgrade tests
operational runbooks and dashboards
```

Acceptance: the distributed release publishes supported consistency semantics, failure behavior, capacity limits, and upgrade path.

---

# 175. Distributed Testing and Observability

Required test matrix:

```text
1 Edge CPU / 1 Edge GPU / mixed CPU+GPU / 3+ Edge fan-out
fresh index / warming index / evicted index / GPU unavailable
valid map / stale map / Edge restart / duplicate write / deadline exceeded
embedded API and distributed-module conformance against the same protocol version
```

Required metrics include shard-map epoch, routed request count, idempotency hits, per-Edge latency, fan-out width, partial-query count, stream cancellation count, CPU/GPU selection and fallback count, device memory, warm-index bytes, vector recall, and merge oversampling factor. Every distributed request log and trace MUST include request ID, tenant/database, shard IDs, and Edge IDs, subject to configured privacy controls.

---

# 176. Updated Release Strategy

Add the following releases after the embedded v0.6 foundation:

## v0.7

```text
gRPC protocol
single Edge server
```

## v0.8

```text
optional GPU exact vector runtime
capability discovery and CPU fallback
```

## v0.9

```text
multi-Edge control plane
single-primary shard routing
distributed vector query coordinator
```

## v1.0

```text
stable embedded API and file format
stable v1 gRPC internal-contract compatibility policy
document + KV + vector + text
local ACID, recovery, backup, integrity
defined multi-Edge read/search and single-shard write semantics
```

---

# 177. Additional Architecture Invariants

1. The embedded core MUST compile and run without gRPC, protobuf, a GPU library, or a network connection.
2. The embedded Go API defines application behavior; protobuf is an internal distributed-module contract.
3. A distributed v1 write is atomic only within its owning shard's local CoCo transaction.
4. The coordinator MUST make partial, stale-routing, and retry outcomes observable to callers.
5. GPU acceleration MUST be optional, cancellable, capacity-bounded, and correctness-equivalent to the CPU-defined operation.
6. No distributed module may serialize internal pages, WAL records, or Go-specific types over gRPC.
