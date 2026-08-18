package dashboard_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"cocodb"
	"cocodb/dashboard"
)

func setupTestDB(t *testing.T) *cocodb.DB {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "dashboard_test.coco")
	db, err := cocodb.Open(dbPath, cocodb.Profile(cocodb.Balanced))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	return db
}

func TestDashboardEndpoints(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	srv := dashboard.NewServer(db, ":9090")

	// 1. Real-time telemetry endpoint
	req := httptest.NewRequest("GET", "/api/metrics/realtime", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/metrics/realtime status = %d, want 200", rec.Code)
	}

	var telemetry map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&telemetry); err != nil {
		t.Fatalf("Decode telemetry response failed: %v", err)
	}
	if _, ok := telemetry["page_count"]; !ok {
		t.Fatalf("missing page_count field in telemetry response")
	}

	// 2. Prometheus Endpoint
	req = httptest.NewRequest("GET", "/metrics", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want 200", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("cocodb_storage_allocated_bytes")) {
		t.Fatalf("GET /metrics response missing Prometheus metrics")
	}

	// 3. Synthetic Benchmark Probe
	req = httptest.NewRequest("POST", "/api/benchmark/probe", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/benchmark/probe status = %d, want 200", rec.Code)
	}

	var probe map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&probe); err != nil {
		t.Fatalf("Decode probe response failed: %v", err)
	}
	if _, ok := probe["p50"]; !ok {
		t.Fatalf("missing p50 in probe result")
	}
}
