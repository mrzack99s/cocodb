export interface DBStats {
  page_count: number
  cache_hits: number
  cache_misses: number
  cache_hit_rate: number
  last_lsn: number
  last_txn_id: number
  read_only: boolean
  uptime_seconds?: number
  queue_count?: number
  queue_ready_tasks?: number
  queue_inflight_tasks?: number
  queue_dlq_tasks?: number
  pubsub_events_count?: number
}

export interface CatalogObject {
  id: number
  type: string
  name: string
  root: number
  flags: number
  indexes?: string[]
}

export interface CatalogData {
  buckets: CatalogObject[]
  collections: CatalogObject[]
  queues?: CatalogObject[]
}

export interface TimeSeriesPoint {
  Timestamp: string
  Tags: Record<string, string>
  Fields: Record<string, any>
}

export interface KVEntry {
  key: string
  value: string
  size: number
  is_json?: boolean
}

export interface QueryResult {
  documents: Record<string, any>[]
  count: number
  execution_plan?: string
  duration_ms: number
}

export interface VectorSearchResult {
  id: number
  doc_id?: string
  distance: number
  similarity_pct: number
  document?: Record<string, any>
}

export interface TextSearchResult {
  record_id: number
  doc_id?: string
  score: number
  document?: Record<string, any>
}

export interface IntegrityReport {
  valid: boolean
  pages_checked: number
  errors: string[]
  warnings: string[]
}

export interface QueueStats {
  ReadyCount: number
  InFlightCount: number
  DLQCount: number
}

export interface QueueItem {
  name: string
  stats: QueueStats
  has_dlq: boolean
  dlq_stats?: QueueStats
}

export interface PubSubEvent {
  id: string
  topic: string
  payload: string
  dedup_id?: string
  created_at: string
}
