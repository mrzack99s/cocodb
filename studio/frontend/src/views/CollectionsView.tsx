import React, { useState, useEffect } from 'react'
import {
  Layers,
  Plus,
  Trash2,
  Edit,
  Search,
  Filter,
  Eye,
  AlertCircle,
} from 'lucide-react'
import { api } from '../api'
import type { CatalogData } from '../types'

interface CollectionsViewProps {
  catalog: CatalogData | null
  onRefreshCatalog: () => void
}

export const CollectionsView: React.FC<CollectionsViewProps> = ({
  catalog,
  onRefreshCatalog,
}) => {
  const collections = catalog?.collections || []
  const [selectedCollection, setSelectedCollection] = useState<string>('')
  const [documents, setDocuments] = useState<Record<string, any>[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Filter state
  const [filterField, setFilterField] = useState('')
  const [filterOp, setFilterOp] = useState('eq')
  const [filterVal, setFilterVal] = useState('')

  // View / Edit Modal
  const [viewDoc, setViewDoc] = useState<Record<string, any> | null>(null)
  const [isModalOpen, setIsModalOpen] = useState(false)
  const [jsonInput, setJsonInput] = useState('')
  const [modalMode, setModalMode] = useState<'create' | 'edit'>('create')

  useEffect(() => {
    if (collections.length > 0 && !selectedCollection) {
      setSelectedCollection(collections[0].name)
    }
  }, [collections, selectedCollection])

  useEffect(() => {
    if (selectedCollection) {
      loadDocuments()
    }
  }, [selectedCollection])

  const loadDocuments = async () => {
    if (!selectedCollection) return
    setLoading(true)
    setError(null)
    try {
      const filters = []
      if (filterField.trim()) {
        let val: any = filterVal
        if (filterVal === 'true') val = true
        else if (filterVal === 'false') val = false
        else if (!isNaN(Number(filterVal)) && filterVal.trim() !== '') val = Number(filterVal)
        filters.push({ field: filterField.trim(), op: filterOp, value: val })
      }

      const res = await api.queryDocuments(selectedCollection, {
        filters: filters.length > 0 ? filters : undefined,
        limit: 50,
      })
      setDocuments(res.documents || [])
    } catch (err: any) {
      setError(err.message || 'Failed to load documents')
    } finally {
      setLoading(false)
    }
  }

  const handleOpenCreate = () => {
    setModalMode('create')
    setJsonInput('{\n  "name": "New Document",\n  "active": true\n}')
    setIsModalOpen(true)
  }

  const handleOpenEdit = (doc: Record<string, any>) => {
    setModalMode('edit')
    setJsonInput(JSON.stringify(doc, null, 2))
    setIsModalOpen(true)
  }

  const handleSaveDocument = async () => {
    setError(null)
    try {
      const parsed = JSON.parse(jsonInput)
      if (modalMode === 'create') {
        await api.insertDocument(selectedCollection, parsed)
      } else {
        const id = parsed._id
        if (!id) throw new Error('Document must have an _id for updating')
        await api.updateDocument(selectedCollection, String(id), parsed)
      }
      setIsModalOpen(false)
      loadDocuments()
      onRefreshCatalog()
    } catch (err: any) {
      setError(err.message || 'Invalid JSON or Save Failed')
    }
  }

  const handleDelete = async (id: string) => {
    if (!confirm(`Are you sure you want to delete document "${id}"?`)) return
    try {
      await api.deleteDocument(selectedCollection, id)
      loadDocuments()
    } catch (err: any) {
      setError(err.message || 'Failed to delete document')
    }
  }

  // Extract all distinct column keys for table view
  const allKeys = Array.from(
    new Set(documents.flatMap((doc) => Object.keys(doc)))
  ).slice(0, 6)

  return (
    <div className="p-8 space-y-6 max-w-7xl mx-auto">
      {/* View Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold tracking-tight text-zinc-900 dark:text-white flex items-center gap-2.5">
            <Layers className="w-5 h-5 text-emerald-600 dark:text-emerald-400" />
            <span>Document Collections</span>
          </h1>
          <p className="text-xs text-zinc-500 dark:text-zinc-400 mt-1">
            Browse and query CSON-encoded NoSQL collections with schema validation
          </p>
        </div>

        {selectedCollection && (
          <button
            onClick={handleOpenCreate}
            className="flex items-center gap-1.5 px-3 py-2 rounded-lg bg-emerald-500 hover:bg-emerald-400 text-zinc-950 text-xs font-semibold shadow-md shadow-emerald-500/20 transition cursor-pointer"
          >
            <Plus className="w-4 h-4 stroke-[2.5]" />
            <span>Insert Document</span>
          </button>
        )}
      </div>

      {/* Collection Tabs / Selector */}
      <div className="flex items-center gap-2 border-b border-zinc-200 dark:border-zinc-800 pb-3 overflow-x-auto">
        {collections.length === 0 ? (
          <span className="text-xs text-zinc-400">No collections found</span>
        ) : (
          collections.map((c) => (
            <button
              key={c.name}
              onClick={() => setSelectedCollection(c.name)}
              className={`px-3 py-1.5 rounded-lg text-xs font-medium transition cursor-pointer ${
                selectedCollection === c.name
                  ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400 border border-emerald-500/30 font-semibold'
                  : 'text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-200 hover:bg-zinc-200/50 dark:hover:bg-zinc-800/50 border border-transparent'
              }`}
            >
              {c.name}
            </button>
          ))
        )}
      </div>

      {/* Filter / Query Bar */}
      <div className="p-3.5 rounded-xl bg-white dark:bg-zinc-900/60 border border-zinc-200 dark:border-zinc-800/80 shadow-xs flex flex-wrap items-center gap-3">
        <div className="flex items-center gap-2 text-xs text-zinc-600 dark:text-zinc-400">
          <Filter className="w-3.5 h-3.5" />
          <span>Filter:</span>
        </div>

        <input
          type="text"
          placeholder="Field name (e.g. status, age)"
          value={filterField}
          onChange={(e) => setFilterField(e.target.value)}
          className="bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg px-3 py-1.5 text-xs text-zinc-900 dark:text-zinc-200 placeholder-zinc-400 dark:placeholder-zinc-600 focus:outline-none focus:border-emerald-500/50"
        />

        <select
          value={filterOp}
          onChange={(e) => setFilterOp(e.target.value)}
          className="bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg px-3 py-1.5 text-xs text-zinc-800 dark:text-zinc-300 focus:outline-none focus:border-emerald-500/50"
        >
          <option value="eq">== (Equals)</option>
          <option value="ne">!= (Not Equals)</option>
          <option value="gt">&gt; (Greater Than)</option>
          <option value="gte">&gt;= (Greater or Equal)</option>
          <option value="lt">&lt; (Less Than)</option>
          <option value="lte">&lt;= (Less or Equal)</option>
          <option value="contains">Contains (Substring)</option>
        </select>

        <input
          type="text"
          placeholder="Value (e.g. active, 25)"
          value={filterVal}
          onChange={(e) => setFilterVal(e.target.value)}
          className="bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg px-3 py-1.5 text-xs text-zinc-900 dark:text-zinc-200 placeholder-zinc-400 dark:placeholder-zinc-600 focus:outline-none focus:border-emerald-500/50"
        />

        <button
          onClick={loadDocuments}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-zinc-200 hover:bg-zinc-300 dark:bg-zinc-800 dark:hover:bg-zinc-700 text-zinc-800 dark:text-zinc-200 text-xs font-medium transition cursor-pointer"
        >
          <Search className="w-3.5 h-3.5" />
          <span>Apply Filter</span>
        </button>

        {filterField && (
          <button
            onClick={() => {
              setFilterField('')
              setFilterVal('')
              setTimeout(loadDocuments, 0)
            }}
            className="text-xs text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300 transition cursor-pointer"
          >
            Clear
          </button>
        )}
      </div>

      {error && (
        <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-red-600 dark:text-red-400 text-xs flex items-center gap-2">
          <AlertCircle className="w-4 h-4 shrink-0" />
          <span>{error}</span>
        </div>
      )}

      {/* Documents Table */}
      <div className="rounded-xl bg-white dark:bg-zinc-900/70 border border-zinc-200 dark:border-zinc-800/80 shadow-xs overflow-hidden">
        <div className="p-4 border-b border-zinc-200 dark:border-zinc-800/80 flex items-center justify-between text-xs text-zinc-500 dark:text-zinc-400 font-medium bg-zinc-50/50 dark:bg-transparent">
          <span>{documents.length} Documents Loaded</span>
          <span className="font-mono text-[11px] text-zinc-400 dark:text-zinc-500">CSON Zero-Copy Projection</span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-zinc-50 dark:bg-zinc-950/60 border-b border-zinc-200 dark:border-zinc-800 text-zinc-500 dark:text-zinc-400 uppercase tracking-wider text-[10px]">
              <tr>
                {allKeys.map((k) => (
                  <th key={k} className="p-3.5 font-mono">
                    {k}
                  </th>
                ))}
                <th className="p-3.5 text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-200 dark:divide-zinc-800/50 font-mono">
              {loading ? (
                <tr>
                  <td colSpan={allKeys.length + 1} className="p-8 text-center text-zinc-500">
                    Loading collection documents...
                  </td>
                </tr>
              ) : documents.length === 0 ? (
                <tr>
                  <td colSpan={allKeys.length + 1} className="p-8 text-center text-zinc-500">
                    No documents found.
                  </td>
                </tr>
              ) : (
                documents.map((doc, idx) => (
                  <tr key={doc._id || idx} className="hover:bg-zinc-50 dark:hover:bg-zinc-800/30 transition">
                    {allKeys.map((k) => {
                      const val = doc[k]
                      let displayVal = String(val)
                      if (typeof val === 'object' && val !== null) {
                        displayVal = JSON.stringify(val)
                      }
                      return (
                        <td
                          key={k}
                          className="p-3.5 text-zinc-700 dark:text-zinc-300 max-w-[200px] truncate"
                          title={displayVal}
                        >
                          {k === '_id' ? (
                            <span className="text-emerald-600 dark:text-emerald-400 font-semibold">{displayVal}</span>
                          ) : (
                            displayVal
                          )}
                        </td>
                      )
                    })}
                    <td className="p-3.5 text-right space-x-1">
                      <button
                        onClick={() => setViewDoc(doc)}
                        className="p-1.5 rounded hover:bg-zinc-100 dark:hover:bg-zinc-800 text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-200 transition cursor-pointer"
                        title="View Full JSON"
                      >
                        <Eye className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() => handleOpenEdit(doc)}
                        className="p-1.5 rounded hover:bg-zinc-100 dark:hover:bg-zinc-800 text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-200 transition cursor-pointer"
                        title="Edit Document"
                      >
                        <Edit className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() => handleDelete(String(doc._id))}
                        className="p-1.5 rounded hover:bg-red-50 dark:hover:bg-zinc-800 text-red-600 dark:text-red-400 hover:text-red-700 dark:hover:text-red-300 transition cursor-pointer"
                        title="Delete Document"
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

      {/* JSON Viewer Modal */}
      {viewDoc && (
        <div className="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4 z-50">
          <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-xl p-6 max-w-2xl w-full space-y-4 shadow-2xl">
            <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 pb-3">
              <h3 className="text-sm font-semibold text-zinc-900 dark:text-white font-mono">
                Document: {viewDoc._id}
              </h3>
              <button
                onClick={() => setViewDoc(null)}
                className="text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300 text-xs cursor-pointer"
              >
                Close
              </button>
            </div>
            <pre className="p-4 rounded-lg bg-zinc-950 border border-zinc-800 text-xs font-mono text-emerald-400 overflow-auto max-h-96">
              {JSON.stringify(viewDoc, null, 2)}
            </pre>
          </div>
        </div>
      )}

      {/* Create / Edit Modal */}
      {isModalOpen && (
        <div className="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4 z-50">
          <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-xl p-6 max-w-2xl w-full space-y-4 shadow-2xl">
            <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 pb-3">
              <h3 className="text-sm font-semibold text-zinc-900 dark:text-white">
                {modalMode === 'create' ? 'Insert New Document' : 'Edit Document'}
              </h3>
              <button
                onClick={() => setIsModalOpen(false)}
                className="text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300 text-xs cursor-pointer"
              >
                Cancel
              </button>
            </div>

            <div className="space-y-2">
              <label className="text-xs text-zinc-600 dark:text-zinc-400">JSON Payload</label>
              <textarea
                value={jsonInput}
                onChange={(e) => setJsonInput(e.target.value)}
                rows={12}
                className="w-full bg-zinc-950 border border-zinc-800 rounded-lg p-3 text-xs font-mono text-zinc-200 focus:outline-none focus:border-emerald-500/50"
              />
            </div>

            <div className="flex justify-end gap-2 pt-2">
              <button
                onClick={() => setIsModalOpen(false)}
                className="px-4 py-2 rounded-lg bg-zinc-100 hover:bg-zinc-200 dark:bg-zinc-800 dark:hover:bg-zinc-700 text-zinc-700 dark:text-zinc-300 text-xs font-medium cursor-pointer"
              >
                Cancel
              </button>
              <button
                onClick={handleSaveDocument}
                className="px-4 py-2 rounded-lg bg-emerald-500 hover:bg-emerald-400 text-zinc-950 text-xs font-semibold cursor-pointer"
              >
                Save Document
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
