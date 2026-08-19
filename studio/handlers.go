package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/mrzack99s/cocodb"
	"github.com/mrzack99s/cocodb/document"
	"github.com/mrzack99s/cocodb/internal/vector"
	"github.com/mrzack99s/cocodb/kv"
	"github.com/mrzack99s/cocodb/pubsub"
	"github.com/mrzack99s/cocodb/queue"
	"github.com/mrzack99s/cocodb/search"
)

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/catalog", s.handleCatalog)

	// KV endpoints
	mux.HandleFunc("/api/kv/", s.handleKVRoute)

	// Document endpoints
	mux.HandleFunc("/api/doc/", s.handleDocRoute)

	// Time-series endpoints
	mux.HandleFunc("/api/timeseries/list", s.handleTimeSeriesList)
	mux.HandleFunc("/api/timeseries/query", s.handleTimeSeriesQuery)
	mux.HandleFunc("/api/timeseries/write", s.handleTimeSeriesWrite)
	mux.HandleFunc("/api/timeseries/prune", s.handleTimeSeriesPrune)

	// Search endpoints
	mux.HandleFunc("/api/vector/search", s.handleVectorSearch)
	mux.HandleFunc("/api/text/search", s.handleTextSearch)

	// Maintenance
	mux.HandleFunc("/api/integrity/check", s.handleIntegrityCheck)
	mux.HandleFunc("/api/maintenance/checkpoint", s.handleCheckpoint)
	mux.HandleFunc("/api/maintenance/backup", s.handleBackup)

	// Queue endpoints
	mux.HandleFunc("/api/queue/list", s.handleQueueList)
	mux.HandleFunc("/api/queue/stats", s.handleQueueStats)
	mux.HandleFunc("/api/queue/enqueue", s.handleQueueEnqueue)
	mux.HandleFunc("/api/queue/dequeue", s.handleQueueDequeue)

	// PubSub endpoints
	mux.HandleFunc("/api/pubsub/publish", s.handlePubSubPublish)
	mux.HandleFunc("/api/pubsub/history", s.handlePubSubHistory)

	// Metrics endpoints (Prometheus Exposition Format)
	mux.HandleFunc("/api/metrics", s.handleMetrics)
	mux.HandleFunc("/metrics", s.handleMetrics)

	// Cluster Status endpoint
	mux.HandleFunc("/api/cluster/status", s.handleClusterStatus)

	// Frontend SPA Static Assets
	s.registerStaticFileServer(mux)
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]string{"error": message})
}

// GET /api/stats
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
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

	psStats := s.db.PubSub().Stats()
	clusterStatus := s.db.ClusterStatus()

	s.writeJSON(w, http.StatusOK, map[string]any{
		"page_count":           stats.PageCount,
		"cache_hits":           stats.CacheHits,
		"cache_misses":         stats.CacheMisses,
		"cache_hit_rate":       stats.CacheHitRate,
		"last_lsn":             stats.LastLSN,
		"last_txn_id":          stats.LastTxnID,
		"read_only":            stats.ReadOnly,
		"queue_count":          len(queueNames),
		"queue_ready_tasks":    totalReady,
		"queue_inflight_tasks": totalInFlight,
		"queue_dlq_tasks":      totalDLQ,
		"pubsub_events_count":  psStats.TotalPublished,
		"cluster":              clusterStatus,
	})
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

// GET /api/catalog
func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	bucketNames := s.db.ListBuckets()
	collNames := s.db.ListCollections()
	queueNames := s.db.ListQueues()

	type item struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}

	buckets := make([]item, len(bucketNames))
	for i, b := range bucketNames {
		buckets[i] = item{Name: b, Type: "Bucket"}
	}

	// Time-series data has its own Studio explorer, so keep its internal
	// backing collections out of the regular document browser.
	colls := make([]item, 0, len(collNames))
	for _, c := range collNames {
		if strings.HasPrefix(c, "_ts:") {
			continue
		}
		colls = append(colls, item{Name: c, Type: "Collection"})
	}

	queues := make([]item, len(queueNames))
	for i, q := range queueNames {
		queues[i] = item{Name: q, Type: "Queue"}
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"buckets":     buckets,
		"collections": colls,
		"queues":      queues,
	})
}

// KV Router: /api/kv/{bucket}/{action}
func (s *Server) handleKVRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/kv/"), "/")
	if len(parts) < 2 {
		s.writeError(w, http.StatusBadRequest, "Invalid KV endpoint format")
		return
	}

	bucketName := parts[0]
	action := parts[1]

	switch action {
	case "scan":
		prefix := r.URL.Query().Get("prefix")
		bucket := s.db.Bucket(bucketName)
		var it *kv.Iterator
		if prefix != "" {
			it = bucket.Prefix([]byte(prefix))
		} else {
			it = bucket.Iterator()
		}
		defer it.Close()

		type entry struct {
			Key   string `json:"key"`
			Value string `json:"value"`
			Size  int    `json:"size"`
		}
		var entries []entry
		for it.Valid() && len(entries) < 100 {
			k := string(it.Key())
			v := string(it.Value())
			entries = append(entries, entry{
				Key:   k,
				Value: v,
				Size:  len(it.Value()),
			})
			it.Next()
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "total": len(entries)})

	case "get":
		key := r.URL.Query().Get("key")
		bucket := s.db.Bucket(bucketName)
		val, err := bucket.Get([]byte(key))
		if err != nil {
			s.writeError(w, http.StatusNotFound, "Key not found")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"key": key, "value": string(val)})

	case "put":
		if s.readOnly {
			s.writeError(w, http.StatusForbidden, "Database is in read-only mode")
			return
		}
		var req struct {
			Key   string `json:"key"`
			Value string `json:"value"`
			TTL   int    `json:"ttl"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		var opts []kv.Option
		if req.TTL > 0 {
			opts = append(opts, cocodb.TTL(time.Duration(req.TTL)*time.Second))
		}
		err := s.db.Update(func(tx *cocodb.Tx) error {
			return tx.Bucket(bucketName).Put([]byte(req.Key), []byte(req.Value), opts...)
		})
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]bool{"success": true})

	case "delete":
		if s.readOnly {
			s.writeError(w, http.StatusForbidden, "Database is in read-only mode")
			return
		}
		key := r.URL.Query().Get("key")
		err := s.db.Update(func(tx *cocodb.Tx) error {
			return tx.Bucket(bucketName).Delete([]byte(key))
		})
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]bool{"success": true})

	case "incr":
		if s.readOnly {
			s.writeError(w, http.StatusForbidden, "Database is in read-only mode")
			return
		}
		var req struct {
			Key   string `json:"key"`
			Delta int64  `json:"delta"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Delta == 0 {
			req.Delta = 1
		}
		var newVal int64
		err := s.db.Update(func(tx *cocodb.Tx) error {
			var err error
			newVal, err = tx.Bucket(bucketName).Increment([]byte(req.Key), req.Delta)
			return err
		})
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"new_value": newVal})

	default:
		s.writeError(w, http.StatusNotFound, "Unknown KV action")
	}
}

// Document Router: /api/doc/{collection}/{action}
func (s *Server) handleDocRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/doc/"), "/")
	if len(parts) < 2 {
		s.writeError(w, http.StatusBadRequest, "Invalid document endpoint format")
		return
	}

	collName := parts[0]
	action := parts[1]

	switch action {
	case "query":
		var req struct {
			Filters []struct {
				Field string `json:"field"`
				Op    string `json:"op"`
				Value any    `json:"value"`
			} `json:"filters"`
			OrderBy *struct {
				Field string `json:"field"`
				Desc  bool   `json:"desc"`
			} `json:"order_by"`
			Limit   int  `json:"limit"`
			Offset  int  `json:"offset"`
			Explain bool `json:"explain"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}

		coll := s.db.Collection(collName)
		q := coll.Query()
		for _, f := range req.Filters {
			switch f.Op {
			case "eq":
				q = q.Where(f.Field).Eq(f.Value)
			case "ne":
				q = q.Where(f.Field).Ne(f.Value)
			case "gt":
				q = q.Where(f.Field).Gt(f.Value)
			case "gte":
				q = q.Where(f.Field).Gte(f.Value)
			case "lt":
				q = q.Where(f.Field).Lt(f.Value)
			case "lte":
				q = q.Where(f.Field).Lte(f.Value)
			case "contains":
				q = q.Where(f.Field).Contains(fmt.Sprintf("%v", f.Value))
			}
		}

		if req.OrderBy != nil && req.OrderBy.Field != "" {
			if req.OrderBy.Desc {
				q = q.OrderBy(req.OrderBy.Field, cocodb.Desc)
			} else {
				q = q.OrderBy(req.OrderBy.Field, cocodb.Asc)
			}
		}

		if req.Limit > 0 {
			q = q.Limit(req.Limit)
		}
		if req.Offset > 0 {
			q = q.Offset(req.Offset)
		}

		planDesc, _ := q.Explain()
		if req.Explain {
			s.writeJSON(w, http.StatusOK, map[string]any{
				"execution_plan": planDesc,
				"documents":      []any{},
				"duration_ms":    0.1,
			})
			return
		}

		start := time.Now()
		docs, err := q.All()
		dur := float64(time.Since(start).Microseconds()) / 1000.0

		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		s.writeJSON(w, http.StatusOK, map[string]any{
			"documents":      docs,
			"count":          len(docs),
			"execution_plan": planDesc,
			"duration_ms":    dur,
		})

	case "get":
		id := r.URL.Query().Get("id")
		coll := s.db.Collection(collName)
		doc, err := coll.Get(id)
		if err != nil {
			s.writeError(w, http.StatusNotFound, "Document not found")
			return
		}
		s.writeJSON(w, http.StatusOK, doc)

	case "insert":
		if s.readOnly {
			s.writeError(w, http.StatusForbidden, "Database is in read-only mode")
			return
		}
		var doc cocodb.Document
		if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
			s.writeError(w, http.StatusBadRequest, "Invalid JSON document")
			return
		}
		var id string
		err := s.db.Update(func(tx *cocodb.Tx) error {
			var err error
			id, err = tx.Collection(collName).Insert(doc)
			return err
		})
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]string{"id": id})

	case "update":
		if s.readOnly {
			s.writeError(w, http.StatusForbidden, "Database is in read-only mode")
			return
		}
		id := r.URL.Query().Get("id")
		var doc cocodb.Document
		if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
			s.writeError(w, http.StatusBadRequest, "Invalid JSON document")
			return
		}
		err := s.db.Update(func(tx *cocodb.Tx) error {
			return tx.Collection(collName).Replace(id, doc)
		})
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]bool{"success": true})

	case "delete":
		if s.readOnly {
			s.writeError(w, http.StatusForbidden, "Database is in read-only mode")
			return
		}
		id := r.URL.Query().Get("id")
		err := s.db.Update(func(tx *cocodb.Tx) error {
			return tx.Collection(collName).Delete(id)
		})
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]bool{"success": true})

	default:
		s.writeError(w, http.StatusNotFound, "Unknown document action")
	}
}

// POST /api/vector/search
func (s *Server) handleVectorSearch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Collection string    `json:"collection"`
		Field      string    `json:"field"`
		Vector     []float32 `json:"vector"`
		K          int       `json:"k"`
		Metric     string    `json:"metric"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid vector query payload")
		return
	}

	if req.K <= 0 {
		req.K = 5
	}
	m := vector.Cosine
	switch req.Metric {
	case "l2":
		m = vector.L2
	case "dot":
		m = vector.DotProduct
	}

	coll := s.db.Collection(req.Collection)
	docs, err := coll.Query().All()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type matchRes struct {
		ID            int            `json:"id"`
		DocID         string         `json:"doc_id"`
		Distance      float32        `json:"distance"`
		SimilarityPct float32        `json:"similarity_pct"`
		Document      map[string]any `json:"document"`
	}

	var matches []matchRes
	for i, doc := range docs {
		if rawVec, ok := doc[req.Field]; ok {
			var vec []float32
			if arr, ok := rawVec.([]any); ok {
				for _, num := range arr {
					if f, ok := num.(float64); ok {
						vec = append(vec, float32(f))
					}
				}
			} else if fArr, ok := rawVec.([]float32); ok {
				vec = fArr
			}

			if len(vec) == len(req.Vector) {
				dist := vector.Distance(req.Vector, vec, m)
				simPct := float32(100.0)
				if dist > 0 && dist <= 1.0 {
					simPct = (1.0 - dist) * 100.0
				} else if dist > 1.0 {
					simPct = float32(100.0 / (1.0 + dist))
				}

				docID := fmt.Sprintf("%v", doc["_id"])
				matches = append(matches, matchRes{
					ID:            i + 1,
					DocID:         docID,
					Distance:      dist,
					SimilarityPct: simPct,
					Document:      doc,
				})
			}
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"matches": matches})
}

// POST /api/text/search
func (s *Server) handleTextSearch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Collection string `json:"collection"`
		Field      string `json:"field"`
		Query      string `json:"query"`
		K          int    `json:"k"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid search payload")
		return
	}

	if req.K <= 0 {
		req.K = 10
	}
	if strings.TrimSpace(req.Collection) == "" {
		s.writeError(w, http.StatusBadRequest, "Collection is required")
		return
	}
	if strings.TrimSpace(req.Query) == "" {
		s.writeError(w, http.StatusBadRequest, "Search query is required")
		return
	}

	coll := s.db.Collection(req.Collection)
	docs, err := coll.Query().All()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	idx := search.NewInvertedIndex()
	docMap := make(map[uint64]document.Document)

	for i, doc := range docs {
		recID := uint64(i + 1)
		docMap[recID] = doc
		if req.Field != "" {
			if txtVal, ok := doc[req.Field]; ok {
				idx.IndexDoc(recID, fmt.Sprintf("%v", txtVal))
			}
			continue
		}

		// With no field selected, search every top-level string field. This is
		// the useful default for an explorer because document schemas vary.
		var text strings.Builder
		for _, value := range doc {
			if value, ok := value.(string); ok {
				if text.Len() > 0 {
					text.WriteByte(' ')
				}
				text.WriteString(value)
			}
		}
		if text.Len() > 0 {
			idx.IndexDoc(recID, text.String())
		}
	}

	results := idx.Search(req.Query, req.K)

	type textRes struct {
		RecordID uint64         `json:"record_id"`
		DocID    string         `json:"doc_id"`
		Score    float64        `json:"score"`
		Document map[string]any `json:"document"`
	}

	var output []textRes
	for _, res := range results {
		d := docMap[res.RecordID]
		output = append(output, textRes{
			RecordID: res.RecordID,
			DocID:    fmt.Sprintf("%v", d["_id"]),
			Score:    res.Score,
			Document: d,
		})
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"results": output})
}

// GET /api/integrity/check
func (s *Server) handleIntegrityCheck(w http.ResponseWriter, r *http.Request) {
	rep, err := s.db.Check(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	errs := rep.Errors
	if errs == nil {
		errs = []string{}
	}
	warns := rep.Warnings
	if warns == nil {
		warns = []string{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"valid":         rep.Valid,
		"pages_checked": rep.PagesChecked,
		"errors":        errs,
		"warnings":      warns,
	})
}

// POST /api/maintenance/checkpoint
func (s *Server) handleCheckpoint(w http.ResponseWriter, r *http.Request) {
	err := s.db.Update(func(tx *cocodb.Tx) error {
		return nil
	})
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	stats := s.db.Stats()
	s.writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"last_lsn": stats.LastLSN,
	})
}

// POST /api/maintenance/backup
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filename string `json:"filename"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Filename == "" {
		req.Filename = fmt.Sprintf("backup-%d.coco", time.Now().Unix())
	}

	dstPath := filepath.Join(".", req.Filename)
	if err := s.db.Backup(r.Context(), dstPath); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"backup_path": dstPath,
	})
}

// GET /api/queue/list
func (s *Server) handleQueueList(w http.ResponseWriter, r *http.Request) {
	queueNames := s.db.ListQueues()
	type queueItem struct {
		Name     string      `json:"name"`
		Stats    queue.Stats `json:"stats"`
		HasDLQ   bool        `json:"has_dlq"`
		DLQStats queue.Stats `json:"dlq_stats"`
	}

	res := make([]queueItem, 0, len(queueNames))
	for _, name := range queueNames {
		q := s.db.Queue(name)
		stats := q.Stats()
		item := queueItem{
			Name:   name,
			Stats:  stats,
			HasDLQ: q.DLQ() != nil,
		}
		if dlq := q.DLQ(); dlq != nil {
			item.DLQStats = dlq.Stats()
		}
		res = append(res, item)
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"queues": res,
	})
}

// GET /api/queue/stats?name=tasks
func (s *Server) handleQueueStats(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		s.writeError(w, http.StatusBadRequest, "Missing queue name")
		return
	}
	q := s.db.Queue(name)
	stats := q.Stats()
	s.writeJSON(w, http.StatusOK, map[string]any{
		"name":            name,
		"ready_count":     stats.ReadyCount,
		"in_flight_count": stats.InFlightCount,
		"dlq_count":       stats.DLQCount,
	})
}

// POST /api/queue/enqueue
func (s *Server) handleQueueEnqueue(w http.ResponseWriter, r *http.Request) {
	if s.readOnly {
		s.writeError(w, http.StatusForbidden, "Studio is in read-only mode")
		return
	}

	var req struct {
		Queue    string `json:"queue"`
		Payload  string `json:"payload"`
		DedupID  string `json:"dedup_id"`
		Priority int    `json:"priority"`
		DelayMs  int    `json:"delay_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if req.Queue == "" {
		s.writeError(w, http.StatusBadRequest, "Missing queue name")
		return
	}

	q := s.db.Queue(req.Queue)
	var opts []queue.Option
	if req.DedupID != "" {
		opts = append(opts, queue.WithDedupID(req.DedupID, 5*time.Minute))
	}
	if req.Priority > 0 {
		opts = append(opts, queue.WithPriority(uint8(req.Priority)))
	}
	if req.DelayMs > 0 {
		opts = append(opts, queue.WithDelay(time.Duration(req.DelayMs)*time.Millisecond))
	}

	msg, err := q.Enqueue(r.Context(), []byte(req.Payload), opts...)
	if err != nil {
		if err == queue.ErrDuplicateMessage {
			s.writeError(w, http.StatusConflict, "Duplicate message rejected within active deduplication window")
			return
		}
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"message_id": msg.ID,
		"queue":      msg.Queue,
		"state":      msg.State.String(),
	})
}

// POST /api/queue/dequeue
func (s *Server) handleQueueDequeue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Queue   string `json:"queue"`
		AutoAck bool   `json:"auto_ack"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if req.Queue == "" {
		s.writeError(w, http.StatusBadRequest, "Missing queue name")
		return
	}

	q := s.db.Queue(req.Queue)
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	msg, err := q.Dequeue(ctx, queue.WithVisibilityTimeout(10*time.Second))
	if err != nil {
		if err == context.DeadlineExceeded || err == queue.ErrQueueEmpty {
			s.writeJSON(w, http.StatusOK, map[string]any{
				"found": false,
			})
			return
		}
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if req.AutoAck {
		_ = msg.Ack()
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"found":       true,
		"message_id":  msg.ID,
		"queue":       msg.Queue,
		"payload":     string(msg.Payload),
		"retry_count": msg.RetryCount,
		"priority":    msg.Priority,
		"state":       msg.State.String(),
	})
}

// POST /api/pubsub/publish
func (s *Server) handlePubSubPublish(w http.ResponseWriter, r *http.Request) {
	if s.readOnly {
		s.writeError(w, http.StatusForbidden, "Studio is in read-only mode")
		return
	}

	var req struct {
		Topic   string `json:"topic"`
		Payload string `json:"payload"`
		DedupID string `json:"dedup_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if req.Topic == "" {
		s.writeError(w, http.StatusBadRequest, "Missing topic")
		return
	}

	var opts []pubsub.Option
	if req.DedupID != "" {
		opts = append(opts, pubsub.WithDedupID(req.DedupID, 5*time.Minute))
	}

	count, err := s.db.Publish(r.Context(), req.Topic, []byte(req.Payload), opts...)
	if err != nil {
		if err == pubsub.ErrDuplicateMessage {
			s.writeError(w, http.StatusConflict, "Duplicate publication rejected within active deduplication window")
			return
		}
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Record in history buffer
	s.historyMu.Lock()
	event := PubSubEvent{
		ID:        fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		Topic:     req.Topic,
		Payload:   req.Payload,
		DedupID:   req.DedupID,
		CreatedAt: time.Now().UTC(),
	}
	s.pubsubHistory = append([]PubSubEvent{event}, s.pubsubHistory...)
	if len(s.pubsubHistory) > 50 {
		s.pubsubHistory = s.pubsubHistory[:50]
	}
	s.historyMu.Unlock()

	s.writeJSON(w, http.StatusOK, map[string]any{
		"success":         true,
		"topic":           req.Topic,
		"subscribers_hit": count,
		"event_id":        event.ID,
	})
}

// GET /api/pubsub/history
func (s *Server) handlePubSubHistory(w http.ResponseWriter, r *http.Request) {
	s.historyMu.RLock()
	defer s.historyMu.RUnlock()

	events := s.pubsubHistory
	if events == nil {
		events = []PubSubEvent{}
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"events": events,
	})
}

// GET /api/metrics or /metrics (Prometheus Exposition Format)
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
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

	psStats := s.db.PubSub().Stats()

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

	fmt.Fprintf(w, "# HELP cocodb_queue_inflight_tasks Total tasks in-flight under lease\n")
	fmt.Fprintf(w, "# TYPE cocodb_queue_inflight_tasks gauge\n")
	fmt.Fprintf(w, "cocodb_queue_inflight_tasks %d\n\n", totalInFlight)

	fmt.Fprintf(w, "# HELP cocodb_queue_dlq_tasks Total tasks in Dead-Letter Queue\n")
	fmt.Fprintf(w, "# TYPE cocodb_queue_dlq_tasks gauge\n")
	fmt.Fprintf(w, "cocodb_queue_dlq_tasks %d\n\n", totalDLQ)

	fmt.Fprintf(w, "# HELP cocodb_pubsub_events_total Total published events in session\n")
	fmt.Fprintf(w, "# TYPE cocodb_pubsub_events_total counter\n")
	fmt.Fprintf(w, "cocodb_pubsub_events_total %d\n\n", psStats.TotalPublished)

	fmt.Fprintf(w, "# HELP cocodb_pubsub_delivered_total Total delivered subscriber events\n")
	fmt.Fprintf(w, "# TYPE cocodb_pubsub_delivered_total counter\n")
	fmt.Fprintf(w, "cocodb_pubsub_delivered_total %d\n", psStats.TotalDelivered)
}
