import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import type { Pipeline, PipelineRun } from '../types/api'
import StatusBadge from '../components/StatusBadge'
import TimeAgo from '../components/TimeAgo'
import RetryButton from '../components/RetryButton'
import StopButton from '../components/StopButton'
import DeleteButton from '../components/DeleteButton'

const TERMINAL = new Set(['Succeeded', 'Failed', 'Stopped', 'Deleting'])
const PAGE_SIZE = 20

export default function PipelineDetail() {
  const { namespace, name } = useParams<{ namespace: string; name: string }>()
  const [pipeline, setPipeline] = useState<Pipeline | null>(null)
  const [runs, setRuns] = useState<PipelineRun[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [activeOnly, setActiveOnly] = useState(false)
  const [page, setPage] = useState(0)
  const [showCreate, setShowCreate] = useState(false)
  const [description, setDescription] = useState('')
  const [creating, setCreating] = useState(false)

  useEffect(() => {
    fetch(`/api/pipelines/${namespace}/${name}`)
      .then(r => r.ok ? r.json() : null)
      .then(setPipeline)
      .catch(() => {})
  }, [namespace, name])

  useEffect(() => {
    const load = async () => {
      try {
        const res = await fetch(`/api/pipelines/${namespace}/${name}/runs`)
        if (!res.ok) throw new Error(await res.text())
        setRuns(await res.json())
      } catch (e) {
        setError(e instanceof Error ? e.message : 'Failed to load')
      } finally {
        setLoading(false)
      }
    }
    load()
    const id = setInterval(load, 3_000)
    return () => clearInterval(id)
  }, [namespace, name])

  const handleCreateRun = async () => {
    if (!description.trim()) return
    setCreating(true)
    try {
      const res = await fetch(`/api/pipelines/${namespace}/${name}/runs`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ description: description.trim() }),
      })
      if (!res.ok) throw new Error(await res.text())
      setShowCreate(false)
      setDescription('')
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Failed to create run')
    } finally {
      setCreating(false)
    }
  }

  const isSpot = pipeline?.triggerType === 'Spot'

  if (loading) return <div className="text-gray-400 py-12">Loading runs...</div>
  if (error) return <div className="text-red-400 py-12">{error}</div>

  const filtered = activeOnly ? runs.filter(r => !TERMINAL.has(r.phase)) : runs
  const totalPages = Math.ceil(filtered.length / PAGE_SIZE)
  const pageRuns = filtered.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE)

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <Link to="/" className="text-gray-400 hover:text-white transition-colors">&larr; Pipelines</Link>
          <span className="text-gray-600">/</span>
          <h1 className="text-2xl font-semibold">{name}</h1>
        </div>
        <div className="flex items-center gap-3">
          {isSpot && (
            <button
              onClick={() => setShowCreate(true)}
              className="px-3 py-1.5 rounded-lg text-xs font-medium bg-indigo-600 text-white hover:bg-indigo-500 transition-colors"
            >
              Create Run
            </button>
          )}
          <button
            onClick={() => { setActiveOnly(!activeOnly); setPage(0) }}
            className={`flex items-center gap-2 px-3 py-1.5 rounded-full text-xs font-medium border transition-colors ${
              activeOnly
                ? 'bg-indigo-500/15 text-indigo-400 border-indigo-500/30'
                : 'bg-gray-800/50 text-gray-500 border-gray-700 hover:text-gray-400 hover:border-gray-600'
            }`}
          >
            <span className={`inline-block w-1.5 h-1.5 rounded-full ${activeOnly ? 'bg-indigo-400' : 'bg-gray-600'}`} />
            Active only
          </button>
        </div>
      </div>

      {showCreate && (
        <div className="mb-6 p-4 rounded-lg border border-gray-700 bg-gray-900/50">
          <h3 className="text-sm font-medium text-gray-300 mb-2">Create Spot Run</h3>
          <textarea
            value={description}
            onChange={e => setDescription(e.target.value)}
            placeholder="Describe the task..."
            rows={3}
            className="w-full bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm text-gray-200 placeholder-gray-500 focus:outline-none focus:border-indigo-500 resize-none"
          />
          <div className="flex justify-end gap-2 mt-2">
            <button
              onClick={() => { setShowCreate(false); setDescription('') }}
              className="px-3 py-1.5 rounded text-xs text-gray-400 hover:text-white transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={handleCreateRun}
              disabled={creating || !description.trim()}
              className="px-3 py-1.5 rounded-lg text-xs font-medium bg-indigo-600 text-white hover:bg-indigo-500 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              {creating ? 'Creating...' : 'Create'}
            </button>
          </div>
        </div>
      )}

      {!filtered.length ? (
        <div className="text-gray-500 py-12">{activeOnly ? 'No active runs.' : 'No runs yet.'}</div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-gray-800">
          <table className="w-full text-sm">
            <thead className="bg-gray-900/50">
              <tr className="text-gray-400 text-left">
                <th className="px-4 py-3 font-medium">Run</th>
                <th className="px-4 py-3 font-medium">Issue</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium">Step</th>
                <th className="px-4 py-3 font-medium">Branch</th>
                <th className="px-4 py-3 font-medium">Started</th>
                <th className="px-4 py-3 font-medium"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-800">
              {pageRuns.map(r => (
                <tr key={r.name} className="hover:bg-gray-900/50 transition-colors">
                  <td className="px-4 py-3">
                    <Link to={`/runs/${r.namespace}/${r.name}`} className="text-indigo-400 hover:text-indigo-300">
                      {r.name}
                    </Link>
                  </td>
                  <td className="px-4 py-3">
                    {r.issueKey || r.issueNumber ? (
                      <>
                        <span className="text-gray-300">{r.issueKey || `#${r.issueNumber}`}</span>
                        <span className="text-gray-500 ml-2 truncate max-w-xs inline-block align-bottom">{r.issueTitle}</span>
                      </>
                    ) : (
                      <span className="text-gray-400 truncate max-w-xs inline-block align-bottom">{r.description || 'Spot run'}</span>
                    )}
                  </td>
                  <td className="px-4 py-3"><StatusBadge status={r.phase} /></td>
                  <td className="px-4 py-3 text-gray-400">{r.currentStep || '—'}</td>
                  <td className="px-4 py-3">
                    <code className="text-xs bg-gray-800 px-2 py-0.5 rounded text-gray-300">{r.branch}</code>
                  </td>
                  <td className="px-4 py-3 text-gray-400"><TimeAgo date={r.startedAt} /></td>
                  <td className="px-4 py-3 text-right">
                    <span className="flex items-center gap-0.5">
                      {TERMINAL.has(r.phase) ? (
                        <RetryButton namespace={r.namespace} name={r.name} small />
                      ) : (
                        <StopButton namespace={r.namespace} name={r.name} small />
                      )}
                      <DeleteButton namespace={r.namespace} name={r.name} small />
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {totalPages > 1 && (
            <div className="flex items-center justify-between px-4 py-3 border-t border-gray-800 text-sm text-gray-500">
              <span>{filtered.length} runs</span>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => setPage(p => p - 1)}
                  disabled={page === 0}
                  className="px-3 py-1 rounded hover:bg-gray-800 disabled:opacity-30 disabled:cursor-not-allowed"
                >
                  ← Prev
                </button>
                <span>{page + 1} / {totalPages}</span>
                <button
                  onClick={() => setPage(p => p + 1)}
                  disabled={page >= totalPages - 1}
                  className="px-3 py-1 rounded hover:bg-gray-800 disabled:opacity-30 disabled:cursor-not-allowed"
                >
                  Next →
                </button>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
