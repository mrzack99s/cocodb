import React, { useState } from 'react'
import {
  BrainCircuit,
  Search,
  Sparkles,
  AlertCircle,
} from 'lucide-react'
import { api } from '../api'
import type { CatalogData, VectorSearchResult } from '../types'

interface VectorPlaygroundViewProps {
  catalog: CatalogData | null
}

export const VectorPlaygroundView: React.FC<VectorPlaygroundViewProps> = ({
  catalog,
}) => {
  const collections = catalog?.collections || []
  const [collection, setCollection] = useState(collections[0]?.name || '')
  const [field, setField] = useState('embedding')
  const [metric, setMetric] = useState<'cosine' | 'l2' | 'dot'>('cosine')
  const [k, setK] = useState(5)
  const [vectorText, setVectorText] = useState('')

  const [results, setResults] = useState<VectorSearchResult[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleSearch = async () => {
    if (!collection) {
      setError('Please select a collection')
      return
    }
    setError(null)
    setLoading(true)
    try {
      let vec: number[]
      if (vectorText.startsWith('[')) {
        vec = JSON.parse(vectorText)
      } else {
        vec = vectorText
          .split(',')
          .map((s) => parseFloat(s.trim()))
          .filter((n) => !isNaN(n))
      }

      if (vec.length === 0) {
        throw new Error('Please enter query vector coordinates (e.g. 0.1, 0.2, 0.3)')
      }

      const res = await api.searchVectors(collection, field, vec, k, metric)
      setResults(res.matches || [])
    } catch (err: any) {
      setError(err.message || 'Vector search failed')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="p-8 space-y-6 max-w-7xl mx-auto">
      {/* Header */}
      <div>
        <h1 className="text-xl font-bold tracking-tight text-zinc-900 dark:text-white flex items-center gap-2.5">
          <BrainCircuit className="w-5 h-5 text-purple-600 dark:text-purple-400" />
          <span>Vector Search Playground</span>
        </h1>
        <p className="text-xs text-zinc-500 dark:text-zinc-400 mt-1">
          Perform high-dimensional approximate nearest neighbor (HNSW) similarity searches
        </p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Search Controls */}
        <div className="p-5 rounded-xl bg-white dark:bg-zinc-900/70 border border-zinc-200 dark:border-zinc-800/80 shadow-xs space-y-4">
          <h2 className="text-xs font-semibold text-zinc-600 dark:text-zinc-300 uppercase tracking-wider">
            Vector Query Config
          </h2>

          <div>
            <label className="text-xs text-zinc-600 dark:text-zinc-400 block mb-1">Collection</label>
            <select
              value={collection}
              onChange={(e) => setCollection(e.target.value)}
              className="w-full bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg p-2 text-xs text-zinc-900 dark:text-zinc-200 focus:outline-none focus:border-purple-500/50"
            >
              {collections.length === 0 ? (
                <option value="">(No collections in database)</option>
              ) : (
                collections.map((c) => (
                  <option key={c.name} value={c.name}>
                    {c.name}
                  </option>
                ))
              )}
            </select>
          </div>

          <div>
            <label className="text-xs text-zinc-600 dark:text-zinc-400 block mb-1">Vector Field</label>
            <input
              type="text"
              value={field}
              onChange={(e) => setField(e.target.value)}
              placeholder="e.g. embedding, vector"
              className="w-full bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg p-2 text-xs text-zinc-900 dark:text-zinc-200 font-mono focus:outline-none focus:border-purple-500/50"
            />
          </div>

          <div>
            <label className="text-xs text-zinc-600 dark:text-zinc-400 block mb-1">Distance Metric</label>
            <select
              value={metric}
              onChange={(e) => setMetric(e.target.value as any)}
              className="w-full bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg p-2 text-xs text-zinc-800 dark:text-zinc-200 focus:outline-none focus:border-purple-500/50"
            >
              <option value="cosine">Cosine Distance (1.0 - Similarity)</option>
              <option value="l2">Euclidean Distance (L2 Norm)</option>
              <option value="dot">Dot Product (Negative Inner Product)</option>
            </select>
          </div>

          <div>
            <label className="text-xs text-zinc-600 dark:text-zinc-400 block mb-1">Top-K Nearest Neighbors ({k})</label>
            <input
              type="range"
              min={1}
              max={50}
              value={k}
              onChange={(e) => setK(Number(e.target.value))}
              className="w-full accent-purple-500 cursor-pointer"
            />
          </div>

          <div>
            <label className="text-xs text-zinc-600 dark:text-zinc-400 block mb-1">Query Vector Coordinates</label>
            <textarea
              rows={4}
              value={vectorText}
              onChange={(e) => setVectorText(e.target.value)}
              placeholder="0.12, 0.45, 0.89, ..."
              className="w-full bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg p-2 text-xs font-mono text-zinc-900 dark:text-zinc-200 focus:outline-none focus:border-purple-500/50"
            />
          </div>

          <button
            onClick={handleSearch}
            disabled={loading || collections.length === 0}
            className="w-full flex items-center justify-center gap-2 px-4 py-2.5 rounded-lg bg-purple-600 hover:bg-purple-500 text-white text-xs font-semibold shadow-md shadow-purple-600/20 transition cursor-pointer disabled:opacity-50"
          >
            <Search className="w-3.5 h-3.5" />
            <span>Search Nearest Vectors</span>
          </button>
        </div>

        {/* Search Results */}
        <div className="lg:col-span-2 space-y-4">
          {error && (
            <div className="p-3.5 rounded-lg bg-red-500/10 border border-red-500/20 text-red-600 dark:text-red-400 text-xs flex items-center gap-2">
              <AlertCircle className="w-4 h-4 shrink-0" />
              <span>{error}</span>
            </div>
          )}

          <div className="rounded-xl bg-white dark:bg-zinc-900/70 border border-zinc-200 dark:border-zinc-800/80 shadow-xs overflow-hidden">
            <div className="p-4 border-b border-zinc-200 dark:border-zinc-800/80 flex items-center justify-between text-xs text-zinc-500 dark:text-zinc-400 font-medium bg-zinc-50/50 dark:bg-transparent">
              <div className="flex items-center gap-2 text-zinc-800 dark:text-zinc-200">
                <Sparkles className="w-4 h-4 text-purple-600 dark:text-purple-400" />
                <span>Top-{k} Nearest Neighbors</span>
              </div>
              <span className="font-mono text-[11px] text-zinc-400 dark:text-zinc-500">
                HNSW Graph Beam Search
              </span>
            </div>

            <div className="p-4">
              {loading ? (
                <div className="p-12 text-center text-xs text-zinc-500">
                  Traversing HNSW layers...
                </div>
              ) : results.length === 0 ? (
                <div className="p-12 text-center text-xs text-zinc-500">
                  Enter vector coordinates and click Search to view nearest matches.
                </div>
              ) : (
                <div className="space-y-3">
                  {results.map((match, i) => (
                    <div
                      key={match.id || i}
                      className="p-4 rounded-lg bg-zinc-50 dark:bg-zinc-950/70 border border-zinc-200 dark:border-zinc-800/80 space-y-2.5"
                    >
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                          <span className="w-5 h-5 rounded-full bg-purple-500/20 text-purple-700 dark:text-purple-400 text-xs flex items-center justify-center font-mono font-semibold">
                            {i + 1}
                          </span>
                          <span className="text-xs font-semibold text-zinc-800 dark:text-zinc-200 font-mono">
                            Doc ID: {match.doc_id || `#${match.id}`}
                          </span>
                        </div>
                        <div className="text-xs font-mono text-zinc-500 dark:text-zinc-400">
                          Dist: <span className="text-purple-600 dark:text-purple-400 font-medium">{match.distance.toFixed(4)}</span>
                        </div>
                      </div>

                      {/* Similarity Bar */}
                      <div className="space-y-1">
                        <div className="flex justify-between text-[10px] text-zinc-500">
                          <span>Similarity Score</span>
                          <span>{match.similarity_pct.toFixed(1)}%</span>
                        </div>
                        <div className="h-1.5 w-full bg-zinc-200 dark:bg-zinc-800 rounded-full overflow-hidden">
                          <div
                            className="h-full bg-gradient-to-r from-purple-500 to-indigo-400 rounded-full"
                            style={{ width: `${Math.max(5, match.similarity_pct)}%` }}
                          ></div>
                        </div>
                      </div>

                      {/* Document JSON Preview */}
                      {match.document && (
                        <pre className="p-2.5 rounded bg-zinc-950 text-[11px] font-mono text-emerald-400 overflow-x-auto">
                          {JSON.stringify(match.document, null, 2)}
                        </pre>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
