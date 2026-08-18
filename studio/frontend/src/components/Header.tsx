import React from 'react'
import { Activity, RefreshCw, HardDrive, Cpu, ShieldCheck, Sun, Moon } from 'lucide-react'
import type { DBStats } from '../types'

interface HeaderProps {
  stats: DBStats | null
  loading: boolean
  theme: 'light' | 'dark'
  onToggleTheme: () => void
  onRefresh: () => void
}

export const Header: React.FC<HeaderProps> = ({
  stats,
  loading,
  theme,
  onToggleTheme,
  onRefresh,
}) => {
  return (
    <header className="h-14 border-b border-zinc-200 dark:border-zinc-800/80 bg-white/80 dark:bg-zinc-900/40 backdrop-blur-md px-6 flex items-center justify-between transition-colors">
      {/* Left title / status */}
      <div className="flex items-center gap-4">
        <div className="flex items-center gap-2">
          <div className="w-2.5 h-2.5 rounded-full bg-emerald-500 shadow-sm shadow-emerald-500/50 animate-pulse"></div>
          <span className="text-xs font-semibold text-zinc-800 dark:text-zinc-300">
            CoCo Embedded Instance
          </span>
        </div>
        {stats && (
          <div className="flex items-center gap-3 text-[11px] text-zinc-500 dark:text-zinc-400 font-mono border-l border-zinc-200 dark:border-zinc-800 pl-4">
            <span className="flex items-center gap-1">
              <HardDrive className="w-3.5 h-3.5 text-zinc-400 dark:text-zinc-400" />
              {stats.page_count} Pages ({(stats.page_count * 16) / 1024} MB)
            </span>
            <span className="flex items-center gap-1">
              <Cpu className="w-3.5 h-3.5 text-zinc-400 dark:text-zinc-400" />
              Hit Rate: {(stats.cache_hit_rate * 100).toFixed(1)}%
            </span>
            <span className="flex items-center gap-1">
              <Activity className="w-3.5 h-3.5 text-zinc-400 dark:text-zinc-400" />
              LSN: #{stats.last_lsn}
            </span>
          </div>
        )}
      </div>

      {/* Right actions */}
      <div className="flex items-center gap-2">
        {/* Theme Switcher Button */}
        <button
          onClick={onToggleTheme}
          className="p-1.5 rounded-lg bg-zinc-100 hover:bg-zinc-200 dark:bg-zinc-800 dark:hover:bg-zinc-700/80 border border-zinc-200 dark:border-zinc-700/50 text-zinc-600 dark:text-zinc-300 text-xs font-medium transition cursor-pointer"
          title={`Switch to ${theme === 'dark' ? 'Light' : 'Dark'} mode`}
          aria-label="Toggle theme"
        >
          {theme === 'dark' ? (
            <Sun className="w-4 h-4 text-amber-400" />
          ) : (
            <Moon className="w-4 h-4 text-indigo-600" />
          )}
        </button>

        {/* Refresh Button */}
        <button
          onClick={onRefresh}
          disabled={loading}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-zinc-100 hover:bg-zinc-200 dark:bg-zinc-800 dark:hover:bg-zinc-700/80 border border-zinc-200 dark:border-zinc-700/50 text-zinc-700 dark:text-zinc-300 text-xs font-medium transition disabled:opacity-50 cursor-pointer"
        >
          <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin text-emerald-500' : ''}`} />
          <span>Refresh</span>
        </button>

        <div className="flex items-center gap-1 px-2.5 py-1 rounded-md bg-emerald-500/10 border border-emerald-500/20 text-emerald-600 dark:text-emerald-400 text-[11px] font-mono">
          <ShieldCheck className="w-3.5 h-3.5" />
          <span>ACID OK</span>
        </div>
      </div>
    </header>
  )
}
