import React, { useState } from 'react'
import {
  Terminal,
  Play,
  Layers,
  Clock,
  CheckCircle2,
  AlertCircle,
  Code2,
} from 'lucide-react'
import { api } from '../api'
import type { CatalogData, QueryResult } from '../types'

interface QueryConsoleViewProps {
  catalog: CatalogData | null
}

export const QueryConsoleView: React.FC<QueryConsoleViewProps> = ({ catalog }) => {
  const collections = catalog?.collections || []
  const [selectedCollection, setSelectedCollection] = useState(
    collections[0]?.name || ''
  )
  const [whereField, setWhereField] = useState('')
  const [whereOp, setWhereOp] = useState('eq')
  const [whereVal, setWhereVal] = useState('')
  const [orderField, setOrderField] = useState('')
  const [orderDesc, setOrderDesc] = useState(false)
  const [limit, setLimit] = useState(20)

  const [result, setResult] = useState<QueryResult | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleExecute = async (explainOnly = false) => {
    if (!selectedCollection) return
    setLoading(true)
    setError(null)
    try {
      let val: any = whereVal
      if (whereVal === 'true') val = true
      else if (whereVal === 'false') val = false
      else if (!isNaN(Number(whereVal)) && whereVal.trim() !== '') val = Number(whereVal)

      const filters = whereField.trim()
        ? [{ field: whereField.trim(), op: whereOp, value: val }]
        : []

      const res = await api.queryDocuments(selectedCollection, {
        filters,
        order_by: orderField.trim() ? { field: orderField.trim(), desc: orderDesc } : undefined,
        limit,
        explain: explainOnly,
      })
      setResult(res)
    } catch (err: any) {
      setError(err.message || 'Query execution failed')
    } finally {
      setLoading(false)
    }
  }

  // Construct Fluent Go query snippet for reference
  const codeSnippet = `col := db.Collection("${selectedCollection || 'collection'}")
rows, err := col.Query()${
    whereField ? `\n  .Where("${whereField}").${whereOp === 'gte' ? 'Gte' : whereOp === 'gt' ? 'Gt' : whereOp === 'lt' ? 'Lt' : whereOp === 'lte' ? 'Lte' : 'Eq'}(${whereVal})` : ''
  }${
    orderField ? `\n  .OrderBy("${orderField}", coco.${orderDesc ? 'Desc' : 'Asc'})` : ''
  }${limit ? `\n  .Limit(${limit})` : ''}
  .All()`

  return (
    <div className="p-8 space-y-6 max-w-7xl mx-auto">
      {/* Header */}
      <div>
        <h1 className="text-xl font-bold tracking-tight text-zinc-900 dark:text-white flex items-center gap-2.5">
          <Terminal className="w-5 h-5 text-emerald-600 dark:text-emerald-400" />
          <span>Query Console & Execution Planner</span>
        </h1>
        <p className="text-xs text-zinc-500 dark:text-zinc-400 mt-1">
          Execute structured queries, inspect Volcano physical operators, and analyze AST plans
        </p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Left: Query Builder Panel */}
        <div className="space-y-4">
          <div className="p-5 rounded-xl bg-white dark:bg-zinc-900/70 border border-zinc-200 dark:border-zinc-800/80 shadow-xs space-y-4">
            <h2 className="text-xs font-semibold text-zinc-600 dark:text-zinc-300 uppercase tracking-wider">
              Query Parameters
            </h2>

            <div>
              <label className="text-xs text-zinc-600 dark:text-zinc-400 block mb-1">Target Collection</label>
              <select
                value={selectedCollection}
                onChange={(e) => setSelectedCollection(e.target.value)}
                className="w-full bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg p-2.5 text-xs text-zinc-900 dark:text-zinc-200 focus:outline-none focus:border-emerald-500/50"
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

            <div className="space-y-2">
              <label className="text-xs text-zinc-600 dark:text-zinc-400 block">Predicate (Where)</label>
              <div className="grid grid-cols-3 gap-2">
                <input
                  type="text"
                  placeholder="field"
                  value={whereField}
                  onChange={(e) => setWhereField(e.target.value)}
                  className="bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg p-2 text-xs text-zinc-900 dark:text-zinc-200 font-mono"
                />
                <select
                  value={whereOp}
                  onChange={(e) => setWhereOp(e.target.value)}
                  className="bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg p-2 text-xs text-zinc-800 dark:text-zinc-200"
                >
                  <option value="eq">==</option>
                  <option value="gt">&gt;</option>
                  <option value="gte">&gt;=</option>
                  <option value="lt">&lt;</option>
                  <option value="lte">&lt;=</option>
                </select>
                <input
                  type="text"
                  placeholder="value"
                  value={whereVal}
                  onChange={(e) => setWhereVal(e.target.value)}
                  className="bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg p-2 text-xs text-zinc-900 dark:text-zinc-200 font-mono"
                />
              </div>
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="text-xs text-zinc-600 dark:text-zinc-400 block mb-1">Order By</label>
                <input
                  type="text"
                  placeholder="field"
                  value={orderField}
                  onChange={(e) => setOrderField(e.target.value)}
                  className="w-full bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg p-2 text-xs text-zinc-900 dark:text-zinc-200 font-mono"
                />
              </div>
              <div>
                <label className="text-xs text-zinc-600 dark:text-zinc-400 block mb-1">Direction</label>
                <select
                  value={orderDesc ? 'desc' : 'asc'}
                  onChange={(e) => setOrderDesc(e.target.value === 'desc')}
                  className="w-full bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg p-2 text-xs text-zinc-800 dark:text-zinc-200"
                >
                  <option value="asc">Ascending (ASC)</option>
                  <option value="desc">Descending (DESC)</option>
                </select>
              </div>
            </div>

            <div>
              <label className="text-xs text-zinc-600 dark:text-zinc-400 block mb-1">Result Limit</label>
              <input
                type="number"
                value={limit}
                onChange={(e) => setLimit(Number(e.target.value))}
                className="w-full bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg p-2 text-xs text-zinc-900 dark:text-zinc-200 font-mono"
              />
            </div>

            <div className="flex gap-2 pt-2">
              <button
                onClick={() => handleExecute(false)}
                disabled={loading || !selectedCollection}
                className="flex-1 flex items-center justify-center gap-1.5 px-4 py-2.5 rounded-lg bg-emerald-500 hover:bg-emerald-400 text-zinc-950 text-xs font-semibold shadow-md shadow-emerald-500/20 transition cursor-pointer disabled:opacity-50"
              >
                <Play className="w-3.5 h-3.5 fill-current" />
                <span>Execute</span>
              </button>
              <button
                onClick={() => handleExecute(true)}
                disabled={loading || !selectedCollection}
                className="px-3.5 py-2.5 rounded-lg bg-zinc-100 hover:bg-zinc-200 dark:bg-zinc-800 dark:hover:bg-zinc-700 text-zinc-700 dark:text-zinc-300 text-xs font-medium transition cursor-pointer disabled:opacity-50"
              >
                Explain Plan
              </button>
            </div>
          </div>

          {/* Code Snippet Box */}
          <div className="p-4 rounded-xl bg-white dark:bg-zinc-900/40 border border-zinc-200 dark:border-zinc-800/80 shadow-xs space-y-2">
            <div className="flex items-center gap-1.5 text-[11px] font-semibold text-zinc-500 dark:text-zinc-400 uppercase tracking-wider">
              <Code2 className="w-3.5 h-3.5 text-emerald-600 dark:text-emerald-400" />
              <span>Equivalent Go Code</span>
            </div>
            <pre className="p-3 rounded-lg bg-zinc-950 border border-zinc-800 text-[11px] font-mono text-zinc-300 overflow-x-auto leading-relaxed">
              {codeSnippet}
            </pre>
          </div>
        </div>

        {/* Right: Results & Physical Execution Plan */}
        <div className="lg:col-span-2 space-y-4">
          {/* Physical Plan Banner */}
          {result?.execution_plan && (
            <div className="p-4 rounded-xl bg-emerald-500/10 border border-emerald-500/20 space-y-1.5">
              <div className="flex items-center gap-2 text-xs font-semibold text-emerald-700 dark:text-emerald-400">
                <Layers className="w-4 h-4" />
                <span>Volcano Physical Execution Plan</span>
              </div>
              <div className="p-2.5 rounded-lg bg-zinc-950 font-mono text-xs text-emerald-300 border border-emerald-500/20">
                {result.execution_plan}
              </div>
            </div>
          )}

          {error && (
            <div className="p-3.5 rounded-lg bg-red-500/10 border border-red-500/20 text-red-600 dark:text-red-400 text-xs flex items-center gap-2">
              <AlertCircle className="w-4 h-4 shrink-0" />
              <span>{error}</span>
            </div>
          )}

          {/* Results Box */}
          <div className="rounded-xl bg-white dark:bg-zinc-900/70 border border-zinc-200 dark:border-zinc-800/80 shadow-xs overflow-hidden">
            <div className="p-4 border-b border-zinc-200 dark:border-zinc-800/80 flex items-center justify-between text-xs text-zinc-500 dark:text-zinc-400 bg-zinc-50/50 dark:bg-transparent">
              <div className="flex items-center gap-2 font-medium text-zinc-800 dark:text-zinc-300">
                <CheckCircle2 className="w-4 h-4 text-emerald-600 dark:text-emerald-400" />
                <span>Query Results ({result?.documents?.length || 0} rows)</span>
              </div>
              {result && (
                <span className="flex items-center gap-1 font-mono text-[11px] text-zinc-400 dark:text-zinc-500">
                  <Clock className="w-3 h-3" />
                  {result.duration_ms.toFixed(2)} ms
                </span>
              )}
            </div>

            <div className="p-4 overflow-auto max-h-[500px]">
              {loading ? (
                <div className="p-12 text-center text-xs text-zinc-500">
                  Executing Volcano operators...
                </div>
              ) : !result ? (
                <div className="p-12 text-center text-xs text-zinc-500">
                  Click Execute to run the query and view results.
                </div>
              ) : result.documents.length === 0 ? (
                <div className="p-12 text-center text-xs text-zinc-500">
                  No documents matched the query predicates.
                </div>
              ) : (
                <pre className="text-xs font-mono text-emerald-600 dark:text-emerald-400 leading-relaxed">
                  {JSON.stringify(result.documents, null, 2)}
                </pre>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
