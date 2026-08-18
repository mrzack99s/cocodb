package studio_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"cocodb"
	"cocodb/studio"
)

func setupTestDB(t *testing.T) *cocodb.DB {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "studio_test.coco")
	db, err := cocodb.Open(dbPath, cocodb.Profile(cocodb.Balanced))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	return db
}

func TestStudioAPIEndpoints(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	srv := studio.NewServer(db, ":0")

	// 1. Stats endpoint
	req := httptest.NewRequest("GET", "/api/stats", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/stats status = %d, want 200", rec.Code)
	}

	var stats map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&stats); err != nil {
		t.Fatalf("Decode stats response failed: %v", err)
	}
	if _, ok := stats["page_count"]; !ok {
		t.Fatalf("stats missing page_count field")
	}

	// 2. Put KV
	kvPutBody, _ := json.Marshal(map[string]any{
		"key":   "app:config:theme",
		"value": "dark",
	})
	req = httptest.NewRequest("POST", "/api/kv/settings/put", bytes.NewReader(kvPutBody))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/kv/settings/put status = %d", rec.Code)
	}

	// 3. Get KV
	req = httptest.NewRequest("GET", "/api/kv/settings/get?key=app:config:theme", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/kv/settings/get status = %d", rec.Code)
	}

	var kvGet map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&kvGet)
	if kvGet["value"] != "dark" {
		t.Fatalf("KV value mismatch: %v", kvGet)
	}

	// 4. Scan KV
	req = httptest.NewRequest("GET", "/api/kv/settings/scan?prefix=app:", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/kv/settings/scan status = %d", rec.Code)
	}

	// 5. Insert Document
	docBody, _ := json.Marshal(map[string]any{
		"_id":       "u1",
		"name":      "John Doe",
		"email":     "john@example.com",
		"age":       32,
		"active":    true,
		"title":     "Software Architecture with Go and Vector Embeddings",
		"embedding": []float32{0.1, 0.2, 0.3, 0.4},
	})
	req = httptest.NewRequest("POST", "/api/doc/users/insert", bytes.NewReader(docBody))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/doc/users/insert status = %d", rec.Code)
	}

	// 6. Query Document with Filters and Explain
	queryBody, _ := json.Marshal(map[string]any{
		"filters": []map[string]any{
			{"field": "active", "op": "eq", "value": true},
		},
		"limit": 10,
	})
	req = httptest.NewRequest("POST", "/api/doc/users/query", bytes.NewReader(queryBody))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/doc/users/query status = %d", rec.Code)
	}

	var queryRes map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&queryRes)
	docs := queryRes["documents"].([]any)
	if len(docs) != 1 {
		t.Fatalf("query expected 1 document, got %d", len(docs))
	}

	// 7. Vector Search
	vecBody, _ := json.Marshal(map[string]any{
		"collection": "users",
		"field":      "embedding",
		"vector":     []float32{0.1, 0.2, 0.3, 0.4},
		"k":          5,
		"metric":     "cosine",
	})
	req = httptest.NewRequest("POST", "/api/vector/search", bytes.NewReader(vecBody))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/vector/search status = %d", rec.Code)
	}

	// 8. Full-Text Search
	textBody, _ := json.Marshal(map[string]any{
		"collection": "users",
		"field":      "title",
		"query":      "software architecture",
		"k":          5,
	})
	req = httptest.NewRequest("POST", "/api/text/search", bytes.NewReader(textBody))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/text/search status = %d", rec.Code)
	}

	// 9. Integrity Check
	req = httptest.NewRequest("GET", "/api/integrity/check", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/integrity/check status = %d", rec.Code)
	}

	// 10. Catalog List
	req = httptest.NewRequest("GET", "/api/catalog", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/catalog status = %d", rec.Code)
	}

	var catalogRes map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&catalogRes)
	buckets := catalogRes["buckets"].([]any)
	colls := catalogRes["collections"].([]any)
	if len(buckets) == 0 || len(colls) == 0 {
		t.Fatalf("catalog expected buckets and collections, got buckets=%d, colls=%d", len(buckets), len(colls))
	}

	// 11. Queue Enqueue & Dequeue
	qEnqueueBody, _ := json.Marshal(map[string]any{
		"queue":    "studio_tasks",
		"payload":  "{\"task\": \"send_email\"}",
		"dedup_id": "email_task_1",
		"priority": 200,
	})
	req = httptest.NewRequest("POST", "/api/queue/enqueue", bytes.NewReader(qEnqueueBody))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/queue/enqueue status = %d", rec.Code)
	}

	// Queue List
	req = httptest.NewRequest("GET", "/api/queue/list", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/queue/list status = %d", rec.Code)
	}

	// Queue Dequeue
	qDequeueBody, _ := json.Marshal(map[string]any{
		"queue":    "studio_tasks",
		"auto_ack": true,
	})
	req = httptest.NewRequest("POST", "/api/queue/dequeue", bytes.NewReader(qDequeueBody))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/queue/dequeue status = %d", rec.Code)
	}

	// 12. PubSub Publish & History
	pubBody, _ := json.Marshal(map[string]any{
		"topic":    "system.alerts",
		"payload":  "high CPU usage warning",
		"dedup_id": "alert_101",
	})
	req = httptest.NewRequest("POST", "/api/pubsub/publish", bytes.NewReader(pubBody))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/pubsub/publish status = %d", rec.Code)
	}

	// 13. Prometheus Metrics Endpoint
	req = httptest.NewRequest("GET", "/api/metrics", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/metrics status = %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("cocodb_storage_allocated_bytes")) {
		t.Fatalf("GET /api/metrics response does not contain Prometheus metrics")
	}
}
