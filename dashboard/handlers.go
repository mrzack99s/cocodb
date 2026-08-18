package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"
)

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/metrics/realtime", s.handleMetricsRealtime)
	mux.HandleFunc("/api/stream", s.handleStream)
	mux.HandleFunc("/api/prometheus", s.handlePrometheus)
	mux.HandleFunc("/api/benchmark/probe", s.handleBenchmarkProbe)
	mux.HandleFunc("/api/cluster/status", s.handleClusterStatus)
	mux.HandleFunc("/metrics", s.handlePrometheus)

	// Static SPA assets
	s.registerStaticFileServer(mux)
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// collectTelemetry gathers real-time engine telemetry and computes throughput delta.
func (s *Server) collectTelemetry() map[string]any {
	stats := s.db.Stats()
	queueNames := s.db.ListQueues()

	var totalReady, totalInFlight, totalDLQ int
	for _, name := range queueNames {
		q := s.db.Queue(name)
		qs := q.Stats()
		totalReady += qs.ReadyCount
		totalInFlight += qs.InFlightCount
		totalDLQ += qs.DLQCount
	}

	// Calculate throughput (QPS)
	s.throughputMu.Lock()
	now := time.Now()
	deltaSec := now.Sub(s.lastTime).Seconds()
	if deltaSec <= 0 {
		deltaSec = 0.5
	}
	currentOps := stats.CacheHits + stats.CacheMisses
	lastOps := s.lastHits + s.lastMisses
	var qps int64
	if currentOps >= lastOps && deltaSec > 0 {
		qps = int64(float64(currentOps-lastOps) / deltaSec)
	}
	s.lastHits = stats.CacheHits
	s.lastMisses = stats.CacheMisses
	s.lastTime = now
	s.throughputMu.Unlock()

	psStats := s.db.PubSub().Stats()
	clusterStatus := s.db.ClusterStatus()

	return map[string]any{
		"timestamp":              now.UnixMilli(),
		"page_count":             stats.PageCount,
		"allocated_bytes":        stats.PageCount * 16384,
		"cache_hits":             stats.CacheHits,
		"cache_misses":           stats.CacheMisses,
		"cache_hit_rate":         stats.CacheHitRate,
		"last_lsn":               stats.LastLSN,
		"last_txn_id":            stats.LastTxnID,
		"read_only":              stats.ReadOnly,
		"qps":                    qps,
		"queue_count":            len(queueNames),
		"queue_ready_tasks":      totalReady,
		"queue_inflight_tasks":   totalInFlight,
		"queue_dlq_tasks":        totalDLQ,
		"pubsub_events_count":    psStats.TotalPublished,
		"pubsub_delivered_count": psStats.TotalDelivered,
		"pubsub_active_topics":   psStats.ActiveTopics,
		"pubsub_active_subs":     psStats.ActiveSubs,
		"cluster":                clusterStatus,
	}
}

// GET /api/cluster/status
func (s *Server) handleClusterStatus(w http.ResponseWriter, r *http.Request) {
	status := s.db.ClusterStatus()
	if status == nil {
		s.writeJSON(w, http.StatusOK, map[string]any{
			"enabled": false,
			"message": "Cluster mode not active (standalone node)",
		})
		return
	}
	s.writeJSON(w, http.StatusOK, status)
}

// GET /api/metrics/realtime
func (s *Server) handleMetricsRealtime(w http.ResponseWriter, r *http.Request) {
	data := s.collectTelemetry()
	s.writeJSON(w, http.StatusOK, data)
}

// GET /api/stream (Server-Sent Events)
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Initial message
	initialData := s.collectTelemetry()
	initialBytes, _ := json.Marshal(initialData)
	fmt.Fprintf(w, "data: %s\n\n", initialBytes)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			data := s.collectTelemetry()
			dataBytes, err := json.Marshal(data)
			if err != nil {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", dataBytes)
			flusher.Flush()
		}
	}
}

// GET /metrics or /api/prometheus (Prometheus Exposition Format)
func (s *Server) handlePrometheus(w http.ResponseWriter, r *http.Request) {
	stats := s.db.Stats()
	queueNames := s.db.ListQueues()

	var totalReady, totalInFlight, totalDLQ int
	for _, name := range queueNames {
		q := s.db.Queue(name)
		qs := q.Stats()
		totalReady += qs.ReadyCount
		totalInFlight += qs.InFlightCount
		totalDLQ += qs.DLQCount
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, "# HELP cocodb_storage_allocated_bytes Total allocated storage in bytes\n")
	fmt.Fprintf(w, "# TYPE cocodb_storage_allocated_bytes gauge\n")
	fmt.Fprintf(w, "cocodb_storage_allocated_bytes %d\n\n", stats.PageCount*16384)

	fmt.Fprintf(w, "# HELP cocodb_page_count Total 16KiB pages allocated\n")
	fmt.Fprintf(w, "# TYPE cocodb_page_count gauge\n")
	fmt.Fprintf(w, "cocodb_page_count %d\n\n", stats.PageCount)

	fmt.Fprintf(w, "# HELP cocodb_cache_hits_total Total page cache hits\n")
	fmt.Fprintf(w, "# TYPE cocodb_cache_hits_total counter\n")
	fmt.Fprintf(w, "cocodb_cache_hits_total %d\n\n", stats.CacheHits)

	fmt.Fprintf(w, "# HELP cocodb_cache_misses_total Total page cache misses\n")
	fmt.Fprintf(w, "# TYPE cocodb_cache_misses_total counter\n")
	fmt.Fprintf(w, "cocodb_cache_misses_total %d\n\n", stats.CacheMisses)

	fmt.Fprintf(w, "# HELP cocodb_cache_hit_rate_ratio Cache hit rate ratio (0.0 to 1.0)\n")
	fmt.Fprintf(w, "# TYPE cocodb_cache_hit_rate_ratio gauge\n")
	fmt.Fprintf(w, "cocodb_cache_hit_rate_ratio %.4f\n\n", stats.CacheHitRate)

	fmt.Fprintf(w, "# HELP cocodb_wal_last_lsn Current WAL Log Sequence Number\n")
	fmt.Fprintf(w, "# TYPE cocodb_wal_last_lsn counter\n")
	fmt.Fprintf(w, "cocodb_wal_last_lsn %d\n\n", stats.LastLSN)

	fmt.Fprintf(w, "# HELP cocodb_last_txn_id Current transaction ID\n")
	fmt.Fprintf(w, "# TYPE cocodb_last_txn_id counter\n")
	fmt.Fprintf(w, "cocodb_last_txn_id %d\n\n", stats.LastTxnID)

	fmt.Fprintf(w, "# HELP cocodb_queues_total Total registered queues\n")
	fmt.Fprintf(w, "# TYPE cocodb_queues_total gauge\n")
	fmt.Fprintf(w, "cocodb_queues_total %d\n\n", len(queueNames))

	fmt.Fprintf(w, "# HELP cocodb_queue_ready_tasks Total tasks ready for workers\n")
	fmt.Fprintf(w, "# TYPE cocodb_queue_ready_tasks gauge\n")
	fmt.Fprintf(w, "cocodb_queue_ready_tasks %d\n\n", totalReady)

	fmt.Fprintf(w, "# HELP cocodb_queue_dlq_tasks Total tasks in Dead-Letter Queue\n")
	fmt.Fprintf(w, "# TYPE cocodb_queue_dlq_tasks gauge\n")
	fmt.Fprintf(w, "cocodb_queue_dlq_tasks %d\n\n", totalDLQ)

	psStats := s.db.PubSub().Stats()
	fmt.Fprintf(w, "# HELP cocodb_pubsub_events_total Total published events in session\n")
	fmt.Fprintf(w, "# TYPE cocodb_pubsub_events_total counter\n")
	fmt.Fprintf(w, "cocodb_pubsub_events_total %d\n\n", psStats.TotalPublished)

	fmt.Fprintf(w, "# HELP cocodb_pubsub_delivered_total Total delivered subscriber events\n")
	fmt.Fprintf(w, "# TYPE cocodb_pubsub_delivered_total counter\n")
	fmt.Fprintf(w, "cocodb_pubsub_delivered_total %d\n", psStats.TotalDelivered)
}

// POST /api/benchmark/probe
func (s *Server) handleBenchmarkProbe(w http.ResponseWriter, r *http.Request) {
	bucket := s.db.Bucket("metrics_probe")
	samples := make([]int64, 0, 1000)

	startTotal := time.Now()
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("probe_key_%d", i%50)
		t0 := time.Now()
		_ = bucket.Put([]byte(key), []byte("sample_payload_probe_data"))
		_, _ = bucket.Get([]byte(key))
		samples = append(samples, time.Since(t0).Microseconds())
	}
	_ = time.Since(startTotal)

	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	p50 := samples[len(samples)*50/100]
	p99 := samples[len(samples)*99/100]

	s.writeJSON(w, http.StatusOK, map[string]any{
		"ops": 2000,
		"p50": p50,
		"p99": p99,
	})
}
