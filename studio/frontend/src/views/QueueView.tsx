import React, { useState, useEffect } from 'react'
import {
  Package,
  Plus,
  Play,
  CheckCircle2,
  AlertTriangle,
  RefreshCw,
  Clock,
  ShieldCheck,
  Zap,
  ArrowRight,
  Layers,
} from 'lucide-react'
import { api } from '../api'
import type { QueueItem } from '../types'

interface QueueViewProps {
  initialQueue?: string
}

export const QueueView: React.FC<QueueViewProps> = ({ initialQueue }) => {
  const [queues, setQueues] = useState<QueueItem[]>([])
  const [selectedQueue, setSelectedQueue] = useState<string>(initialQueue || 'default_tasks')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  // Enqueue state
  const [payload, setPayload] = useState('{\n  "order_id": "ORD-9988",\n  "action": "charge_customer",\n  "amount": 250.00\n}')
  const [dedupId, setDedupId] = useState('order_ORD-9988')
  const [priority, setPriority] = useState(128)
  const [delayMs, setDelayMs] = useState(0)

  // Dequeued task state
  const [lastDequeued, setLastDequeued] = useState<{
    found: boolean
    message_id?: string
    queue?: string
    payload?: string
    retry_count?: number
    priority?: number
    state?: string
  } | null>(null)
  const [autoAck, setAutoAck] = useState(true)

  const loadQueues = async () => {
    setLoading(true)
    try {
      const data = await api.listQueues()
      setQueues(data.queues || [])
      if (data.queues && data.queues.length > 0 && !selectedQueue) {
        setSelectedQueue(data.queues[0].name)
      }
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadQueues()
    const timer = setInterval(loadQueues, 3000)
    return () => clearInterval(timer)
  }, [])

  const currentQueueInfo = queues.find((q) => q.name === selectedQueue)

  const handleEnqueue = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    setSuccess(null)
    try {
      const res = await api.enqueueTask(selectedQueue, payload, dedupId || undefined, priority, delayMs)
      setSuccess(`Task enqueued successfully! Message ID: ${res.message_id}`)
      loadQueues()
    } catch (err: any) {
      setError(err.message)
    }
  }

  const handleDequeue = async () => {
    setError(null)
    setSuccess(null)
    try {
      const res = await api.dequeueTask(selectedQueue, autoAck)
      setLastDequeued(res)
      if (res.found) {
        setSuccess(`Task dequeued: ${res.message_id} (${autoAck ? 'Auto-Acked' : 'Leased'})`)
      } else {
        setSuccess('Queue is currently empty.')
      }
      loadQueues()
    } catch (err: any) {
      setError(err.message)
    }
  }

  return (
    <div className="p-8 space-y-8 max-w-7xl mx-auto">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-zinc-900 dark:text-white flex items-center gap-2.5">
            <Package className="w-6 h-6 text-emerald-500" />
            <span>Transactional Task Queues</span>
            <span className="text-xs font-mono px-2 py-0.5 rounded-md bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border border-emerald-500/20 font-medium">
              Exactly-Once Dedup & DLQ
            </span>
          </h1>
          <p className="text-xs text-zinc-500 dark:text-zinc-400 mt-1">
            Durable task scheduling, visibility timeout leases, and distributed deduplication engine
          </p>
        </div>

        <div className="flex items-center gap-3">
          <button
            onClick={loadQueues}
            disabled={loading}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300 text-xs font-medium hover:bg-zinc-50 dark:hover:bg-zinc-800 transition cursor-pointer"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
            <span>Refresh</span>
          </button>
        </div>
      </div>

      {/* Notifications */}
      {error && (
        <div className="p-4 rounded-xl bg-rose-500/10 border border-rose-500/20 text-rose-700 dark:text-rose-400 text-xs flex items-center gap-2.5">
          <AlertTriangle className="w-4 h-4 shrink-0" />
          <span>{error}</span>
        </div>
      )}
      {success && (
        <div className="p-4 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-700 dark:text-emerald-400 text-xs flex items-center gap-2.5">
          <CheckCircle2 className="w-4 h-4 shrink-0" />
          <span>{success}</span>
        </div>
      )}

      {/* Queue Selection & Metrics Bar */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <div className="p-5 rounded-2xl border border-zinc-200/80 dark:border-zinc-800/80 bg-white/70 dark:bg-zinc-900/50 backdrop-blur-md space-y-2">
          <label className="text-[11px] font-semibold tracking-wider uppercase text-zinc-400 dark:text-zinc-500">
            Select / Active Queue
          </label>
          <div className="flex gap-2">
            <input
              type="text"
              value={selectedQueue}
              onChange={(e) => setSelectedQueue(e.target.value)}
              placeholder="e.g. order_processing"
              className="w-full px-3 py-1.5 rounded-lg bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 text-xs text-zinc-900 dark:text-white font-mono focus:outline-none focus:border-emerald-500"
            />
          </div>
        </div>

        <div className="p-5 rounded-2xl border border-zinc-200/80 dark:border-zinc-800/80 bg-white/70 dark:bg-zinc-900/50 backdrop-blur-md space-y-1">
          <div className="flex items-center justify-between text-xs text-zinc-500">
            <span>Ready Tasks</span>
            <Play className="w-3.5 h-3.5 text-emerald-500" />
          </div>
          <div className="text-2xl font-bold font-mono text-zinc-900 dark:text-white">
            {currentQueueInfo?.stats?.ReadyCount ?? 0}
          </div>
          <p className="text-[10px] text-zinc-400">Available for workers</p>
        </div>

        <div className="p-5 rounded-2xl border border-zinc-200/80 dark:border-zinc-800/80 bg-white/70 dark:bg-zinc-900/50 backdrop-blur-md space-y-1">
          <div className="flex items-center justify-between text-xs text-zinc-500">
            <span>In-Flight (Leased)</span>
            <Clock className="w-3.5 h-3.5 text-amber-500" />
          </div>
          <div className="text-2xl font-bold font-mono text-amber-600 dark:text-amber-400">
            {currentQueueInfo?.stats?.InFlightCount ?? 0}
          </div>
          <p className="text-[10px] text-zinc-400">Under visibility lease</p>
        </div>

        <div className="p-5 rounded-2xl border border-zinc-200/80 dark:border-zinc-800/80 bg-white/70 dark:bg-zinc-900/50 backdrop-blur-md space-y-1">
          <div className="flex items-center justify-between text-xs text-zinc-500">
            <span>Dead-Letter (DLQ)</span>
            <AlertTriangle className="w-3.5 h-3.5 text-rose-500" />
          </div>
          <div className="text-2xl font-bold font-mono text-rose-600 dark:text-rose-400">
            {currentQueueInfo?.stats?.DLQCount ?? 0}
          </div>
          <p className="text-[10px] text-zinc-400">Exceeded max retries</p>
        </div>
      </div>

      {/* Main Interactive Panels: Enqueue vs Dequeue */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
        {/* Enqueue Task Panel */}
        <div className="p-6 rounded-2xl border border-zinc-200/80 dark:border-zinc-800/80 bg-white/70 dark:bg-zinc-900/50 backdrop-blur-md space-y-5">
          <div className="flex items-center justify-between">
            <h2 className="text-base font-semibold text-zinc-900 dark:text-white flex items-center gap-2">
              <Plus className="w-4 h-4 text-emerald-500" />
              <span>Enqueue Task</span>
            </h2>
            <span className="text-[11px] font-mono text-zinc-400">Durable B+Tree Backed</span>
          </div>

          <form onSubmit={handleEnqueue} className="space-y-4">
            <div>
              <label className="block text-xs font-medium text-zinc-700 dark:text-zinc-300 mb-1">
                Payload (JSON or Text)
              </label>
              <textarea
                rows={4}
                value={payload}
                onChange={(e) => setPayload(e.target.value)}
                className="w-full p-3 rounded-xl bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 text-xs font-mono text-zinc-900 dark:text-white focus:outline-none focus:border-emerald-500"
              />
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <div>
                <label className="block text-[11px] font-medium text-zinc-700 dark:text-zinc-300 mb-1">
                  Dedup ID (Idempotency)
                </label>
                <input
                  type="text"
                  value={dedupId}
                  onChange={(e) => setDedupId(e.target.value)}
                  placeholder="e.g. order_9988"
                  className="w-full px-3 py-1.5 rounded-lg bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 text-xs font-mono text-zinc-900 dark:text-white focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div>
                <label className="block text-[11px] font-medium text-zinc-700 dark:text-zinc-300 mb-1">
                  Priority (0 - 255)
                </label>
                <input
                  type="number"
                  min="0"
                  max="255"
                  value={priority}
                  onChange={(e) => setPriority(parseInt(e.target.value) || 128)}
                  className="w-full px-3 py-1.5 rounded-lg bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 text-xs font-mono text-zinc-900 dark:text-white focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div>
                <label className="block text-[11px] font-medium text-zinc-700 dark:text-zinc-300 mb-1">
                  Delay (ms)
                </label>
                <input
                  type="number"
                  min="0"
                  step="100"
                  value={delayMs}
                  onChange={(e) => setDelayMs(parseInt(e.target.value) || 0)}
                  className="w-full px-3 py-1.5 rounded-lg bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 text-xs font-mono text-zinc-900 dark:text-white focus:outline-none focus:border-emerald-500"
                />
              </div>
            </div>

            <button
              type="submit"
              className="w-full py-2.5 rounded-xl bg-emerald-500 hover:bg-emerald-400 text-zinc-950 font-semibold text-xs shadow-md shadow-emerald-500/20 transition cursor-pointer flex items-center justify-center gap-2"
            >
              <Zap className="w-4 h-4 stroke-[2.5]" />
              <span>Enqueue Task to "{selectedQueue}"</span>
            </button>
          </form>
        </div>

        {/* Worker Simulator Panel */}
        <div className="p-6 rounded-2xl border border-zinc-200/80 dark:border-zinc-800/80 bg-white/70 dark:bg-zinc-900/50 backdrop-blur-md space-y-5">
          <div className="flex items-center justify-between">
            <h2 className="text-base font-semibold text-zinc-900 dark:text-white flex items-center gap-2">
              <ShieldCheck className="w-4 h-4 text-emerald-500" />
              <span>Worker Simulator & Dequeue</span>
            </h2>
            <div className="flex items-center gap-2 text-xs">
              <label className="flex items-center gap-1.5 text-zinc-600 dark:text-zinc-400 cursor-pointer">
                <input
                  type="checkbox"
                  checked={autoAck}
                  onChange={(e) => setAutoAck(e.target.checked)}
                  className="rounded border-zinc-300 text-emerald-500 focus:ring-emerald-500"
                />
                <span>Auto-Ack</span>
              </label>
            </div>
          </div>

          <div className="p-4 rounded-xl bg-zinc-100/80 dark:bg-zinc-950 border border-zinc-200/60 dark:border-zinc-800/60 min-h-[160px] flex flex-col justify-between">
            {lastDequeued?.found ? (
              <div className="space-y-2">
                <div className="flex items-center justify-between text-xs">
                  <span className="font-mono text-emerald-600 dark:text-emerald-400 font-semibold">
                    ID: {lastDequeued.message_id}
                  </span>
                  <span className="px-2 py-0.5 rounded-full text-[10px] bg-emerald-500/20 text-emerald-700 dark:text-emerald-300 font-mono">
                    Priority: {lastDequeued.priority} | Retries: {lastDequeued.retry_count}
                  </span>
                </div>
                <pre className="p-2.5 rounded-lg bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 text-xs font-mono text-zinc-800 dark:text-zinc-200 overflow-x-auto">
                  {lastDequeued.payload}
                </pre>
              </div>
            ) : (
              <div className="flex flex-col items-center justify-center py-8 text-zinc-400 space-y-1">
                <Package className="w-8 h-8 stroke-1 text-zinc-300 dark:text-zinc-600" />
                <p className="text-xs">No task dequeued yet</p>
              </div>
            )}

            <button
              onClick={handleDequeue}
              className="mt-4 w-full py-2.5 rounded-xl bg-zinc-900 hover:bg-zinc-800 text-white dark:bg-white dark:hover:bg-zinc-200 dark:text-zinc-950 font-semibold text-xs transition cursor-pointer flex items-center justify-center gap-2"
            >
              <Play className="w-3.5 h-3.5 fill-current" />
              <span>Simulate Worker Dequeue</span>
            </button>
          </div>
        </div>
      </div>

      {/* Catalog of Queues */}
      <div className="p-6 rounded-2xl border border-zinc-200/80 dark:border-zinc-800/80 bg-white/70 dark:bg-zinc-900/50 backdrop-blur-md space-y-4">
        <h3 className="text-sm font-semibold text-zinc-900 dark:text-white flex items-center gap-2">
          <Layers className="w-4 h-4 text-emerald-500" />
          <span>Registered Persistent Queues ({queues.length})</span>
        </h3>

        {queues.length === 0 ? (
          <p className="text-xs text-zinc-500 py-4">No active queues recorded in storage catalog yet.</p>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-4">
            {queues.map((q) => (
              <div
                key={q.name}
                onClick={() => setSelectedQueue(q.name)}
                className={`p-4 rounded-xl border transition cursor-pointer ${
                  selectedQueue === q.name
                    ? 'border-emerald-500/50 bg-emerald-500/5 dark:bg-emerald-500/10'
                    : 'border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 hover:border-zinc-300 dark:hover:border-zinc-700'
                }`}
              >
                <div className="flex items-center justify-between">
                  <span className="font-semibold text-xs text-zinc-900 dark:text-white font-mono">{q.name}</span>
                  <span className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-100 dark:bg-zinc-800 text-zinc-500 font-mono">
                    DLQ: {q.has_dlq ? 'ON' : 'OFF'}
                  </span>
                </div>
                <div className="mt-3 flex items-center justify-between text-[11px] text-zinc-500">
                  <span>Ready: <strong className="text-zinc-900 dark:text-white font-mono">{q.stats?.ReadyCount ?? 0}</strong></span>
                  <span>In-Flight: <strong className="text-amber-500 font-mono">{q.stats?.InFlightCount ?? 0}</strong></span>
                  <span>DLQ: <strong className="text-rose-500 font-mono">{q.stats?.DLQCount ?? 0}</strong></span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
