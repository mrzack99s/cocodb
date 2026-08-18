import React, { useState, useEffect } from 'react'
import {
  Database,
  Plus,
  Trash2,
  Search,
  Clock,
  AlertCircle,
} from 'lucide-react'
import { api } from '../api'
import type { CatalogData, KVEntry } from '../types'

interface BucketsViewProps {
  catalog: CatalogData | null
  onRefreshCatalog: () => void
}

export const BucketsView: React.FC<BucketsViewProps> = ({
  catalog,
  onRefreshCatalog,
}) => {
  const buckets = catalog?.buckets || []
  const [selectedBucket, setSelectedBucket] = useState<string>('')
  const [entries, setEntries] = useState<KVEntry[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [prefix, setPrefix] = useState('')

  // Put Modal
  const [isModalOpen, setIsModalOpen] = useState(false)
  const [keyInput, setKeyInput] = useState('')
  const [valInput, setValInput] = useState('')
  const [ttlInput, setTtlInput] = useState('')

  useEffect(() => {
    if (buckets.length > 0 && !selectedBucket) {
      setSelectedBucket(buckets[0].name)
    }
  }, [buckets, selectedBucket])

  useEffect(() => {
    if (selectedBucket) {
      loadEntries()
    }
  }, [selectedBucket])

  const loadEntries = async () => {
    if (!selectedBucket) return
    setLoading(true)
    setError(null)
    try {
      const res = await api.scanBucket(selectedBucket, prefix, 100)
      setEntries(res.entries || [])
    } catch (err: any) {
      setError(err.message || 'Failed to scan bucket')
    } finally {
      setLoading(false)
    }
  }

  const handlePut = async () => {
    if (!keyInput.trim()) {
      setError('Key cannot be empty')
      return
    }
    setError(null)
    try {
      const ttlSec = ttlInput ? Number(ttlInput) : undefined
      await api.putKV(selectedBucket, keyInput, valInput, ttlSec)
      setIsModalOpen(false)
      setKeyInput('')
      setValInput('')
      setTtlInput('')
      loadEntries()
      onRefreshCatalog()
    } catch (err: any) {
      setError(err.message || 'Failed to put key-value')
    }
  }

  const handleDelete = async (key: string) => {
    if (!confirm(`Are you sure you want to delete key "${key}"?`)) return
    try {
      await api.deleteKV(selectedBucket, key)
      loadEntries()
    } catch (err: any) {
      setError(err.message || 'Failed to delete key')
    }
  }

  const handleIncrement = async (key: string, delta: number) => {
    try {
      await api.incrementKV(selectedBucket, key, delta)
      loadEntries()
    } catch (err: any) {
      setError(err.message || 'Increment failed (is value an 8-byte integer?)')
    }
  }

  return (
    <div className="p-8 space-y-6 max-w-7xl mx-auto">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold tracking-tight text-zinc-900 dark:text-white flex items-center gap-2.5">
            <Database className="w-5 h-5 text-teal-600 dark:text-teal-400" />
            <span>Key/Value Buckets</span>
          </h1>
          <p className="text-xs text-zinc-500 dark:text-zinc-400 mt-1">
            Ordered B+Tree transactional key/value store with TTL and atomic operations
          </p>
        </div>

        {selectedBucket && (
          <button
            onClick={() => {
              setKeyInput('')
              setValInput('')
              setTtlInput('')
              setIsModalOpen(true)
            }}
            className="flex items-center gap-1.5 px-3 py-2 rounded-lg bg-emerald-500 hover:bg-emerald-400 text-zinc-950 text-xs font-semibold shadow-md shadow-emerald-500/20 transition cursor-pointer"
          >
            <Plus className="w-4 h-4 stroke-[2.5]" />
            <span>Put Key/Value</span>
          </button>
        )}
      </div>

      {/* Bucket Tabs */}
      <div className="flex items-center gap-2 border-b border-zinc-200 dark:border-zinc-800 pb-3 overflow-x-auto">
        {buckets.length === 0 ? (
          <span className="text-xs text-zinc-400">No buckets found</span>
        ) : (
          buckets.map((b) => (
            <button
              key={b.name}
              onClick={() => setSelectedBucket(b.name)}
              className={`px-3 py-1.5 rounded-lg text-xs font-medium transition cursor-pointer ${
                selectedBucket === b.name
                  ? 'bg-teal-500/15 text-teal-700 dark:text-teal-300 border border-teal-500/30 font-semibold'
                  : 'text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-200 hover:bg-zinc-200/50 dark:hover:bg-zinc-800/50 border border-transparent'
              }`}
            >
              {b.name}
            </button>
          ))
        )}
      </div>

      {/* Prefix Search Bar */}
      <div className="p-3.5 rounded-xl bg-white dark:bg-zinc-900/60 border border-zinc-200 dark:border-zinc-800/80 shadow-xs flex items-center gap-3">
        <div className="flex items-center gap-2 text-xs text-zinc-600 dark:text-zinc-400">
          <Search className="w-3.5 h-3.5" />
          <span>Prefix Scan:</span>
        </div>

        <input
          type="text"
          placeholder="Filter by key prefix (e.g. user:, session:)"
          value={prefix}
          onChange={(e) => setPrefix(e.target.value)}
          className="flex-1 bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg px-3 py-1.5 text-xs text-zinc-900 dark:text-zinc-200 placeholder-zinc-400 dark:placeholder-zinc-600 focus:outline-none focus:border-teal-500/50 font-mono"
        />

        <button
          onClick={loadEntries}
          className="px-3.5 py-1.5 rounded-lg bg-zinc-200 hover:bg-zinc-300 dark:bg-zinc-800 dark:hover:bg-zinc-700 text-zinc-800 dark:text-zinc-200 text-xs font-medium transition cursor-pointer"
        >
          Scan
        </button>
      </div>

      {error && (
        <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-red-600 dark:text-red-400 text-xs flex items-center gap-2">
          <AlertCircle className="w-4 h-4 shrink-0" />
          <span>{error}</span>
        </div>
      )}

      {/* KV Table */}
      <div className="rounded-xl bg-white dark:bg-zinc-900/70 border border-zinc-200 dark:border-zinc-800/80 shadow-xs overflow-hidden">
        <div className="p-4 border-b border-zinc-200 dark:border-zinc-800/80 flex items-center justify-between text-xs text-zinc-500 dark:text-zinc-400 font-medium bg-zinc-50/50 dark:bg-transparent">
          <span>{entries.length} Keys Found</span>
          <span className="font-mono text-[11px] text-zinc-400 dark:text-zinc-500">B+Tree Lexicographical Order</span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-zinc-50 dark:bg-zinc-950/60 border-b border-zinc-200 dark:border-zinc-800 text-zinc-500 dark:text-zinc-400 uppercase tracking-wider text-[10px]">
              <tr>
                <th className="p-3.5 font-mono">Key</th>
                <th className="p-3.5 font-mono">Value</th>
                <th className="p-3.5 font-mono">Size</th>
                <th className="p-3.5 text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-200 dark:divide-zinc-800/50 font-mono">
              {loading ? (
                <tr>
                  <td colSpan={4} className="p-8 text-center text-zinc-500">
                    Scanning bucket B+Tree...
                  </td>
                </tr>
              ) : entries.length === 0 ? (
                <tr>
                  <td colSpan={4} className="p-8 text-center text-zinc-500">
                    Bucket is empty.
                  </td>
                </tr>
              ) : (
                entries.map((entry) => (
                  <tr key={entry.key} className="hover:bg-zinc-50 dark:hover:bg-zinc-800/30 transition">
                    <td className="p-3.5 text-teal-600 dark:text-teal-400 font-semibold max-w-[240px] truncate">
                      {entry.key}
                    </td>
                    <td className="p-3.5 text-zinc-700 dark:text-zinc-300 max-w-[400px] truncate">
                      {entry.value}
                    </td>
                    <td className="p-3.5 text-zinc-400 dark:text-zinc-500 text-[11px]">
                      {entry.size} B
                    </td>
                    <td className="p-3.5 text-right space-x-1.5">
                      <button
                        onClick={() => handleIncrement(entry.key, 1)}
                        className="px-2 py-1 rounded bg-zinc-100 hover:bg-zinc-200 dark:bg-zinc-800 dark:hover:bg-zinc-700 text-zinc-700 dark:text-zinc-300 text-[11px] transition cursor-pointer"
                        title="Atomic Increment (+1)"
                      >
                        +1
                      </button>
                      <button
                        onClick={() => handleDelete(entry.key)}
                        className="p-1.5 rounded hover:bg-red-50 dark:hover:bg-zinc-800 text-red-600 dark:text-red-400 hover:text-red-700 dark:hover:text-red-300 transition cursor-pointer inline-flex"
                        title="Delete Key"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Put Modal */}
      {isModalOpen && (
        <div className="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4 z-50">
          <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-xl p-6 max-w-lg w-full space-y-4 shadow-2xl">
            <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 pb-3">
              <h3 className="text-sm font-semibold text-zinc-900 dark:text-white">Put Key/Value</h3>
              <button
                onClick={() => setIsModalOpen(false)}
                className="text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300 text-xs cursor-pointer"
              >
                Cancel
              </button>
            </div>

            <div className="space-y-3">
              <div>
                <label className="text-xs text-zinc-600 dark:text-zinc-400 block mb-1">Key</label>
                <input
                  type="text"
                  placeholder="e.g. user:1001"
                  value={keyInput}
                  onChange={(e) => setKeyInput(e.target.value)}
                  className="w-full bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg p-2.5 text-xs font-mono text-zinc-900 dark:text-zinc-200 focus:outline-none focus:border-teal-500/50"
                />
              </div>

              <div>
                <label className="text-xs text-zinc-600 dark:text-zinc-400 block mb-1">Value</label>
                <textarea
                  placeholder="String, JSON, or payload"
                  value={valInput}
                  onChange={(e) => setValInput(e.target.value)}
                  rows={4}
                  className="w-full bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg p-2.5 text-xs font-mono text-zinc-900 dark:text-zinc-200 focus:outline-none focus:border-teal-500/50"
                />
              </div>

              <div>
                <label className="text-xs text-zinc-600 dark:text-zinc-400 block mb-1 flex items-center gap-1">
                  <Clock className="w-3 h-3 text-zinc-400 dark:text-zinc-500" />
                  <span>Optional TTL (Seconds)</span>
                </label>
                <input
                  type="number"
                  placeholder="e.g. 60 (expires in 60s)"
                  value={ttlInput}
                  onChange={(e) => setTtlInput(e.target.value)}
                  className="w-full bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg p-2.5 text-xs font-mono text-zinc-900 dark:text-zinc-200 focus:outline-none focus:border-teal-500/50"
                />
              </div>
            </div>

            <div className="flex justify-end gap-2 pt-2">
              <button
                onClick={() => setIsModalOpen(false)}
                className="px-4 py-2 rounded-lg bg-zinc-100 hover:bg-zinc-200 dark:bg-zinc-800 dark:hover:bg-zinc-700 text-zinc-700 dark:text-zinc-300 text-xs font-medium cursor-pointer"
              >
                Cancel
              </button>
              <button
                onClick={handlePut}
                className="px-4 py-2 rounded-lg bg-teal-500 hover:bg-teal-400 text-zinc-950 text-xs font-semibold cursor-pointer"
              >
                Save Key
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
