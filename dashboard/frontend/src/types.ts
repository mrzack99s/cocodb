export interface RealtimeTelemetry {
  timestamp: number
  page_count: number
  allocated_bytes: number
  cache_hits: number
  cache_misses: number
  cache_hit_rate: number
  last_lsn: number
  last_txn_id: number
  read_only: boolean
  uptime_seconds: number
  
  // Throughput (delta per sec)
  qps: number
  read_iops: number
  write_iops: number

  // Task Queues
  queue_count: number
  queue_ready_tasks: number
  queue_inflight_tasks: number
  queue_dlq_tasks: number

  // PubSub
  pubsub_events_count: number
  pubsub_delivered_count?: number
  pubsub_active_topics?: number
  pubsub_active_subs?: number

  // Latency Probe (microseconds)
  latency_p50_us?: number
  latency_p99_us?: number

  // Cluster (If active)
  cluster?: ClusterStatus | null
}

export interface PeerStatus {
  addr: string
  status: string
  latency_us: number
  is_local: boolean
}

export interface ClusterStatus {
  enabled: boolean
  node_id: string
  addr: string
  tls_enabled: boolean
  mtls_enforced: boolean
  auth_enforced: boolean
  total_nodes: number
  virtual_nodes: number
  active_conns: number
  peers: PeerStatus[]
}

export interface MetricHistoryPoint {
  time: string
  qps: number
  cache_hit_rate: number
  allocated_mb: number
  ready_tasks: number
  pubsub_events: number
}
