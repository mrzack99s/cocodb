import React, { useState, useEffect } from 'react'
import {
  Radio,
  Send,
  RefreshCw,
  CheckCircle2,
  AlertTriangle,
  Zap,
  Users,
  Activity,
  Layers,
} from 'lucide-react'
import { api } from '../api'
import type { PubSubEvent } from '../types'

export const PubSubView: React.FC = () => {
  const [events, setEvents] = useState<PubSubEvent[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  // Publish Form State
  const [topic, setTopic] = useState('sensors.iot.temperature')
  const [payload, setPayload] = useState('{\n  "device_id": "SENSOR-01",\n  "temp_celsius": 24.5,\n  "humidity": 55\n}')
  const [dedupId, setDedupId] = useState('')

  const loadHistory = async () => {
    try {
      const data = await api.getPubSubHistory()
      setEvents(data.events || [])
    } catch (err: any) {
      // ignore in background
    }
  }

  useEffect(() => {
    loadHistory()
    const timer = setInterval(loadHistory, 2000)
    return () => clearInterval(timer)
  }, [])

  const handlePublish = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    setSuccess(null)
    setLoading(true)
    try {
      const res = await api.publishPubSub(topic, payload, dedupId || undefined)
      setSuccess(`Event broadcasted to topic "${res.topic}" (${res.subscribers_hit} active subscribers hit)`)
      loadHistory()
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="p-8 space-y-8 max-w-7xl mx-auto">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-zinc-900 dark:text-white flex items-center gap-2.5">
            <Radio className="w-6 h-6 text-emerald-500" />
            <span>Real-Time Pub/Sub Broker</span>
            <span className="text-xs font-mono px-2 py-0.5 rounded-md bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border border-emerald-500/20 font-medium">
              Wildcards & Consumer Groups
            </span>
          </h1>
          <p className="text-xs text-zinc-500 dark:text-zinc-400 mt-1">
            Ultra-fast in-memory topic routing, wildcard subscriptions, and distributed worker group load-sharing
          </p>
        </div>

        <div className="flex items-center gap-3">
          <button
            onClick={loadHistory}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300 text-xs font-medium hover:bg-zinc-50 dark:hover:bg-zinc-800 transition cursor-pointer"
          >
            <RefreshCw className="w-3.5 h-3.5" />
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

      {/* Architecture Highlights Banner */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="p-4 rounded-2xl border border-zinc-200/80 dark:border-zinc-800/80 bg-white/70 dark:bg-zinc-900/50 backdrop-blur-md flex items-start gap-3">
          <div className="p-2 rounded-xl bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
            <Activity className="w-4 h-4" />
          </div>
          <div>
            <h4 className="text-xs font-semibold text-zinc-900 dark:text-white">~1.95M Broadcasts/sec</h4>
            <p className="text-[11px] text-zinc-500 mt-0.5">High-throughput lock-free routing tree with zero interface allocations.</p>
          </div>
        </div>

        <div className="p-4 rounded-2xl border border-zinc-200/80 dark:border-zinc-800/80 bg-white/70 dark:bg-zinc-900/50 backdrop-blur-md flex items-start gap-3">
          <div className="p-2 rounded-xl bg-teal-500/10 text-teal-600 dark:text-teal-400">
            <Layers className="w-4 h-4" />
          </div>
          <div>
            <h4 className="text-xs font-semibold text-zinc-900 dark:text-white">Wildcard Patterns</h4>
            <p className="text-[11px] text-zinc-500 mt-0.5">Supports <code>*</code> (single-segment) and <code>&gt;</code> (multi-level wildcards).</p>
          </div>
        </div>

        <div className="p-4 rounded-2xl border border-zinc-200/80 dark:border-zinc-800/80 bg-white/70 dark:bg-zinc-900/50 backdrop-blur-md flex items-start gap-3">
          <div className="p-2 rounded-xl bg-indigo-500/10 text-indigo-600 dark:text-indigo-400">
            <Users className="w-4 h-4" />
          </div>
          <div>
            <h4 className="text-xs font-semibold text-zinc-900 dark:text-white">Consumer Groups</h4>
            <p className="text-[11px] text-zinc-500 mt-0.5">Dispatches each message to exactly one competing worker in a group.</p>
          </div>
        </div>
      </div>

      {/* Main Grid: Publisher vs Live Stream */}
      <div className="grid grid-cols-1 lg:grid-cols-12 gap-8">
        {/* Publish Form (5 cols) */}
        <div className="lg:col-span-5 p-6 rounded-2xl border border-zinc-200/80 dark:border-zinc-800/80 bg-white/70 dark:bg-zinc-900/50 backdrop-blur-md space-y-5">
          <h2 className="text-base font-semibold text-zinc-900 dark:text-white flex items-center gap-2">
            <Send className="w-4 h-4 text-emerald-500" />
            <span>Publish Topic Event</span>
          </h2>

          <form onSubmit={handlePublish} className="space-y-4">
            <div>
              <label className="block text-xs font-medium text-zinc-700 dark:text-zinc-300 mb-1">
                Topic Name
              </label>
              <input
                type="text"
                value={topic}
                onChange={(e) => setTopic(e.target.value)}
                placeholder="e.g. orders.created or sensors.kitchen.temp"
                className="w-full px-3 py-2 rounded-xl bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 text-xs font-mono text-zinc-900 dark:text-white focus:outline-none focus:border-emerald-500"
              />
            </div>

            <div>
              <label className="block text-xs font-medium text-zinc-700 dark:text-zinc-300 mb-1">
                Payload
              </label>
              <textarea
                rows={5}
                value={payload}
                onChange={(e) => setPayload(e.target.value)}
                className="w-full p-3 rounded-xl bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 text-xs font-mono text-zinc-900 dark:text-white focus:outline-none focus:border-emerald-500"
              />
            </div>

            <div>
              <label className="block text-xs font-medium text-zinc-700 dark:text-zinc-300 mb-1">
                Dedup ID (Optional Deduplication Window)
              </label>
              <input
                type="text"
                value={dedupId}
                onChange={(e) => setDedupId(e.target.value)}
                placeholder="e.g. evt_unique_123"
                className="w-full px-3 py-2 rounded-xl bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 text-xs font-mono text-zinc-900 dark:text-white focus:outline-none focus:border-emerald-500"
              />
            </div>

            <button
              type="submit"
              disabled={loading}
              className="w-full py-2.5 rounded-xl bg-emerald-500 hover:bg-emerald-400 text-zinc-950 font-semibold text-xs shadow-md shadow-emerald-500/20 transition cursor-pointer flex items-center justify-center gap-2"
            >
              <Zap className="w-4 h-4 stroke-[2.5]" />
              <span>Broadcast Event</span>
            </button>
          </form>
        </div>

        {/* Live Stream / History (7 cols) */}
        <div className="lg:col-span-7 p-6 rounded-2xl border border-zinc-200/80 dark:border-zinc-800/80 bg-white/70 dark:bg-zinc-900/50 backdrop-blur-md space-y-4">
          <div className="flex items-center justify-between">
            <h2 className="text-base font-semibold text-zinc-900 dark:text-white flex items-center gap-2">
              <Activity className="w-4 h-4 text-emerald-500" />
              <span>Live Broadcast Stream ({events.length})</span>
            </h2>
            <span className="text-[10px] font-mono text-zinc-400">Auto-refreshing</span>
          </div>

          <div className="space-y-2.5 max-h-[480px] overflow-y-auto pr-1">
            {events.length === 0 ? (
              <div className="py-16 text-center text-zinc-400 text-xs space-y-2">
                <Radio className="w-8 h-8 stroke-1 mx-auto text-zinc-300 dark:text-zinc-600" />
                <p>No broadcast events recorded yet.</p>
              </div>
            ) : (
              events.map((evt) => (
                <div
                  key={evt.id}
                  className="p-3.5 rounded-xl border border-zinc-200 dark:border-zinc-800/80 bg-white dark:bg-zinc-950/80 space-y-1.5 transition hover:border-zinc-300 dark:hover:border-zinc-700"
                >
                  <div className="flex items-center justify-between">
                    <span className="font-mono text-xs font-semibold text-emerald-600 dark:text-emerald-400">
                      {evt.topic}
                    </span>
                    <span className="text-[10px] text-zinc-400 font-mono">
                      {new Date(evt.created_at).toLocaleTimeString()}
                    </span>
                  </div>
                  <pre className="p-2 rounded bg-zinc-50 dark:bg-zinc-900/60 border border-zinc-100 dark:border-zinc-800/50 text-[11px] font-mono text-zinc-700 dark:text-zinc-300 overflow-x-auto">
                    {evt.payload}
                  </pre>
                  {evt.dedup_id && (
                    <div className="text-[10px] text-zinc-400 font-mono">
                      Dedup ID: <span className="text-zinc-600 dark:text-zinc-300">{evt.dedup_id}</span>
                    </div>
                  )}
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
