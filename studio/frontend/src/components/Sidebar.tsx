import React from 'react'
import {
  LayoutDashboard,
  Layers,
  Database,
  Terminal,
  BrainCircuit,
  Search,
  ShieldCheck,
  Zap,
  Package,
  Radio,
} from 'lucide-react'

export type ViewType =
  | 'dashboard'
  | 'collections'
  | 'buckets'
  | 'queues'
  | 'pubsub'
  | 'query'
  | 'vector'
  | 'search'
  | 'maintenance'

interface SidebarProps {
  currentView: ViewType
  onSelectView: (view: ViewType) => void
  bucketCount: number
  collectionCount: number
  queueCount?: number
}

export const Sidebar: React.FC<SidebarProps> = ({
  currentView,
  onSelectView,
  bucketCount,
  collectionCount,
  queueCount = 0,
}) => {
  const navItems: {
    id: ViewType
    label: string
    icon: React.ComponentType<{ className?: string }>
    badge?: number
  }[] = [
    { id: 'dashboard', label: 'Dashboard', icon: LayoutDashboard },
    { id: 'collections', label: 'Collections', icon: Layers, badge: collectionCount },
    { id: 'buckets', label: 'KV Buckets', icon: Database, badge: bucketCount },
    { id: 'queues', label: 'Task Queues', icon: Package, badge: queueCount },
    { id: 'pubsub', label: 'Real-Time Pub/Sub', icon: Radio },
    { id: 'query', label: 'Query Console', icon: Terminal },
    { id: 'vector', label: 'Vector Search', icon: BrainCircuit },
    { id: 'search', label: 'Full-Text Search', icon: Search },
    { id: 'maintenance', label: 'Integrity & Tools', icon: ShieldCheck },
  ]

  return (
    <aside className="w-64 bg-zinc-50/90 dark:bg-zinc-900/60 border-r border-zinc-200 dark:border-zinc-800/80 flex flex-col h-screen select-none backdrop-blur-md transition-colors">
      {/* Brand Header */}
      <div className="p-5 border-b border-zinc-200 dark:border-zinc-800/80 flex items-center gap-3">
        <div className="w-9 h-9 rounded-xl bg-gradient-to-tr from-emerald-500 to-teal-400 flex items-center justify-center shadow-lg shadow-emerald-500/20">
          <Zap className="w-5 h-5 text-zinc-950 stroke-[2.5]" />
        </div>
        <div>
          <div className="flex items-center gap-1.5">
            <span className="font-bold text-base tracking-tight bg-gradient-to-r from-zinc-900 via-zinc-700 to-zinc-500 dark:from-white dark:via-zinc-200 dark:to-zinc-400 bg-clip-text text-transparent">
              CoCo DB
            </span>
            <span className="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded-full bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20 font-semibold">
              Studio
            </span>
          </div>
          <p className="text-xs text-zinc-500 dark:text-zinc-500">Multi-Model Engine</p>
        </div>
      </div>

      {/* Navigation */}
      <nav className="flex-1 p-3 space-y-1 overflow-y-auto">
        <div className="px-3 py-2 text-[11px] font-semibold tracking-wider uppercase text-zinc-400 dark:text-zinc-500">
          Database Explorer
        </div>
        {navItems.map((item) => {
          const Icon = item.icon
          const isActive = currentView === item.id
          return (
            <button
              key={item.id}
              onClick={() => onSelectView(item.id)}
              className={`w-full flex items-center justify-between px-3 py-2.5 rounded-lg text-xs font-medium transition-all cursor-pointer ${
                isActive
                  ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400 shadow-xs border border-emerald-500/30 font-semibold'
                  : 'text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-200 hover:bg-zinc-200/50 dark:hover:bg-zinc-800/50 border border-transparent'
              }`}
            >
              <div className="flex items-center gap-2.5">
                <Icon className={`w-4 h-4 ${isActive ? 'text-emerald-600 dark:text-emerald-400' : 'text-zinc-400 dark:text-zinc-400'}`} />
                <span>{item.label}</span>
              </div>
              {item.badge !== undefined && (
                <span
                  className={`text-[10px] px-1.5 py-0.5 rounded-md font-mono ${
                    isActive
                      ? 'bg-emerald-500/20 text-emerald-700 dark:text-emerald-300'
                      : 'bg-zinc-200 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400'
                  }`}
                >
                  {item.badge}
                </span>
              )}
            </button>
          )
        })}
      </nav>

      {/* Footer Info */}
      <div className="p-4 border-t border-zinc-200 dark:border-zinc-800/80 bg-zinc-100/50 dark:bg-zinc-950/40">
        <div className="flex items-center justify-between text-[11px] text-zinc-500">
          <span className="flex items-center gap-1.5">
            <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
            Storage Kernel Active
          </span>
          <span className="font-mono text-[10px] text-zinc-400 dark:text-zinc-600">v1.0 (Pure Go)</span>
        </div>
      </div>
    </aside>
  )
}
