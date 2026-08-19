import React, { useEffect, useRef, useState } from 'react'
import { Activity, AlertTriangle, CheckCircle2, Clock3, Filter, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { api } from '../api'
import type { TimeSeriesPoint } from '../types'

interface TimeSeriesViewProps {
  onRefreshCatalog: () => void
}

const toLocalInput = (date: Date) => {
  const offset = date.getTimezoneOffset() * 60_000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}

const parseJSONMap = (value: string, label: string) => {
  const parsed: unknown = JSON.parse(value)
  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error(`${label} must be a JSON object`)
  }
  return parsed as Record<string, any>
}

export const TimeSeriesView: React.FC<TimeSeriesViewProps> = ({ onRefreshCatalog }) => {
  const [series, setSeries] = useState<string[]>([])
  const [selected, setSelected] = useState('')
  const [points, setPoints] = useState<TimeSeriesPoint[]>([])
  const [start, setStart] = useState(() => toLocalInput(new Date(Date.now() - 24 * 60 * 60 * 1000)))
  const [end, setEnd] = useState(() => toLocalInput(new Date()))
  const [tags, setTags] = useState('{}')
  const [limit, setLimit] = useState(100)
  const [descending, setDescending] = useState(true)
  const [timestamp, setTimestamp] = useState(() => toLocalInput(new Date()))
  const [writeTags, setWriteTags] = useState('{\n  "device": "sensor-01"\n}')
  const [fields, setFields] = useState('{\n  "temperature": 22.4\n}')
  const [retentionCutoff, setRetentionCutoff] = useState(() => toLocalInput(new Date(Date.now() - 30 * 24 * 60 * 60 * 1000)))
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const latestRequest = useRef(0)

  const loadSeries = async () => {
    const response = await api.listTimeSeries()
    setSeries(response.series)
    setSelected((current) => current || response.series[0] || '')
  }

  const loadPoints = async () => {
    if (!selected.trim()) return
    const requestID = ++latestRequest.current
    setLoading(true)
    setError(null)
    try {
      const response = await api.queryTimeSeries({
        series: selected.trim(),
        start: start ? new Date(start).toISOString() : undefined,
        end: end ? new Date(end).toISOString() : undefined,
        tags: parseJSONMap(tags, 'Tag filter') as Record<string, string>,
        limit,
        descending,
      })
      if (requestID === latestRequest.current) setPoints(response.points)
    } catch (err: any) {
      if (requestID === latestRequest.current) setError(err.message || 'Unable to load points')
    } finally {
      if (requestID === latestRequest.current) setLoading(false)
    }
  }

  useEffect(() => { void loadSeries().catch((err: any) => setError(err.message || 'Unable to load series')) }, [])
  useEffect(() => { if (selected) void loadPoints() }, [selected])

  const handleWrite = async (event: React.FormEvent) => {
    event.preventDefault()
    setError(null)
    setSuccess(null)
    try {
      if (!selected.trim()) throw new Error('Enter a series name first')
      const point: TimeSeriesPoint = {
        Timestamp: timestamp ? new Date(timestamp).toISOString() : new Date().toISOString(),
        Tags: parseJSONMap(writeTags, 'Tags') as Record<string, string>,
        Fields: parseJSONMap(fields, 'Fields'),
      }
      await api.writeTimeSeries(selected.trim(), point)
      setSuccess('Point written successfully.')
      await loadSeries()
      onRefreshCatalog()
      void loadPoints()
    } catch (err: any) { setError(err.message || 'Unable to write point') }
  }

  const handlePrune = async () => {
    setError(null)
    setSuccess(null)
    try {
      if (!selected.trim()) throw new Error('Enter a series name first')
      const result = await api.pruneTimeSeries(selected.trim(), new Date(retentionCutoff).toISOString())
      setSuccess(`Retention applied: removed ${result.removed} point${result.removed === 1 ? '' : 's'}.`)
      void loadPoints()
    } catch (err: any) { setError(err.message || 'Unable to apply retention') }
  }

  return (
    <div className="p-8 space-y-6 max-w-7xl mx-auto">
      <header className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 className="text-xl font-bold tracking-tight text-zinc-900 dark:text-white flex items-center gap-2.5"><Activity className="w-5 h-5 text-emerald-500" />Time-Series Explorer</h1>
          <p className="text-xs text-zinc-500 dark:text-zinc-400 mt-1">Inspect logs, metrics, and IoT telemetry by time range and exact tags.</p>
        </div>
        <button onClick={() => void loadSeries()} className="flex items-center gap-1.5 px-3 py-2 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 text-xs font-medium hover:bg-zinc-50 dark:hover:bg-zinc-800 transition cursor-pointer"><RefreshCw className="w-3.5 h-3.5" />Refresh series</button>
      </header>

      {error ? <div className="p-3 rounded-lg bg-rose-500/10 border border-rose-500/20 text-rose-700 dark:text-rose-400 text-xs flex gap-2"><AlertTriangle className="w-4 h-4 shrink-0" />{error}</div> : null}
      {success ? <div className="p-3 rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-emerald-700 dark:text-emerald-400 text-xs flex gap-2"><CheckCircle2 className="w-4 h-4 shrink-0" />{success}</div> : null}

      <section className="p-5 rounded-2xl border border-zinc-200 dark:border-zinc-800 bg-white/70 dark:bg-zinc-900/50 grid grid-cols-1 lg:grid-cols-[1fr_auto] gap-4">
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <label className="text-xs text-zinc-600 dark:text-zinc-300">Series name<input list="time-series" value={selected} onChange={(event) => setSelected(event.target.value)} placeholder="e.g. sensor-readings" className="mt-1.5 w-full px-3 py-2 rounded-lg bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 font-mono text-xs focus:outline-none focus:border-emerald-500" /><datalist id="time-series">{series.map((name) => <option key={name} value={name} />)}</datalist></label>
          <label className="text-xs text-zinc-600 dark:text-zinc-300">Result limit<input type="number" min="1" max="1000" value={limit} onChange={(event) => setLimit(Number(event.target.value) || 100)} className="mt-1.5 w-full px-3 py-2 rounded-lg bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 font-mono text-xs focus:outline-none focus:border-emerald-500" /></label>
          <label className="text-xs text-zinc-600 dark:text-zinc-300">Start<input type="datetime-local" value={start} onChange={(event) => setStart(event.target.value)} className="mt-1.5 w-full px-3 py-2 rounded-lg bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 text-xs focus:outline-none focus:border-emerald-500" /></label>
          <label className="text-xs text-zinc-600 dark:text-zinc-300">End<input type="datetime-local" value={end} onChange={(event) => setEnd(event.target.value)} className="mt-1.5 w-full px-3 py-2 rounded-lg bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 text-xs focus:outline-none focus:border-emerald-500" /></label>
        </div>
        <div className="flex lg:flex-col justify-end gap-2"><label className="flex items-center gap-2 text-xs text-zinc-600 dark:text-zinc-300"><input type="checkbox" checked={descending} onChange={(event) => setDescending(event.target.checked)} />Newest first</label><button onClick={() => void loadPoints()} disabled={loading || !selected.trim()} className="flex items-center justify-center gap-1.5 px-4 py-2 rounded-lg bg-emerald-500 hover:bg-emerald-400 disabled:opacity-50 text-zinc-950 text-xs font-semibold transition cursor-pointer"><Filter className="w-3.5 h-3.5" />{loading ? 'Loading…' : 'Run query'}</button></div>
      </section>

      <section className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div className="p-5 rounded-2xl border border-zinc-200 dark:border-zinc-800 bg-white/70 dark:bg-zinc-900/50 space-y-3"><h2 className="text-sm font-semibold flex gap-2 items-center"><Filter className="w-4 h-4 text-emerald-500" />Exact tag filter</h2><textarea value={tags} onChange={(event) => setTags(event.target.value)} rows={4} className="w-full p-3 rounded-xl bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 text-xs font-mono focus:outline-none focus:border-emerald-500" /><p className="text-[11px] text-zinc-500">JSON object, e.g. {`{"device":"sensor-01","severity":"error"}`}</p></div>
        <form onSubmit={handleWrite} className="p-5 rounded-2xl border border-zinc-200 dark:border-zinc-800 bg-white/70 dark:bg-zinc-900/50 space-y-3"><h2 className="text-sm font-semibold flex gap-2 items-center"><Plus className="w-4 h-4 text-emerald-500" />Ingest point</h2><label className="block text-xs text-zinc-600 dark:text-zinc-300">Timestamp<input type="datetime-local" value={timestamp} onChange={(event) => setTimestamp(event.target.value)} className="mt-1.5 w-full px-3 py-2 rounded-lg bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 text-xs focus:outline-none focus:border-emerald-500" /></label><div className="grid grid-cols-1 sm:grid-cols-2 gap-3"><label className="text-xs text-zinc-600 dark:text-zinc-300">Tags<textarea value={writeTags} onChange={(event) => setWriteTags(event.target.value)} rows={4} className="mt-1.5 w-full p-3 rounded-xl bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 text-xs font-mono focus:outline-none focus:border-emerald-500" /></label><label className="text-xs text-zinc-600 dark:text-zinc-300">Fields<textarea value={fields} onChange={(event) => setFields(event.target.value)} rows={4} className="mt-1.5 w-full p-3 rounded-xl bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 text-xs font-mono focus:outline-none focus:border-emerald-500" /></label></div><button className="flex items-center gap-1.5 px-3 py-2 rounded-lg bg-emerald-500 hover:bg-emerald-400 text-zinc-950 text-xs font-semibold transition cursor-pointer"><Plus className="w-3.5 h-3.5" />Write point</button></form>
      </section>

      <section className="rounded-2xl border border-zinc-200 dark:border-zinc-800 bg-white/70 dark:bg-zinc-900/50 overflow-hidden"><div className="p-4 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between text-xs"><span className="font-semibold">{points.length} points loaded</span><span className="font-mono text-zinc-400">timestamp · tags · fields</span></div><div className="overflow-x-auto"><table className="w-full text-left text-xs"><thead className="bg-zinc-50 dark:bg-zinc-950/60 text-zinc-500 uppercase text-[10px]"><tr><th className="p-3 font-mono">Timestamp</th><th className="p-3">Tags</th><th className="p-3">Fields</th></tr></thead><tbody className="divide-y divide-zinc-200 dark:divide-zinc-800/50">{points.length === 0 ? <tr><td colSpan={3} className="p-8 text-center text-zinc-500">No points found in this range.</td></tr> : points.map((point, index) => <tr key={`${point.Timestamp}-${index}`} className="hover:bg-zinc-50 dark:hover:bg-zinc-800/30"><td className="p-3 font-mono whitespace-nowrap text-emerald-700 dark:text-emerald-400">{new Date(point.Timestamp).toLocaleString()}</td><td className="p-3 font-mono text-zinc-600 dark:text-zinc-300">{JSON.stringify(point.Tags)}</td><td className="p-3 font-mono text-zinc-600 dark:text-zinc-300">{JSON.stringify(point.Fields)}</td></tr>)}</tbody></table></div></section>

      <section className="p-5 rounded-2xl border border-amber-500/20 bg-amber-500/5 flex flex-wrap items-end gap-3"><div className="flex-1 min-w-56"><h2 className="text-sm font-semibold flex gap-2 items-center"><Clock3 className="w-4 h-4 text-amber-500" />Retention policy</h2><p className="text-[11px] text-zinc-500 mt-1">Permanently remove points older than this cutoff.</p><input type="datetime-local" value={retentionCutoff} onChange={(event) => setRetentionCutoff(event.target.value)} className="mt-3 w-full px-3 py-2 rounded-lg bg-white dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 text-xs focus:outline-none focus:border-amber-500" /></div><button onClick={() => void handlePrune()} disabled={!selected.trim()} className="flex items-center gap-1.5 px-3 py-2 rounded-lg bg-amber-500 hover:bg-amber-400 disabled:opacity-50 text-zinc-950 text-xs font-semibold transition cursor-pointer"><Trash2 className="w-3.5 h-3.5" />Apply retention</button></section>
    </div>
  )
}
