import type { DBStats, CatalogData, KVEntry, QueryResult, VectorSearchResult, TextSearchResult, IntegrityReport, TimeSeriesPoint } from './types'

const BASE_URL = '/api'

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    headers: {
      'Content-Type': 'application/json',
      ...options.headers,
    },
    ...options,
  })

  if (!res.ok) {
    let errorMsg = `HTTP Error ${res.status}: ${res.statusText}`
    try {
      const data = await res.json()
      if (data.error) errorMsg = data.error
    } catch {
      // ignore
    }
    throw new Error(errorMsg)
  }

  return res.json()
}

export const api = {
  // Stats
  getStats: () => request<DBStats>('/stats'),

  // Catalog
  getCatalog: () => request<CatalogData>('/catalog'),

  // Time series
  listTimeSeries: () => request<{ series: string[] }>('/timeseries/list'),

  queryTimeSeries: (payload: {
    series: string
    start?: string
    end?: string
    tags?: Record<string, string>
    limit?: number
    descending?: boolean
  }) => request<{ points: TimeSeriesPoint[]; count: number }>('/timeseries/query', {
    method: 'POST',
    body: JSON.stringify(payload),
  }),

  writeTimeSeries: (series: string, point: TimeSeriesPoint) =>
    request<{ id: string }>('/timeseries/write', {
      method: 'POST',
      body: JSON.stringify({ series, point }),
    }),

  pruneTimeSeries: (series: string, before: string) =>
    request<{ removed: number }>('/timeseries/prune', {
      method: 'POST',
      body: JSON.stringify({ series, before }),
    }),

  // KV Buckets
  scanBucket: (bucket: string, prefix = '', limit = 100) =>
    request<{ entries: KVEntry[]; total: number }>(`/kv/${encodeURIComponent(bucket)}/scan?prefix=${encodeURIComponent(prefix)}&limit=${limit}`),

  getKV: (bucket: string, key: string) =>
    request<{ key: string; value: string }>(`/kv/${encodeURIComponent(bucket)}/get?key=${encodeURIComponent(key)}`),

  putKV: (bucket: string, key: string, value: string, ttlSeconds?: number) =>
    request<{ success: boolean }>(`/kv/${encodeURIComponent(bucket)}/put`, {
      method: 'POST',
      body: JSON.stringify({ key, value, ttl: ttlSeconds }),
    }),

  deleteKV: (bucket: string, key: string) =>
    request<{ success: boolean }>(`/kv/${encodeURIComponent(bucket)}/delete?key=${encodeURIComponent(key)}`, {
      method: 'DELETE',
    }),

  incrementKV: (bucket: string, key: string, delta = 1) =>
    request<{ new_value: number }>(`/kv/${encodeURIComponent(bucket)}/incr`, {
      method: 'POST',
      body: JSON.stringify({ key, delta }),
    }),

  // Document Collections
  queryDocuments: (collection: string, queryPayload: {
    filters?: { field: string; op: string; value: any }[]
    order_by?: { field: string; desc: boolean }
    limit?: number
    offset?: number
    explain?: boolean
  }) => request<QueryResult>(`/doc/${encodeURIComponent(collection)}/query`, {
    method: 'POST',
    body: JSON.stringify(queryPayload),
  }),

  getDocument: (collection: string, id: string) =>
    request<Record<string, any>>(`/doc/${encodeURIComponent(collection)}/get?id=${encodeURIComponent(id)}`),

  insertDocument: (collection: string, doc: Record<string, any>) =>
    request<{ id: string }>(`/doc/${encodeURIComponent(collection)}/insert`, {
      method: 'POST',
      body: JSON.stringify(doc),
    }),

  updateDocument: (collection: string, id: string, doc: Record<string, any>) =>
    request<{ success: boolean }>(`/doc/${encodeURIComponent(collection)}/update?id=${encodeURIComponent(id)}`, {
      method: 'POST',
      body: JSON.stringify(doc),
    }),

  deleteDocument: (collection: string, id: string) =>
    request<{ success: boolean }>(`/doc/${encodeURIComponent(collection)}/delete?id=${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),

  // Vector Search
  searchVectors: (collection: string, field: string, vector: number[], k = 5, metric = 'cosine') =>
    request<{ matches: VectorSearchResult[] }>('/vector/search', {
      method: 'POST',
      body: JSON.stringify({ collection, field, vector, k, metric }),
    }),

  // Full-Text Search
  searchFullText: (collection: string, field: string, queryText: string, k = 10) =>
    request<{ results: TextSearchResult[] }>('/text/search', {
      method: 'POST',
      body: JSON.stringify({ collection, field, query: queryText, k }),
    }),

  // Maintenance & Integrity
  checkIntegrity: () => request<IntegrityReport>('/integrity/check'),

  checkpointWAL: () =>
    request<{ success: boolean; last_lsn: number }>('/maintenance/checkpoint', {
      method: 'POST',
    }),

  triggerBackup: (filename?: string) =>
    request<{ success: boolean; backup_path: string }>('/maintenance/backup', {
      method: 'POST',
      body: JSON.stringify({ filename }),
    }),

  // Task Queues
  listQueues: () => request<{ queues: import('./types').QueueItem[] }>('/queue/list'),

  getQueueStats: (name: string) =>
    request<{ name: string; ready_count: number; in_flight_count: number; dlq_count: number }>(`/queue/stats?name=${encodeURIComponent(name)}`),

  enqueueTask: (queueName: string, payload: string, dedupId?: string, priority = 128, delayMs = 0) =>
    request<{ success: boolean; message_id: string; queue: string; state: string }>('/queue/enqueue', {
      method: 'POST',
      body: JSON.stringify({ queue: queueName, payload, dedup_id: dedupId, priority, delay_ms: delayMs }),
    }),

  dequeueTask: (queueName: string, autoAck = true) =>
    request<{ found: boolean; message_id?: string; queue?: string; payload?: string; retry_count?: number; priority?: number; state?: string }>('/queue/dequeue', {
      method: 'POST',
      body: JSON.stringify({ queue: queueName, auto_ack: autoAck }),
    }),

  // Pub/Sub
  publishPubSub: (topic: string, payload: string, dedupId?: string) =>
    request<{ success: boolean; topic: string; subscribers_hit: number; event_id: string }>('/pubsub/publish', {
      method: 'POST',
      body: JSON.stringify({ topic, payload, dedup_id: dedupId }),
    }),

  getPubSubHistory: () => request<{ events: import('./types').PubSubEvent[] }>('/pubsub/history'),
}
