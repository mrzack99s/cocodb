import React, { useState } from 'react'
import {
  Search,
  Sparkles,
  AlertCircle,
  Tag,
} from 'lucide-react'
import { api } from '../api'
import type { CatalogData, TextSearchResult } from '../types'

interface SearchPlaygroundViewProps {
  catalog: CatalogData | null
}

export const SearchPlaygroundView: React.FC<SearchPlaygroundViewProps> = ({
  catalog,
}) => {
  const collections = catalog?.collections || []
  const [collection, setCollection] = useState(collections[0]?.name || '')
  const [field, setField] = useState('title')
  const [queryText, setQueryText] = useState('')
  const [k, setK] = useState(10)

  const [results, setResults] = useState<TextSearchResult[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleSearch = async () => {
    if (!collection) {
      setError('Please select a collection')
      return
    }
    if (!queryText.trim()) {
      setError('Please enter search keywords')
      return
    }
    setError(null)
    setLoading(true)
    try {
      const res = await api.searchFullText(collection, field, queryText, k)
      setResults(res.results || [])
    } catch (err: any) {
      setError(err.message || 'Full-text search failed')
    } finally {
      setLoading(false)
    }
  }

  // Tokenizer terms preview
  const queryTokens = queryText
    .toLowerCase()
    .split(/\W+/)
    .filter(Boolean)

  return (
    <div className="p-8 space-y-6 max-w-7xl mx-auto">
      {/* Header */}
      <div>
        <h1 className="text-xl font-bold tracking-tight text-zinc-900 dark:text-white flex items-center gap-2.5">
          <Search className="w-5 h-5 text-sky-600 dark:text-sky-400" />
          <span>Full-Text Search & BM25 Scoring</span>
        </h1>
        <p className="text-xs text-zinc-500 dark:text-zinc-400 mt-1">
          Unicode-aware tokenization, Inverted Index postings, and BM25 relevance scoring
        </p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Search Config */}
        <div className="p-5 rounded-xl bg-white dark:bg-zinc-900/70 border border-zinc-200 dark:border-zinc-800/80 shadow-xs space-y-4">
          <h2 className="text-xs font-semibold text-zinc-600 dark:text-zinc-300 uppercase tracking-wider">
            Search Query Config
          </h2>

          <div>
            <label className="text-xs text-zinc-600 dark:text-zinc-400 block mb-1">Collection</label>
            <select
              value={collection}
              onChange={(e) => setCollection(e.target.value)}
              className="w-full bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg p-2 text-xs text-zinc-900 dark:text-zinc-200 focus:outline-none focus:border-sky-500/50"
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
            <label className="text-xs text-zinc-600 dark:text-zinc-400 block mb-1">Indexed Field</label>
            <input
              type="text"
              value={field}
              onChange={(e) => setField(e.target.value)}
              placeholder="e.g. content, description, title"
              className="w-full bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg p-2 text-xs text-zinc-900 dark:text-zinc-200 font-mono focus:outline-none focus:border-sky-500/50"
            />
          </div>

          <div>
            <label className="text-xs text-zinc-600 dark:text-zinc-400 block mb-1">Search Keywords</label>
            <input
              type="text"
              value={queryText}
              onChange={(e) => setQueryText(e.target.value)}
              placeholder="e.g. high performance database"
              className="w-full bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg p-2 text-xs text-zinc-900 dark:text-zinc-200 focus:outline-none focus:border-sky-500/50"
            />
          </div>

          {/* Tokenizer chips */}
          {queryTokens.length > 0 && (
            <div className="space-y-1.5 pt-1">
              <label className="text-[11px] text-zinc-500 block">Parsed Token Terms:</label>
              <div className="flex flex-wrap gap-1.5">
                {queryTokens.map((tok, i) => (
                  <span
                    key={i}
                    className="inline-flex items-center gap-1 text-[10px] font-mono px-2 py-0.5 rounded bg-sky-500/10 text-sky-700 dark:text-sky-400 border border-sky-500/20"
                  >
                    <Tag className="w-2.5 h-2.5" />
                    {tok}
                  </span>
                ))}
              </div>
            </div>
          )}

          <div>
            <label className="text-xs text-zinc-600 dark:text-zinc-400 block mb-1">Max Results ({k})</label>
            <input
              type="range"
              min={1}
              max={30}
              value={k}
              onChange={(e) => setK(Number(e.target.value))}
              className="w-full accent-sky-500 cursor-pointer"
            />
          </div>

          <button
            onClick={handleSearch}
            disabled={loading || collections.length === 0}
            className="w-full flex items-center justify-center gap-2 px-4 py-2.5 rounded-lg bg-sky-600 hover:bg-sky-500 text-white text-xs font-semibold shadow-md shadow-sky-600/20 transition cursor-pointer disabled:opacity-50"
          >
            <Search className="w-3.5 h-3.5" />
            <span>Execute BM25 Query</span>
          </button>
        </div>

        {/* Results Box */}
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
                <Sparkles className="w-4 h-4 text-sky-600 dark:text-sky-400" />
                <span>Ranked Search Results ({results.length})</span>
              </div>
              <span className="font-mono text-[11px] text-zinc-400 dark:text-zinc-500">
                k1=1.2 · b=0.75 · IDF Weighted
              </span>
            </div>

            <div className="p-4">
              {loading ? (
                <div className="p-12 text-center text-xs text-zinc-500">
                  Scanning inverted index postings...
                </div>
              ) : results.length === 0 ? (
                <div className="p-12 text-center text-xs text-zinc-500">
                  Enter query keywords and click Execute to test BM25 rankings.
                </div>
              ) : (
                <div className="space-y-3">
                  {results.map((res, i) => (
                    <div
                      key={res.record_id || i}
                      className="p-4 rounded-lg bg-zinc-50 dark:bg-zinc-950/70 border border-zinc-200 dark:border-zinc-800/80 space-y-2"
                    >
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                          <span className="w-5 h-5 rounded-full bg-sky-500/20 text-sky-700 dark:text-sky-400 text-xs flex items-center justify-center font-mono font-semibold">
                            {i + 1}
                          </span>
                          <span className="text-xs font-semibold text-zinc-800 dark:text-zinc-200 font-mono">
                            Doc ID: {res.doc_id || `#${res.record_id}`}
                          </span>
                        </div>
                        <span className="px-2 py-0.5 rounded bg-sky-500/15 border border-sky-500/30 text-sky-700 dark:text-sky-400 text-xs font-mono font-semibold">
                          Score: {res.score.toFixed(3)}
                        </span>
                      </div>

                      {res.document && (
                        <pre className="p-2.5 rounded bg-zinc-950 text-[11px] font-mono text-emerald-400 overflow-x-auto">
                          {JSON.stringify(res.document, null, 2)}
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
