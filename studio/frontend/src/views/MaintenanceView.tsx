import React, { useState } from 'react'
import {
  ShieldCheck,
  FileCheck2,
  Download,
  Zap,
  CheckCircle2,
  AlertTriangle,
  AlertCircle,
  RefreshCw,
} from 'lucide-react'
import { api } from '../api'
import type { IntegrityReport } from '../types'

export const MaintenanceView: React.FC = () => {
  const [report, setReport] = useState<IntegrityReport | null>(null)
  const [loadingCheck, setLoadingCheck] = useState(false)
  const [loadingCheckpoint, setLoadingCheckpoint] = useState(false)
  const [loadingBackup, setLoadingBackup] = useState(false)
  const [statusMsg, setStatusMsg] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const handleRunCheck = async () => {
    setError(null)
    setStatusMsg(null)
    setLoadingCheck(true)
    try {
      const res = await api.checkIntegrity()
      setReport({
        valid: res.valid,
        pages_checked: res.pages_checked,
        errors: res.errors || [],
        warnings: res.warnings || [],
      })
    } catch (err: any) {
      setError(err.message || 'Integrity check failed')
    } finally {
      setLoadingCheck(false)
    }
  }

  const handleCheckpoint = async () => {
    setError(null)
    setStatusMsg(null)
    setLoadingCheckpoint(true)
    try {
      const res = await api.checkpointWAL()
      setStatusMsg(`WAL checkpoint completed successfully. LSN synced to #${res.last_lsn}`)
    } catch (err: any) {
      setError(err.message || 'WAL Checkpoint failed')
    } finally {
      setLoadingCheckpoint(false)
    }
  }

  const handleBackup = async () => {
    setError(null)
    setStatusMsg(null)
    setLoadingBackup(true)
    try {
      const res = await api.triggerBackup()
      setStatusMsg(`Snapshot backup saved to: ${res.backup_path}`)
    } catch (err: any) {
      setError(err.message || 'Backup failed')
    } finally {
      setLoadingBackup(false)
    }
  }

  const errors = report?.errors || []
  const warnings = report?.warnings || []

  return (
    <div className="p-8 space-y-8 max-w-7xl mx-auto">
      {/* Header */}
      <div>
        <h1 className="text-xl font-bold tracking-tight text-zinc-900 dark:text-white flex items-center gap-2.5">
          <ShieldCheck className="w-5 h-5 text-emerald-600 dark:text-emerald-400" />
          <span>Integrity Verification & Storage Tools</span>
        </h1>
        <p className="text-xs text-zinc-500 dark:text-zinc-400 mt-1">
          Perform storage kernel integrity diagnostics, force WAL checkpoints, and create snapshots
        </p>
      </div>

      {statusMsg && (
        <div className="p-3.5 rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-emerald-700 dark:text-emerald-400 text-xs flex items-center gap-2">
          <CheckCircle2 className="w-4 h-4 shrink-0" />
          <span>{statusMsg}</span>
        </div>
      )}

      {error && (
        <div className="p-3.5 rounded-lg bg-red-500/10 border border-red-500/20 text-red-600 dark:text-red-400 text-xs flex items-center gap-2">
          <AlertCircle className="w-4 h-4 shrink-0" />
          <span>{error}</span>
        </div>
      )}

      {/* Action Cards Grid */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        {/* Tool 1: Integrity Check */}
        <div className="p-6 rounded-xl bg-white dark:bg-zinc-900/70 border border-zinc-200 dark:border-zinc-800/80 shadow-xs space-y-4 flex flex-col justify-between">
          <div className="space-y-2">
            <div className="w-10 h-10 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-600 dark:text-emerald-400">
              <FileCheck2 className="w-5 h-5" />
            </div>
            <h2 className="font-semibold text-sm text-zinc-900 dark:text-white">Kernel Integrity Check</h2>
            <p className="text-xs text-zinc-500 dark:text-zinc-400 leading-relaxed">
              Scans all allocated slotted pages, checks CRC32C Castagnoli hardware checksums, validates B+Tree pointers, and ensures record directory consistency.
            </p>
          </div>

          <button
            onClick={handleRunCheck}
            disabled={loadingCheck}
            className="w-full flex items-center justify-center gap-2 px-4 py-2.5 rounded-lg bg-emerald-500 hover:bg-emerald-400 text-zinc-950 text-xs font-semibold shadow-md shadow-emerald-500/20 transition cursor-pointer disabled:opacity-50"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loadingCheck ? 'animate-spin' : ''}`} />
            <span>{loadingCheck ? 'Verifying...' : 'Run Integrity Check'}</span>
          </button>
        </div>

        {/* Tool 2: Force WAL Checkpoint */}
        <div className="p-6 rounded-xl bg-white dark:bg-zinc-900/70 border border-zinc-200 dark:border-zinc-800/80 shadow-xs space-y-4 flex flex-col justify-between">
          <div className="space-y-2">
            <div className="w-10 h-10 rounded-lg bg-teal-500/10 border border-teal-500/20 flex items-center justify-center text-teal-600 dark:text-teal-400">
              <Zap className="w-5 h-5" />
            </div>
            <h2 className="font-semibold text-sm text-zinc-900 dark:text-white">Force WAL Checkpoint</h2>
            <p className="text-xs text-zinc-500 dark:text-zinc-400 leading-relaxed">
              Synchronously flushes all dirty pages from the 16-partition LRU cache to disk, advances the Meta LSN checkpoint watermark, and truncates the WAL file.
            </p>
          </div>

          <button
            onClick={handleCheckpoint}
            disabled={loadingCheckpoint}
            className="w-full flex items-center justify-center gap-2 px-4 py-2.5 rounded-lg bg-zinc-100 hover:bg-zinc-200 dark:bg-zinc-800 dark:hover:bg-zinc-700 text-zinc-800 dark:text-zinc-200 text-xs font-medium transition cursor-pointer disabled:opacity-50"
          >
            <span>{loadingCheckpoint ? 'Checkpointing...' : 'Flush & Checkpoint'}</span>
          </button>
        </div>

        {/* Tool 3: Snapshot Backup */}
        <div className="p-6 rounded-xl bg-white dark:bg-zinc-900/70 border border-zinc-200 dark:border-zinc-800/80 shadow-xs space-y-4 flex flex-col justify-between">
          <div className="space-y-2">
            <div className="w-10 h-10 rounded-lg bg-sky-500/10 border border-sky-500/20 flex items-center justify-center text-sky-600 dark:text-sky-400">
              <Download className="w-5 h-5" />
            </div>
            <h2 className="font-semibold text-sm text-zinc-900 dark:text-white">Point-In-Time Backup</h2>
            <p className="text-xs text-zinc-500 dark:text-zinc-400 leading-relaxed">
              Creates a consistent, standalone copy of the single-file storage kernel while transactions continue executing.
            </p>
          </div>

          <button
            onClick={handleBackup}
            disabled={loadingBackup}
            className="w-full flex items-center justify-center gap-2 px-4 py-2.5 rounded-lg bg-zinc-100 hover:bg-zinc-200 dark:bg-zinc-800 dark:hover:bg-zinc-700 text-zinc-800 dark:text-zinc-200 text-xs font-medium transition cursor-pointer disabled:opacity-50"
          >
            <span>{loadingBackup ? 'Copying...' : 'Create Backup Snapshot'}</span>
          </button>
        </div>
      </div>

      {/* Integrity Report Display */}
      {report && (
        <div className="p-6 rounded-xl bg-white dark:bg-zinc-900/70 border border-zinc-200 dark:border-zinc-800/80 shadow-xs space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              {report.valid ? (
                <CheckCircle2 className="w-5 h-5 text-emerald-600 dark:text-emerald-400" />
              ) : (
                <AlertTriangle className="w-5 h-5 text-red-600 dark:text-red-400" />
              )}
              <h2 className="text-sm font-semibold text-zinc-900 dark:text-white">
                Integrity Report: {report.valid ? 'All Structures Healthy' : 'Corruption Detected'}
              </h2>
            </div>
            <span className="text-xs font-mono px-2.5 py-1 rounded bg-zinc-100 dark:bg-zinc-800 text-zinc-700 dark:text-zinc-300 border border-zinc-200 dark:border-zinc-700/50">
              {report.pages_checked} Pages Validated
            </span>
          </div>

          {errors.length > 0 && (
            <div className="space-y-2">
              <span className="text-xs font-semibold text-red-600 dark:text-red-400">Errors:</span>
              <ul className="list-disc list-inside text-xs text-red-700 dark:text-red-300 space-y-1 font-mono">
                {errors.map((e, idx) => (
                  <li key={idx}>{e}</li>
                ))}
              </ul>
            </div>
          )}

          {warnings.length > 0 && (
            <div className="space-y-2">
              <span className="text-xs font-semibold text-amber-600 dark:text-amber-400">Warnings:</span>
              <ul className="list-disc list-inside text-xs text-amber-700 dark:text-amber-300 space-y-1 font-mono">
                {warnings.map((w, idx) => (
                  <li key={idx}>{w}</li>
                ))}
              </ul>
            </div>
          )}

          {errors.length === 0 && (
            <div className="p-4 rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-xs text-emerald-800 dark:text-emerald-300 font-mono">
              ✓ Meta A / Meta B generation headers valid.<br />
              ✓ All page checksums passed hardware CRC32C verification.<br />
              ✓ B+Tree internal and leaf nodes consistent.<br />
              ✓ Record directory version chains consistent.
            </div>
          )}
        </div>
      )}
    </div>
  )
}
