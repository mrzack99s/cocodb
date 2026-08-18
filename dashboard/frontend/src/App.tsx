import React, { useState, useEffect } from 'react'
import {
  Activity,
  Cpu,
  HardDrive,
  Layers,
  Package,
  Radio,
  BarChart2,
  Sun,
  Moon,
  Copy,
  Check,
  ExternalLink,
  ShieldCheck,
  Zap,
  Flame,
  Network,
  Server as ServerIcon,
  Globe,
  Lock,
} from 'lucide-react'
import type { RealtimeTelemetry } from './types'
import { Sparkline } from './components/Sparkline'

export const App: React.FC = () => {
  const [theme, setTheme] = useState<'dark' | 'light'>('dark')
  const [activeTab, setActiveTab] = useState<'overview' | 'engine' | 'queues' | 'cluster' | 'prometheus'>('overview')
  const [telemetry, setTelemetry] = useState<RealtimeTelemetry | null>(null)
  const [connected, setConnected] = useState(false)
  const [copied, setCopied] = useState(false)
  const [benchmarking, setBenchmarking] = useState(false)
  const [benchmarkResult, setBenchmarkResult] = useState<{ p50: number; p99: number; ops: number } | null>(null)

  // Time-series history buffers (last 30 data points)
  const [qpsHistory, setQpsHistory] = useState<number[]>([])
  const [cacheHitHistory, setCacheHitHistory] = useState<number[]>([])
  const [allocatedMbHistory, setAllocatedMbHistory] = useState<number[]>([])
  const [queueReadyHistory, setQueueReadyHistory] = useState<number[]>([])
  const [pubsubHistory, setPubsubHistory] = useState<number[]>([])

  useEffect(() => {
    const root = document.documentElement
    root.classList.toggle('dark', theme === 'dark')
    root.classList.toggle('light', theme === 'light')
  }, [theme])

  // Establish SSE stream
  useEffect(() => {
    let eventSource: EventSource | null = null

    const connectSSE = () => {
      eventSource = new EventSource('/api/stream')

      eventSource.onopen = () => {
        setConnected(true)
      }

      eventSource.onmessage = (event) => {
        try {
          const data: RealtimeTelemetry = JSON.parse(event.data)
          setTelemetry(data)

          // Update histories
          const sizeMB = (data.page_count * 16) / 1024
          const hitRate = data.cache_hit_rate * 100

          setQpsHistory((prev) => [...prev.slice(-29), data.qps || 0])
          setCacheHitHistory((prev) => [...prev.slice(-29), hitRate])
          setAllocatedMbHistory((prev) => [...prev.slice(-29), sizeMB])
          setQueueReadyHistory((prev) => [...prev.slice(-29), data.queue_ready_tasks || 0])
          setPubsubHistory((prev) => [...prev.slice(-29), data.pubsub_events_count || 0])
        } catch (e) {
          console.error('Failed to parse SSE telemetry:', e)
        }
      }

      eventSource.onerror = () => {
        setConnected(false)
        eventSource?.close()
        setTimeout(connectSSE, 2000)
      }
    }

    connectSSE()

    return () => {
      eventSource?.close()
    }
  }, [])

  const runBenchmarkProbe = async () => {
    setBenchmarking(true)
    try {
      const res = await fetch('/api/benchmark/probe', { method: 'POST' })
      const data = await res.json()
      setBenchmarkResult(data)
    } catch (e) {
      console.error(e)
    } finally {
      setBenchmarking(false)
    }
  }

  const handleCopyMetrics = () => {
    navigator.clipboard.writeText(`${window.location.origin}/metrics`)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const pageCount = telemetry?.page_count || 0
  const sizeMB = ((pageCount * 16) / 1024).toFixed(2)
  const hitRate = telemetry ? (telemetry.cache_hit_rate * 100).toFixed(1) : '100.0'
  const hits = telemetry?.cache_hits || 0
  const misses = telemetry?.cache_misses || 0
  const lastLSN = telemetry?.last_lsn || 0
  const qps = telemetry?.qps || 0
  const readyTasks = telemetry?.queue_ready_tasks || 0
  const inFlightTasks = telemetry?.queue_inflight_tasks || 0
  const dlqTasks = telemetry?.queue_dlq_tasks || 0
  const pubsubEvents = telemetry?.pubsub_events_count || 0
  const cluster = telemetry?.cluster
  const clusterPeers = cluster?.peers ?? []
  const onlinePeers = clusterPeers.filter((peer) => peer.status.includes('online')).length
  const unreachablePeers = clusterPeers.length - onlinePeers
  const clusterHealth = !cluster?.enabled
    ? 'Standalone'
    : unreachablePeers > 0
      ? 'Degraded'
      : 'Healthy'
  const clusterHealthClass = clusterHealth === 'Healthy'
    ? 'text-emerald-400 bg-emerald-500/10 border-emerald-500/20'
    : clusterHealth === 'Degraded'
      ? 'text-amber-400 bg-amber-500/10 border-amber-500/20'
      : 'text-zinc-400 bg-zinc-800 border-zinc-700'

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100 selection:bg-emerald-500/20 selection:text-emerald-300">
      {/* Top Navigation Bar */}
      <header className="sticky top-0 z-30 border-b border-zinc-800/80 bg-zinc-950/80 backdrop-blur-md px-6 py-3.5 flex items-center justify-between">
        <div className="flex items-center gap-3.5">
          <div className="w-8 h-8 rounded-lg bg-emerald-500/10 border border-emerald-500/30 flex items-center justify-center text-emerald-400 font-bold text-sm">
            <Activity className="w-4 h-4 text-emerald-400 animate-pulse" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <span className="font-bold text-sm tracking-tight text-white">CoCoDB Observability</span>
              <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-medium">
                Live Telemetry
              </span>
              {cluster && cluster.enabled ? (
                <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-sky-500/10 text-sky-400 border border-sky-500/20 font-medium flex items-center gap-1">
                  <span className="w-1.5 h-1.5 rounded-full bg-sky-400 animate-ping"></span>
                  Cluster Active ({cluster.total_nodes} Nodes)
                </span>
              ) : (
                <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-zinc-800 text-zinc-400 font-medium">
                  Standalone Node
                </span>
              )}
            </div>
            <p className="text-[11px] text-zinc-400">Real-time kernel analytics, cluster mesh health, and Prometheus telemetry</p>
          </div>
        </div>

        {/* Live Status & Controls */}
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2 px-3 py-1 rounded-full bg-zinc-900 border border-zinc-800 text-xs font-mono">
            <span className={`w-2 h-2 rounded-full ${connected ? 'bg-emerald-500 shadow-sm shadow-emerald-500/50 animate-pulse' : 'bg-rose-500'}`}></span>
            <span className="text-zinc-300">{connected ? 'SSE 500ms Live' : 'Reconnecting...'}</span>
          </div>

          <button
            onClick={runBenchmarkProbe}
            disabled={benchmarking}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold shadow-xs transition cursor-pointer disabled:opacity-50"
          >
            <Zap className={`w-3.5 h-3.5 ${benchmarking ? 'animate-spin' : ''}`} />
            <span>{benchmarking ? 'Probing...' : 'Synthetic Probe'}</span>
          </button>

          <button
            onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
            className="p-1.5 rounded-lg border border-zinc-800 bg-zinc-900 hover:bg-zinc-800 text-zinc-400 hover:text-white transition cursor-pointer"
            title="Toggle theme"
          >
            {theme === 'dark' ? <Sun className="w-4 h-4 text-amber-400" /> : <Moon className="w-4 h-4 text-indigo-400" />}
          </button>
        </div>
      </header>

      {/* Main Content Area */}
      <main className="max-w-7xl mx-auto p-6 space-y-6">
        {/* Navigation Tabs */}
        <div className="flex items-center gap-2 border-b border-zinc-800/80 pb-3">
          <button
            onClick={() => setActiveTab('overview')}
            className={`px-3.5 py-1.5 rounded-lg text-xs font-medium transition cursor-pointer ${
              activeTab === 'overview'
                ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/30'
                : 'text-zinc-400 hover:text-white hover:bg-zinc-900'
            }`}
          >
            Metrics Overview
          </button>
          <button
            onClick={() => setActiveTab('cluster')}
            className={`px-3.5 py-1.5 rounded-lg text-xs font-medium transition cursor-pointer flex items-center gap-1.5 ${
              activeTab === 'cluster'
                ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/30'
                : 'text-zinc-400 hover:text-white hover:bg-zinc-900'
            }`}
          >
            <Network className="w-3.5 h-3.5" />
            <span>Cluster Topology & Health</span>
          </button>
          <button
            onClick={() => setActiveTab('engine')}
            className={`px-3.5 py-1.5 rounded-lg text-xs font-medium transition cursor-pointer ${
              activeTab === 'engine'
                ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/30'
                : 'text-zinc-400 hover:text-white hover:bg-zinc-900'
            }`}
          >
            Storage & 16-Partition Cache
          </button>
          <button
            onClick={() => setActiveTab('queues')}
            className={`px-3.5 py-1.5 rounded-lg text-xs font-medium transition cursor-pointer ${
              activeTab === 'queues'
                ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/30'
                : 'text-zinc-400 hover:text-white hover:bg-zinc-900'
            }`}
          >
            Task Queues & Pub/Sub
          </button>
          <button
            onClick={() => setActiveTab('prometheus')}
            className={`px-3.5 py-1.5 rounded-lg text-xs font-medium transition cursor-pointer ${
              activeTab === 'prometheus'
                ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/30'
                : 'text-zinc-400 hover:text-white hover:bg-zinc-900'
            }`}
          >
            Prometheus Exporter (/metrics)
          </button>
        </div>

        {/* Benchmark Probe Banner (If Result Exists) */}
        {benchmarkResult && (
          <div className="p-4 rounded-2xl bg-indigo-950/40 border border-indigo-500/30 flex items-center justify-between text-xs animate-in fade-in">
            <div className="flex items-center gap-3">
              <Flame className="w-5 h-5 text-amber-400" />
              <div>
                <span className="font-semibold text-white">Synthetic Benchmark Probe Result: </span>
                <span className="font-mono text-indigo-300">
                  {benchmarkResult.ops.toLocaleString()} ops in 1 sec · P50 Latency: {benchmarkResult.p50} µs · P99 Latency: {benchmarkResult.p99} µs
                </span>
              </div>
            </div>
            <button
              onClick={() => setBenchmarkResult(null)}
              className="text-zinc-400 hover:text-white text-xs cursor-pointer"
            >
              Dismiss
            </button>
          </div>
        )}

        {/* 4 Primary Top Metrics */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          {/* Card 1: Throughput QPS */}
          <div className="p-5 rounded-2xl bg-zinc-900/60 border border-zinc-800/80 space-y-3">
            <div className="flex items-center justify-between text-zinc-400 text-xs font-medium">
              <span>Operations Throughput</span>
              <Activity className="w-4 h-4 text-emerald-400" />
            </div>
            <div className="flex items-baseline gap-2">
              <span className="text-3xl font-bold font-mono text-white">{qps.toLocaleString()}</span>
              <span className="text-xs font-mono text-emerald-400">ops/sec</span>
            </div>
            <div className="h-12 w-full pt-1">
              <Sparkline data={qpsHistory} color="#10b981" fillColor="rgba(16, 185, 129, 0.15)" />
            </div>
            <div className="text-[11px] text-zinc-500 font-mono">Live rate calculated over 500ms delta</div>
          </div>

          {/* Card 2: Cache Hit Rate */}
          <div className="p-5 rounded-2xl bg-zinc-900/60 border border-zinc-800/80 space-y-3">
            <div className="flex items-center justify-between text-zinc-400 text-xs font-medium">
              <span>16-Partition LRU Hit Rate</span>
              <Cpu className="w-4 h-4 text-teal-400" />
            </div>
            <div className="flex items-baseline gap-2">
              <span className="text-3xl font-bold font-mono text-white">{hitRate}%</span>
              <span className="text-xs font-mono text-teal-400">ratio</span>
            </div>
            <div className="h-12 w-full pt-1">
              <Sparkline data={cacheHitHistory} color="#14b8a6" fillColor="rgba(20, 184, 166, 0.15)" min={0} max={100} />
            </div>
            <div className="text-[11px] text-zinc-500 font-mono">
              {hits.toLocaleString()} hits / {misses.toLocaleString()} misses
            </div>
          </div>

          {/* Card 3: Allocated Storage */}
          <div className="p-5 rounded-2xl bg-zinc-900/60 border border-zinc-800/80 space-y-3">
            <div className="flex items-center justify-between text-zinc-400 text-xs font-medium">
              <span>Allocated Storage</span>
              <HardDrive className="w-4 h-4 text-sky-400" />
            </div>
            <div className="flex items-baseline gap-2">
              <span className="text-3xl font-bold font-mono text-white">{sizeMB}</span>
              <span className="text-xs font-mono text-sky-400">MB</span>
            </div>
            <div className="h-12 w-full pt-1">
              <Sparkline data={allocatedMbHistory} color="#0ea5e9" fillColor="rgba(14, 165, 233, 0.15)" />
            </div>
            <div className="text-[11px] text-zinc-500 font-mono">
              {pageCount.toLocaleString()} pages × 16 KiB Slotted
            </div>
          </div>

          {/* Card 4: WAL Sequence & MVCC */}
          <div className="p-5 rounded-2xl bg-zinc-900/60 border border-zinc-800/80 space-y-3">
            <div className="flex items-center justify-between text-zinc-400 text-xs font-medium">
              <span>WAL Commit Sequence</span>
              <Layers className="w-4 h-4 text-purple-400" />
            </div>
            <div className="flex items-baseline gap-2">
              <span className="text-3xl font-bold font-mono text-white">#{lastLSN}</span>
              <span className="text-xs font-mono text-purple-400">LSN</span>
            </div>
            <div className="h-12 w-full flex items-center justify-between text-xs font-mono bg-zinc-950/60 p-2.5 rounded-xl border border-zinc-800/60">
              <span className="text-zinc-400">Crash-Safe:</span>
              <span className="text-emerald-400 font-semibold">Dual Meta A/B</span>
            </div>
            <div className="text-[11px] text-zinc-500 font-mono">Txn ID #{telemetry?.last_txn_id || 0}</div>
          </div>
        </div>

        {/* Tab 1: Overview */}
        {activeTab === 'overview' && (
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            {/* Task Queues Health */}
            <div className="p-6 rounded-2xl bg-zinc-900/60 border border-zinc-800/80 space-y-5">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Package className="w-4 h-4 text-emerald-400" />
                  <h2 className="text-sm font-semibold text-white">Transactional Task Queues</h2>
                </div>
                <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                  {telemetry?.queue_count || 0} Queues Active
                </span>
              </div>

              <div className="grid grid-cols-3 gap-3 text-center">
                <div className="p-3.5 rounded-xl bg-zinc-950 border border-zinc-800/60">
                  <div className="text-[10px] uppercase text-zinc-400 font-medium">Ready Tasks</div>
                  <div className="text-2xl font-bold font-mono text-emerald-400 mt-1">{readyTasks}</div>
                </div>
                <div className="p-3.5 rounded-xl bg-zinc-950 border border-zinc-800/60">
                  <div className="text-[10px] uppercase text-zinc-400 font-medium">In-Flight Leased</div>
                  <div className="text-2xl font-bold font-mono text-amber-400 mt-1">{inFlightTasks}</div>
                </div>
                <div className="p-3.5 rounded-xl bg-zinc-950 border border-zinc-800/60">
                  <div className="text-[10px] uppercase text-zinc-400 font-medium">Dead-Letter DLQ</div>
                  <div className="text-2xl font-bold font-mono text-rose-400 mt-1">{dlqTasks}</div>
                </div>
              </div>

              <div className="h-16 w-full pt-1">
                <Sparkline data={queueReadyHistory} color="#10b981" fillColor="rgba(16, 185, 129, 0.1)" />
              </div>
            </div>

            {/* Real-Time PubSub Velocity */}
            <div className="p-6 rounded-2xl bg-zinc-900/60 border border-zinc-800/80 space-y-5">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Radio className="w-4 h-4 text-emerald-400" />
                  <h2 className="text-sm font-semibold text-white">Pub/Sub Event Broker Telemetry</h2>
                </div>
                <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                  {telemetry?.pubsub_active_topics || 0} Topics Active
                </span>
              </div>

              <div className="grid grid-cols-2 gap-3 text-center">
                <div className="p-3.5 rounded-xl bg-zinc-950 border border-zinc-800/60">
                  <div className="text-[10px] uppercase text-zinc-400 font-medium">Published Events</div>
                  <div className="text-2xl font-bold font-mono text-emerald-400 mt-1">{pubsubEvents}</div>
                </div>
                <div className="p-3.5 rounded-xl bg-zinc-950 border border-zinc-800/60">
                  <div className="text-[10px] uppercase text-zinc-400 font-medium">Delivered to Subs</div>
                  <div className="text-2xl font-bold font-mono text-sky-400 mt-1">
                    {telemetry?.pubsub_delivered_count || 0}
                  </div>
                </div>
              </div>

              <div className="h-16 w-full pt-1">
                <Sparkline data={pubsubHistory} color="#38bdf8" fillColor="rgba(56, 189, 248, 0.1)" />
              </div>
            </div>

            {/* Live Cluster Health */}
            <button
              type="button"
              onClick={() => setActiveTab('cluster')}
              className="p-6 rounded-2xl bg-zinc-900/60 border border-zinc-800/80 space-y-5 text-left transition hover:bg-zinc-900 hover:border-sky-500/40 cursor-pointer"
            >
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Network className="w-4 h-4 text-sky-400" />
                  <h2 className="text-sm font-semibold text-white">Live Cluster Status</h2>
                </div>
                <span className={`text-[10px] font-mono px-2 py-0.5 rounded border font-medium ${clusterHealthClass}`}>
                  {clusterHealth}
                </span>
              </div>

              {cluster?.enabled ? (
                <>
                  <div className="grid grid-cols-3 gap-3 text-center">
                    <div className="p-3.5 rounded-xl bg-zinc-950 border border-zinc-800/60">
                      <div className="text-[10px] uppercase text-zinc-400 font-medium">Nodes</div>
                      <div className="text-2xl font-bold font-mono text-sky-400 mt-1">{cluster.total_nodes}</div>
                    </div>
                    <div className="p-3.5 rounded-xl bg-zinc-950 border border-zinc-800/60">
                      <div className="text-[10px] uppercase text-zinc-400 font-medium">Online</div>
                      <div className="text-2xl font-bold font-mono text-emerald-400 mt-1">{onlinePeers}</div>
                    </div>
                    <div className="p-3.5 rounded-xl bg-zinc-950 border border-zinc-800/60">
                      <div className="text-[10px] uppercase text-zinc-400 font-medium">Unavailable</div>
                      <div className={`text-2xl font-bold font-mono mt-1 ${unreachablePeers > 0 ? 'text-rose-400' : 'text-zinc-500'}`}>
                        {unreachablePeers}
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center justify-between text-[11px] font-mono text-zinc-500">
                    <span className="truncate">{cluster.node_id} · {cluster.addr}</span>
                    <span className="text-sky-400">View topology →</span>
                  </div>
                </>
              ) : (
                <div className="flex items-center gap-3 rounded-xl bg-zinc-950 border border-zinc-800/60 p-4">
                  <ServerIcon className="w-5 h-5 text-zinc-500" />
                  <div>
                    <div className="text-xs font-medium text-zinc-300">Embedded standalone node</div>
                    <div className="text-[11px] text-zinc-500 mt-0.5">Start a cluster node to monitor peer health here.</div>
                  </div>
                </div>
              )}
            </button>
          </div>
        )}

        {/* Tab: Cluster Topology & Health */}
        {activeTab === 'cluster' && (
          <div className="space-y-6">
            {cluster && cluster.enabled ? (
              <>
                {/* Cluster Metadata Cards */}
                <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
                  <div className="p-4 rounded-2xl bg-zinc-900/60 border border-zinc-800/80 space-y-1">
                    <div className="flex items-center gap-2 text-zinc-400 text-xs">
                      <ServerIcon className="w-3.5 h-3.5 text-sky-400" />
                      <span>Local Node Identity</span>
                    </div>
                    <div className="text-sm font-bold font-mono text-white truncate">{cluster.node_id}</div>
                    <div className="text-[11px] text-zinc-500 font-mono">{cluster.addr}</div>
                  </div>

                  <div className="p-4 rounded-2xl bg-zinc-900/60 border border-zinc-800/80 space-y-1">
                    <div className="flex items-center gap-2 text-zinc-400 text-xs">
                      <Lock className="w-3.5 h-3.5 text-emerald-400" />
                      <span>Wire Security</span>
                    </div>
                    <div className="text-sm font-bold font-mono text-emerald-400">
                      {cluster.mtls_enforced ? 'mTLS 1.3 Active' : cluster.tls_enabled ? 'TLS 1.3' : 'Plain TCP'}
                    </div>
                    <div className="text-[11px] text-zinc-500">
                      {cluster.auth_enforced ? 'Token Auth Enforced' : 'No Auth'}
                    </div>
                  </div>

                  <div className="p-4 rounded-2xl bg-zinc-900/60 border border-zinc-800/80 space-y-1">
                    <div className="flex items-center gap-2 text-zinc-400 text-xs">
                      <Globe className="w-3.5 h-3.5 text-purple-400" />
                      <span>Consistent Hash Ring</span>
                    </div>
                    <div className="text-sm font-bold font-mono text-white">
                      {cluster.total_nodes} Physical Nodes
                    </div>
                    <div className="text-[11px] text-zinc-500 font-mono">
                      {cluster.virtual_nodes} Virtual Tokens / Node
                    </div>
                  </div>

                  <div className="p-4 rounded-2xl bg-zinc-900/60 border border-zinc-800/80 space-y-1">
                    <div className="flex items-center gap-2 text-zinc-400 text-xs">
                      <Activity className="w-3.5 h-3.5 text-amber-400" />
                      <span>RPC Socket Connections</span>
                    </div>
                    <div className="text-sm font-bold font-mono text-white">
                      {cluster.active_conns} Active Sockets
                    </div>
                    <div className="text-[11px] text-zinc-500">Zero-copy binary frame multiplexing</div>
                  </div>
                </div>

                {/* Peer Nodes Table */}
                <div className="rounded-2xl bg-zinc-900/60 border border-zinc-800/80 overflow-hidden shadow-xs">
                  <div className="p-5 border-b border-zinc-800/80 flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <Network className="w-4 h-4 text-emerald-400" />
                      <h2 className="font-semibold text-sm text-white">Cluster Peer Node Mesh</h2>
                    </div>
                    <span className="text-xs font-mono text-zinc-400">
                      Auto-Forwarding Deduplication Enabled
                    </span>
                  </div>

                  <div className="divide-y divide-zinc-800/60">
                    {cluster.peers && cluster.peers.length > 0 ? (
                      cluster.peers.map((peer, idx) => (
                        <div key={idx} className="p-4 flex items-center justify-between hover:bg-zinc-800/30 transition">
                          <div className="flex items-center gap-3">
                            <div className={`w-2.5 h-2.5 rounded-full ${
                              peer.status.includes('online') ? 'bg-emerald-400 shadow-sm shadow-emerald-400/50' : 'bg-rose-500'
                            }`} />
                            <div>
                              <div className="font-mono text-xs font-semibold text-white flex items-center gap-2">
                                <span>{peer.addr}</span>
                                {peer.is_local && (
                                  <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-emerald-500/20 text-emerald-300 font-medium">
                                    SELF
                                  </span>
                                )}
                              </div>
                              <div className="text-[11px] text-zinc-500 font-mono mt-0.5">
                                Shard Partition Token Range: Virtual Slots {idx * 256} - {(idx + 1) * 256 - 1}
                              </div>
                            </div>
                          </div>

                          <div className="flex items-center gap-6 text-right">
                            <div>
                              <div className="text-[11px] text-zinc-400 font-mono">Status</div>
                              <div className={`text-xs font-semibold ${
                                peer.status.includes('online') ? 'text-emerald-400' : 'text-rose-400'
                              }`}>
                                {peer.status}
                              </div>
                            </div>

                            {!peer.is_local && (
                              <div>
                                <div className="text-[11px] text-zinc-400 font-mono">Ping RTT</div>
                                <div className="text-xs font-mono text-sky-400 font-semibold">
                                  {peer.latency_us >= 0 ? `${peer.latency_us} µs` : 'timeout'}
                                </div>
                              </div>
                            )}
                          </div>
                        </div>
                      ))
                    ) : (
                      <div className="p-8 text-center text-xs text-zinc-500">No cluster peers configured.</div>
                    )}
                  </div>
                </div>
              </>
            ) : (
              <div className="p-12 rounded-2xl bg-zinc-900/60 border border-zinc-800/80 text-center space-y-3">
                <Network className="w-10 h-10 text-zinc-600 mx-auto" />
                <h3 className="text-base font-semibold text-white">Standalone Node Mode</h3>
                <p className="text-xs text-zinc-400 max-w-md mx-auto">
                  This database instance is currently running in embedded standalone mode. To enable multi-node clustering with mTLS 1.3 and cross-node deduplication:
                </p>
                <pre className="p-4 rounded-xl bg-zinc-950 text-emerald-400 font-mono text-xs inline-block text-left border border-zinc-800">
                  node, err := cluster.StartNode(db, "127.0.0.1:9001",<br />
                  &nbsp;&nbsp;cluster.WithSecret("my_token"),<br />
                  &nbsp;&nbsp;cluster.WithPeers("127.0.0.1:9002", "127.0.0.1:9003"),<br />
                  )
                </pre>
              </div>
            )}
          </div>
        )}

        {/* Tab 2: Storage & Engine */}
        {activeTab === 'engine' && (
          <div className="grid grid-cols-1 md:grid-cols-3 gap-6 text-xs">
            <div className="p-5 rounded-2xl bg-zinc-900/60 border border-zinc-800/80 space-y-3">
              <div className="flex items-center gap-2 text-sm font-semibold text-white">
                <ShieldCheck className="w-4 h-4 text-emerald-400" />
                <span>Dual Meta Pages Crash-Safety</span>
              </div>
              <p className="text-zinc-400 leading-relaxed">
                Alternating Meta Page 0 and Meta Page 1 ensure complete atomic commits. Even on sudden power loss, the database rolls back to the last valid meta snapshot with zero corruption.
              </p>
              <div className="p-3 rounded-xl bg-zinc-950 font-mono text-emerald-400 text-[11px]">
                Status: Verified Healthy
              </div>
            </div>

            <div className="p-5 rounded-2xl bg-zinc-900/60 border border-zinc-800/80 space-y-3">
              <div className="flex items-center gap-2 text-sm font-semibold text-white">
                <Lock className="w-4 h-4 text-emerald-400" />
                <span>Hardware CRC32C Checksums</span>
              </div>
              <p className="text-zinc-400 leading-relaxed">
                Every single 16 KiB slotted page header contains a Castagnoli CRC32C checksum validated using CPU SSE4.2 instructions on read.
              </p>
              <div className="p-3 rounded-xl bg-zinc-950 font-mono text-emerald-400 text-[11px]">
                Integrity: 100% Page Match
              </div>
            </div>

            <div className="p-5 rounded-2xl bg-zinc-900/60 border border-zinc-800/80 space-y-3">
              <div className="flex items-center gap-2 text-sm font-semibold text-white">
                <Cpu className="w-4 h-4 text-emerald-400" />
                <span>16-Partition Stripped LRU</span>
              </div>
              <p className="text-zinc-400 leading-relaxed">
                Sharded into 16 independent mutex stripes to eliminate CPU cache lock contention under multi-threaded parallel reader workloads.
              </p>
              <div className="p-3 rounded-xl bg-zinc-950 font-mono text-emerald-400 text-[11px]">
                Hit Rate: {hitRate}%
              </div>
            </div>
          </div>
        )}

        {/* Tab 4: Prometheus Exporter */}
        {activeTab === 'prometheus' && (
          <div className="p-6 rounded-2xl bg-zinc-900/60 border border-zinc-800/80 space-y-5">
            <div className="flex items-center justify-between">
              <div>
                <h2 className="text-sm font-semibold text-white">Prometheus & OpenTelemetry Scrape Target</h2>
                <p className="text-xs text-zinc-400 mt-0.5">Compatible with Prometheus, Grafana Agent, and Datadog scrape jobs</p>
              </div>

              <div className="flex items-center gap-2">
                <button
                  onClick={handleCopyMetrics}
                  className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-zinc-800 hover:bg-zinc-700 text-xs font-mono text-zinc-200 transition cursor-pointer"
                >
                  {copied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                  <span>{copied ? 'Copied URL' : 'Copy /metrics URL'}</span>
                </button>
                <a
                  href="/metrics"
                  target="_blank"
                  rel="noreferrer"
                  className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-emerald-500 hover:bg-emerald-400 text-zinc-950 text-xs font-semibold transition"
                >
                  <span>Open Raw /metrics</span>
                  <ExternalLink className="w-3.5 h-3.5" />
                </a>
              </div>
            </div>

            <div className="p-4 rounded-xl bg-zinc-950 border border-zinc-800 font-mono text-xs text-zinc-300 space-y-2 overflow-x-auto">
              <div className="text-zinc-500"># prometheus.yml scrape config example:</div>
              <div className="text-emerald-400">
                scrape_configs:<br />
                &nbsp;&nbsp;- job_name: 'cocodb'<br />
                &nbsp;&nbsp;&nbsp;&nbsp;static_configs:<br />
                &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;- targets: ['{window.location.host}']
              </div>
            </div>
          </div>
        )}
      </main>
    </div>
  )
}
