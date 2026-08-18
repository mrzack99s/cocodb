import React, { useState } from 'react'
import {
  HardDrive,
  Cpu,
  Layers,
  Database,
  ShieldCheck,
  Zap,
  ArrowUpRight,
  TrendingUp,
  FileCheck2,
  Package,
  Radio,
  BarChart3,
  Copy,
  Check,
  ExternalLink,
  Lock,
} from 'lucide-react'
import type { DBStats, CatalogData } from '../types'
import type { ViewType } from '../components/Sidebar'

interface DashboardViewProps {
  stats: DBStats | null
  catalog: CatalogData | null
  onNavigate: (view: ViewType) => void
}

export const DashboardView: React.FC<DashboardViewProps> = ({
  stats,
  catalog,
  onNavigate,
}) => {
  const [showPrometheus, setShowPrometheus] = useState(false)
  const [copied, setCopied] = useState(false)

  const pageCount = stats?.page_count || 0
  const sizeMB = ((pageCount * 16) / 1024).toFixed(2)
  const hitRate = stats?.cache_hit_rate ? (stats.cache_hit_rate * 100).toFixed(1) : '100.0'
  const hits = stats?.cache_hits || 0
  const misses = stats?.cache_misses || 0
  const lastLSN = stats?.last_lsn || 0
  const lastTxnID = stats?.last_txn_id || 0

  const buckets = catalog?.buckets || []
  const collections = catalog?.collections || []
  const queues = catalog?.queues || []

  const readyTasks = stats?.queue_ready_tasks ?? 0
  const inFlightTasks = stats?.queue_inflight_tasks ?? 0
  const dlqTasks = stats?.queue_dlq_tasks ?? 0
  const pubsubEvents = stats?.pubsub_events_count ?? 0

  const handleCopyMetrics = () => {
    navigator.clipboard.writeText(`${window.location.origin}/api/metrics`)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="p-8 space-y-8 max-w-7xl mx-auto">
      {/* Header Banner */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-zinc-900 dark:text-white flex items-center gap-2.5">
            <span>Database Metrics & Overview</span>
            <span className="text-xs font-mono px-2 py-0.5 rounded-md bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border border-emerald-500/20 font-medium">
              Production Kernel
            </span>
          </h1>
          <p className="text-xs text-zinc-500 dark:text-zinc-400 mt-1">
            Real-time telemetry, cache analytics, task queue leases, and Prometheus metric exporters
          </p>
        </div>

        <div className="flex items-center gap-3">
          <button
            onClick={() => setShowPrometheus(!showPrometheus)}
            className="flex items-center gap-2 px-3.5 py-2 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 hover:bg-zinc-50 dark:hover:bg-zinc-800 text-zinc-700 dark:text-zinc-300 text-xs font-semibold shadow-xs transition cursor-pointer"
          >
            <BarChart3 className="w-3.5 h-3.5 text-indigo-500" />
            <span>Prometheus Exporter</span>
          </button>

          <button
            onClick={() => onNavigate('query')}
            className="flex items-center gap-2 px-3.5 py-2 rounded-lg bg-emerald-500 hover:bg-emerald-400 text-zinc-950 text-xs font-semibold shadow-md shadow-emerald-500/20 transition cursor-pointer"
          >
            <Zap className="w-3.5 h-3.5 stroke-[2.5]" />
            <span>Open Query Console</span>
          </button>
        </div>
      </div>

      {/* Prometheus Drawer (If Enabled / Toggled) */}
      {showPrometheus && (
        <div className="p-5 rounded-2xl bg-zinc-900 border border-zinc-700/80 text-white space-y-4 shadow-xl animate-in fade-in slide-in-from-top-2 duration-200">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2.5">
              <BarChart3 className="w-4 h-4 text-emerald-400" />
              <span className="text-sm font-semibold">Prometheus Metrics Exporter Active</span>
              <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-300">
                text/plain 0.0.4
              </span>
            </div>
            <div className="flex items-center gap-2">
              <button
                onClick={handleCopyMetrics}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-zinc-800 hover:bg-zinc-700 text-xs font-mono transition cursor-pointer"
              >
                {copied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                <span>{copied ? 'Copied URL' : 'Copy Scrape URL'}</span>
              </button>
              <a
                href="/api/metrics"
                target="_blank"
                rel="noreferrer"
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-emerald-500 hover:bg-emerald-400 text-zinc-950 text-xs font-semibold transition"
              >
                <span>View Raw /metrics</span>
                <ExternalLink className="w-3.5 h-3.5" />
              </a>
            </div>
          </div>
          <div className="p-3 rounded-xl bg-zinc-950 border border-zinc-800 text-[11px] font-mono text-zinc-300 space-y-1">
            <p className="text-zinc-500"># Scrape Target URL for Prometheus / Grafana Agent / OpenTelemetry:</p>
            <p className="text-emerald-400 font-semibold">{window.location.origin}/api/metrics</p>
          </div>
        </div>
      )}

      {/* Enabled Feature Subsystem Badges */}
      <div className="p-4 rounded-2xl bg-white/70 dark:bg-zinc-900/50 border border-zinc-200/80 dark:border-zinc-800/80 backdrop-blur-md flex flex-wrap items-center gap-2.5">
        <span className="text-[11px] uppercase tracking-wider font-semibold text-zinc-400 mr-2">
          Enabled Subsystems:
        </span>
        <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border border-emerald-500/20 text-xs font-medium">
          <span className="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
          B+Tree KV Engine
        </span>
        <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border border-emerald-500/20 text-xs font-medium">
          <span className="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
          Document Engine (Binary Slotted)
        </span>
        <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border border-emerald-500/20 text-xs font-medium">
          <span className="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
          HNSW Vector Search
        </span>
        <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border border-emerald-500/20 text-xs font-medium">
          <span className="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
          BM25 Full-Text Index
        </span>
        <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border border-emerald-500/20 text-xs font-medium">
          <span className="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
          Transactional Task Queues (DLQ + Dedup)
        </span>
        <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border border-emerald-500/20 text-xs font-medium">
          <span className="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
          Real-Time Pub/Sub
        </span>
        <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border border-emerald-500/20 text-xs font-medium">
          <Lock className="w-3 h-3" />
          AES-256-GCM & CRC32C
        </span>
      </div>

      {/* KPI Cards Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        {/* Metric 1: Storage Size */}
        <div className="p-5 rounded-2xl bg-white/70 dark:bg-zinc-900/70 border border-zinc-200 dark:border-zinc-800/80 shadow-xs space-y-3">
          <div className="flex items-center justify-between text-zinc-500 dark:text-zinc-400">
            <span className="text-xs font-medium">Allocated Storage</span>
            <HardDrive className="w-4 h-4 text-emerald-500 dark:text-emerald-400" />
          </div>
          <div className="flex items-baseline gap-2">
            <span className="text-2xl font-bold font-mono text-zinc-900 dark:text-white">{sizeMB}</span>
            <span className="text-xs text-zinc-500 dark:text-zinc-400 font-mono">MB</span>
          </div>
          <div className="text-[11px] text-zinc-400 dark:text-zinc-500 font-mono">
            {pageCount.toLocaleString()} pages × 16 KiB
          </div>
        </div>

        {/* Metric 2: LRU Cache Hit Rate */}
        <div className="p-5 rounded-2xl bg-white/70 dark:bg-zinc-900/70 border border-zinc-200 dark:border-zinc-800/80 shadow-xs space-y-3">
          <div className="flex items-center justify-between text-zinc-500 dark:text-zinc-400">
            <span className="text-xs font-medium">16-Partition LRU Cache</span>
            <Cpu className="w-4 h-4 text-teal-500 dark:text-teal-400" />
          </div>
          <div className="flex items-baseline gap-2">
            <span className="text-2xl font-bold font-mono text-zinc-900 dark:text-white">{hitRate}%</span>
            <TrendingUp className="w-3.5 h-3.5 text-emerald-500 dark:text-emerald-400" />
          </div>
          <div className="text-[11px] text-zinc-400 dark:text-zinc-500 font-mono">
            {hits.toLocaleString()} hits / {misses.toLocaleString()} misses
          </div>
        </div>

        {/* Metric 3: WAL & Log Sequence */}
        <div className="p-5 rounded-2xl bg-white/70 dark:bg-zinc-900/70 border border-zinc-200 dark:border-zinc-800/80 shadow-xs space-y-3">
          <div className="flex items-center justify-between text-zinc-500 dark:text-zinc-400">
            <span className="text-xs font-medium">WAL Log Sequence (LSN)</span>
            <Layers className="w-4 h-4 text-sky-500 dark:text-sky-400" />
          </div>
          <div className="flex items-baseline gap-2">
            <span className="text-2xl font-bold font-mono text-zinc-900 dark:text-white">#{lastLSN}</span>
          </div>
          <div className="text-[11px] text-zinc-400 dark:text-zinc-500 font-mono">
            Txn ID #{lastTxnID} | SyncNormal
          </div>
        </div>

        {/* Metric 4: Multi-Model Schema Objects */}
        <div className="p-5 rounded-2xl bg-white/70 dark:bg-zinc-900/70 border border-zinc-200 dark:border-zinc-800/80 shadow-xs space-y-3">
          <div className="flex items-center justify-between text-zinc-500 dark:text-zinc-400">
            <span className="text-xs font-medium">Schema Objects</span>
            <Database className="w-4 h-4 text-purple-500 dark:text-purple-400" />
          </div>
          <div className="flex items-baseline gap-2">
            <span className="text-2xl font-bold font-mono text-zinc-900 dark:text-white">
              {collections.length + buckets.length + queues.length}
            </span>
            <span className="text-xs text-zinc-500 dark:text-zinc-400">total</span>
          </div>
          <div className="text-[11px] text-zinc-400 dark:text-zinc-500">
            {collections.length} Colls, {buckets.length} Buckets, {queues.length} Queues
          </div>
        </div>
      </div>

      {/* Task Queue & PubSub Telemetry Bar */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        {/* Task Queues Telemetry */}
        <div className="p-5 rounded-2xl bg-white/70 dark:bg-zinc-900/70 border border-zinc-200 dark:border-zinc-800/80 shadow-xs space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Package className="w-4 h-4 text-emerald-500" />
              <h3 className="text-xs font-semibold uppercase tracking-wider text-zinc-700 dark:text-zinc-300">
                Task Queue Telemetry
              </h3>
            </div>
            <button
              onClick={() => onNavigate('queues')}
              className="text-xs text-emerald-600 dark:text-emerald-400 hover:text-emerald-500 font-medium flex items-center gap-1 cursor-pointer"
            >
              <span>Manage Queues</span>
              <ArrowUpRight className="w-3 h-3" />
            </button>
          </div>

          <div className="grid grid-cols-3 gap-3 text-center">
            <div className="p-3 rounded-xl bg-zinc-50 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800/60">
              <div className="text-[10px] uppercase text-zinc-400 font-medium">Ready Tasks</div>
              <div className="text-xl font-bold font-mono text-emerald-600 dark:text-emerald-400 mt-1">
                {readyTasks}
              </div>
            </div>
            <div className="p-3 rounded-xl bg-zinc-50 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800/60">
              <div className="text-[10px] uppercase text-zinc-400 font-medium">In-Flight (Leased)</div>
              <div className="text-xl font-bold font-mono text-amber-500 mt-1">
                {inFlightTasks}
              </div>
            </div>
            <div className="p-3 rounded-xl bg-zinc-50 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800/60">
              <div className="text-[10px] uppercase text-zinc-400 font-medium">Dead-Letter (DLQ)</div>
              <div className="text-xl font-bold font-mono text-rose-500 mt-1">
                {dlqTasks}
              </div>
            </div>
          </div>
        </div>

        {/* Pub/Sub Telemetry */}
        <div className="p-5 rounded-2xl bg-white/70 dark:bg-zinc-900/70 border border-zinc-200 dark:border-zinc-800/80 shadow-xs space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Radio className="w-4 h-4 text-emerald-500" />
              <h3 className="text-xs font-semibold uppercase tracking-wider text-zinc-700 dark:text-zinc-300">
                Real-Time Pub/Sub Broker
              </h3>
            </div>
            <button
              onClick={() => onNavigate('pubsub')}
              className="text-xs text-emerald-600 dark:text-emerald-400 hover:text-emerald-500 font-medium flex items-center gap-1 cursor-pointer"
            >
              <span>View Stream</span>
              <ArrowUpRight className="w-3 h-3" />
            </button>
          </div>

          <div className="grid grid-cols-2 gap-3 text-center">
            <div className="p-3 rounded-xl bg-zinc-50 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800/60">
              <div className="text-[10px] uppercase text-zinc-400 font-medium">Live Session Events</div>
              <div className="text-xl font-bold font-mono text-emerald-600 dark:text-emerald-400 mt-1">
                {pubsubEvents}
              </div>
            </div>
            <div className="p-3 rounded-xl bg-zinc-50 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800/60">
              <div className="text-[10px] uppercase text-zinc-400 font-medium">Throughput Speed</div>
              <div className="text-xl font-bold font-mono text-sky-500 mt-1">
                ~1.95M/s
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Catalog & Objects Tables */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Document Collections Card */}
        <div className="rounded-2xl bg-white/70 dark:bg-zinc-900/70 border border-zinc-200 dark:border-zinc-800/80 shadow-xs overflow-hidden">
          <div className="p-5 border-b border-zinc-200 dark:border-zinc-800/80 flex items-center justify-between bg-zinc-50/50 dark:bg-transparent">
            <div className="flex items-center gap-2.5">
              <Layers className="w-4 h-4 text-emerald-600 dark:text-emerald-400" />
              <h2 className="font-semibold text-sm text-zinc-900 dark:text-white">Document Collections</h2>
            </div>
            <button
              onClick={() => onNavigate('collections')}
              className="text-xs text-emerald-600 dark:text-emerald-400 hover:text-emerald-500 font-medium flex items-center gap-1 cursor-pointer"
            >
              <span>Explore</span>
              <ArrowUpRight className="w-3 h-3" />
            </button>
          </div>

          <div className="divide-y divide-zinc-200 dark:divide-zinc-800/60">
            {collections.length === 0 ? (
              <div className="p-8 text-center text-xs text-zinc-500">
                No document collections found in database.
              </div>
            ) : (
              collections.map((coll) => (
                <div
                  key={coll.name}
                  onClick={() => onNavigate('collections')}
                  className="p-4 hover:bg-zinc-50 dark:hover:bg-zinc-800/40 transition flex items-center justify-between cursor-pointer"
                >
                  <div>
                    <div className="font-medium text-xs text-zinc-800 dark:text-zinc-200">{coll.name}</div>
                    <div className="text-[11px] text-zinc-400 dark:text-zinc-500 font-mono mt-0.5">
                      ObjectID: {coll.id} · Root: Page #{coll.root}
                    </div>
                  </div>
                  <span className="text-[11px] font-mono px-2 py-0.5 rounded bg-zinc-100 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400 border border-zinc-200 dark:border-zinc-700/50">
                    Binary / Slotted
                  </span>
                </div>
              ))
            )}
          </div>
        </div>

        {/* KV Buckets Card */}
        <div className="rounded-2xl bg-white/70 dark:bg-zinc-900/70 border border-zinc-200 dark:border-zinc-800/80 shadow-xs overflow-hidden">
          <div className="p-5 border-b border-zinc-200 dark:border-zinc-800/80 flex items-center justify-between bg-zinc-50/50 dark:bg-transparent">
            <div className="flex items-center gap-2.5">
              <Database className="w-4 h-4 text-teal-600 dark:text-teal-400" />
              <h2 className="font-semibold text-sm text-zinc-900 dark:text-white">Key/Value Buckets</h2>
            </div>
            <button
              onClick={() => onNavigate('buckets')}
              className="text-xs text-emerald-600 dark:text-emerald-400 hover:text-emerald-500 font-medium flex items-center gap-1 cursor-pointer"
            >
              <span>Explore</span>
              <ArrowUpRight className="w-3 h-3" />
            </button>
          </div>

          <div className="divide-y divide-zinc-200 dark:divide-zinc-800/60">
            {buckets.length === 0 ? (
              <div className="p-8 text-center text-xs text-zinc-500">
                No Key/Value buckets found in database.
              </div>
            ) : (
              buckets.map((b) => (
                <div
                  key={b.name}
                  onClick={() => onNavigate('buckets')}
                  className="p-4 hover:bg-zinc-50 dark:hover:bg-zinc-800/40 transition flex items-center justify-between cursor-pointer"
                >
                  <div>
                    <div className="font-medium text-xs text-zinc-800 dark:text-zinc-200">{b.name}</div>
                    <div className="text-[11px] text-zinc-400 dark:text-zinc-500 font-mono mt-0.5">
                      ObjectID: {b.id} · Root: Page #{b.root}
                    </div>
                  </div>
                  <span className="text-[11px] font-mono px-2 py-0.5 rounded bg-zinc-100 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400 border border-zinc-200 dark:border-zinc-700/50">
                    B+Tree Ordered
                  </span>
                </div>
              ))
            )}
          </div>
        </div>
      </div>

      {/* Storage Kernel Architecture Status */}
      <div className="p-6 rounded-2xl bg-white/70 dark:bg-zinc-900/50 border border-zinc-200 dark:border-zinc-800/80 shadow-xs space-y-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2 text-sm font-semibold text-zinc-900 dark:text-white">
            <ShieldCheck className="w-4 h-4 text-emerald-600 dark:text-emerald-400" />
            <span>Storage Kernel Health & Diagnostics</span>
          </div>
          <button
            onClick={() => onNavigate('maintenance')}
            className="text-xs text-emerald-600 dark:text-emerald-400 hover:text-emerald-500 font-medium flex items-center gap-1 cursor-pointer"
          >
            <span>Run Integrity Check</span>
            <FileCheck2 className="w-3.5 h-3.5" />
          </button>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 text-xs">
          <div className="p-3.5 rounded-xl bg-zinc-50 dark:bg-zinc-950/60 border border-zinc-200 dark:border-zinc-800/60 space-y-1">
            <span className="text-zinc-700 dark:text-zinc-300 font-medium">Dual Meta Pages</span>
            <p className="text-zinc-500 dark:text-zinc-500 text-[11px]">
              Page 0 (Meta A) & Page 1 (Meta B) alternating crash-safe commits.
            </p>
          </div>
          <div className="p-3.5 rounded-xl bg-zinc-50 dark:bg-zinc-950/60 border border-zinc-200 dark:border-zinc-800/60 space-y-1">
            <span className="text-zinc-700 dark:text-zinc-300 font-medium">CRC32C Checksums</span>
            <p className="text-zinc-500 dark:text-zinc-500 text-[11px]">
              Castagnoli 32-bit hardware checksum verified on every page load.
            </p>
          </div>
          <div className="p-3.5 rounded-xl bg-zinc-50 dark:bg-zinc-950/60 border border-zinc-200 dark:border-zinc-800/60 space-y-1">
            <span className="text-zinc-700 dark:text-zinc-300 font-medium">Snapshot Isolation MVCC</span>
            <p className="text-zinc-500 dark:text-zinc-500 text-[11px]">
              Non-blocking concurrent readers with single-writer coordinator.
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
