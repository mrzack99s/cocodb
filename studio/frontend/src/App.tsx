import React, { useState, useEffect } from 'react'
import { Sidebar, type ViewType } from './components/Sidebar'
import { Header } from './components/Header'
import { ErrorBoundary } from './components/ErrorBoundary'
import { DashboardView } from './views/DashboardView'
import { CollectionsView } from './views/CollectionsView'
import { BucketsView } from './views/BucketsView'
import { QueueView } from './views/QueueView'
import { PubSubView } from './views/PubSubView'
import { QueryConsoleView } from './views/QueryConsoleView'
import { VectorPlaygroundView } from './views/VectorPlaygroundView'
import { SearchPlaygroundView } from './views/SearchPlaygroundView'
import { MaintenanceView } from './views/MaintenanceView'
import { api } from './api'
import type { DBStats, CatalogData } from './types'

export const App: React.FC = () => {
  const [currentView, setCurrentView] = useState<ViewType>('dashboard')
  const [stats, setStats] = useState<DBStats | null>(null)
  const [catalog, setCatalog] = useState<CatalogData | null>(null)
  const [loading, setLoading] = useState(false)

  // Theme state: 'light' | 'dark'
  const [theme, setTheme] = useState<'light' | 'dark'>(() => {
    const saved = localStorage.getItem('coco_theme')
    if (saved === 'light' || saved === 'dark') return saved
    return 'dark'
  })

  useEffect(() => {
    const root = document.documentElement
    if (theme === 'dark') {
      root.classList.add('dark')
    } else {
      root.classList.remove('dark')
    }
    localStorage.setItem('coco_theme', theme)
  }, [theme])

  const toggleTheme = () => {
    setTheme((prev) => (prev === 'dark' ? 'light' : 'dark'))
  }

  const fetchData = async () => {
    setLoading(true)
    try {
      const [statsRes, catalogRes] = await Promise.all([
        api.getStats().catch(() => null),
        api.getCatalog().catch(() => ({ buckets: [], collections: [], queues: [] })),
      ])
      if (statsRes) setStats(statsRes)
      if (catalogRes) setCatalog(catalogRes)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchData()
    const interval = setInterval(fetchData, 5000)
    return () => clearInterval(interval)
  }, [])

  return (
    <div className="flex h-screen bg-zinc-50 dark:bg-zinc-950 text-zinc-900 dark:text-zinc-100 overflow-hidden font-sans transition-colors">
      {/* Sidebar */}
      <Sidebar
        currentView={currentView}
        onSelectView={setCurrentView}
        bucketCount={catalog?.buckets?.length || 0}
        collectionCount={catalog?.collections?.length || 0}
        queueCount={catalog?.queues?.length || 0}
      />

      {/* Main Content Area */}
      <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
        <Header
          stats={stats}
          loading={loading}
          theme={theme}
          onToggleTheme={toggleTheme}
          onRefresh={fetchData}
        />

        <main className="flex-1 overflow-y-auto bg-gradient-to-b from-zinc-100/50 to-zinc-50 dark:from-zinc-900/20 dark:to-zinc-950 transition-colors">
          <ErrorBoundary>
            {currentView === 'dashboard' && (
              <DashboardView
                stats={stats}
                catalog={catalog}
                onNavigate={setCurrentView}
              />
            )}
            {currentView === 'collections' && (
              <CollectionsView
                catalog={catalog}
                onRefreshCatalog={fetchData}
              />
            )}
            {currentView === 'buckets' && (
              <BucketsView
                catalog={catalog}
                onRefreshCatalog={fetchData}
              />
            )}
            {currentView === 'queues' && (
              <QueueView />
            )}
            {currentView === 'pubsub' && (
              <PubSubView />
            )}
            {currentView === 'query' && (
              <QueryConsoleView catalog={catalog} />
            )}
            {currentView === 'vector' && (
              <VectorPlaygroundView catalog={catalog} />
            )}
            {currentView === 'search' && (
              <SearchPlaygroundView catalog={catalog} />
            )}
            {currentView === 'maintenance' && <MaintenanceView />}
          </ErrorBoundary>
        </main>
      </div>
    </div>
  )
}
