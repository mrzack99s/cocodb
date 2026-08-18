# CoCo Database Examples

This directory contains complete, runnable examples showcasing all capabilities of the **CoCo Multi-Model Embedded Database**.

## Examples Overview

| Directory | Feature Demonstrated | How to Run |
| :--- | :--- | :--- |
| **`01_basic_kv/`** | Key/Value CRUD, TTL Expiration, Prefix Scans, Atomic Increments, ACID Transactions | `go run examples/01_basic_kv/main.go` |
| **`02_document_query/`** | Document Collections, Secondary Indexes, Fluent Query Builder, Explain Plan, Document Updates | `go run examples/02_document_query/main.go` |
| **`03_vector_similarity/`** | Persistent HNSW Vector Graph, Cosine/L2/Dot Product metrics, Top-K Nearest Neighbor Searches | `go run examples/03_vector_similarity/main.go` |
| **`04_fulltext_bm25/`** | Unicode Tokenizer, Inverted Index Postings, BM25 Relevance Scoring | `go run examples/04_fulltext_bm25/main.go` |
| **`05_admin_studio/`** | Full Multi-Model Application with interactive Web Admin Studio on `http://localhost:8787` | `go run examples/05_admin_studio/main.go` |

---

## Running the Examples

### 1. Basic Key/Value Operations
```bash
go run examples/01_basic_kv/main.go
```

### 2. Document Collections & Queries
```bash
go run examples/02_document_query/main.go
```

### 3. Vector Similarity Search (HNSW)
```bash
go run examples/03_vector_similarity/main.go
```

### 4. Full-Text BM25 Search
```bash
go run examples/04_fulltext_bm25/main.go
```

### 5. Interactive Web Admin Studio
```bash
go run examples/05_admin_studio/main.go
```
Open **[http://localhost:8787](http://localhost:8787)** in your browser to explore the database via the GUI.
