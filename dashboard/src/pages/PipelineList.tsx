import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import type { Pipeline, PipelineRun } from '../types/api'
import StatusBadge from '../components/StatusBadge'
import TimeAgo from '../components/TimeAgo'
import RetryButton from '../components/RetryButton'
import StopButton from '../components/StopButton'
import DeleteButton from '../components/DeleteButton'
import YamlEditor from '../components/YamlEditor'

const TERMINAL = new Set(['Succeeded', 'Failed', 'Stopped', 'Deleting'])
const PAGE_SIZE = 5

const BAR_COLORS: Record<string, string> = {
  Running: 'bg-blue-500',
  WaitingForInput: 'bg-orange-500',
  Initializing: 'bg-cyan-500',
  Failed: 'bg-red-500',
  Succeeded: 'bg-green-500',
  Stopped: 'bg-gray-600',
  Pending: 'bg-yellow-500',
  Deleting: 'bg-red-500',
}

const TEXT_COLORS: Record<string, string> = {
  Running: 'text-blue-400',
  WaitingForInput: 'text-orange-400',
  Initializing: 'text-cyan-400',
  Failed: 'text-red-400',
  Stopped: 'text-gray-400',
}

// Order for rendering bar segments and attention counts
const STATUS_ORDER = ['Failed', 'WaitingForInput', 'Running', 'Initializing', 'Pending', 'Stopped', 'Succeeded', 'Deleting']

function HealthBar({ runs }: { runs: PipelineRun[] }) {
  const statusCounts = useMemo(() => {
    const counts: Record<string, number> = {}
    runs.forEach(r => { counts[r.phase] = (counts[r.phase] || 0) + 1 })
    return counts
  }, [runs])

  const total = runs.length
  if (total === 0) return <span className="text-xs text-gray-600">No runs</span>

  const segments = STATUS_ORDER
    .map(phase => ({ phase, color: BAR_COLORS[phase] || 'bg-gray-600', count: statusCounts[phase] || 0 }))
    .filter(s => s.count > 0)

  // Show attention items: anything that's not Succeeded
  const attentionItems = STATUS_ORDER
    .filter(phase => phase !== 'Succeeded' && statusCounts[phase])
    .map(phase => ({ phase, count: statusCounts[phase] }))

  return (
    <div className="flex flex-col items-end gap-1">
      <div
        className="flex h-1.5 rounded-full overflow-hidden w-24 bg-gray-800"
        title={segments.map(s => `${s.count} ${s.phase}`).join(', ')}
      >
        {segments.map(s => (
          <div
            key={s.phase}
            className={s.color}
            style={{ width: `${Math.max((s.count / total) * 100, 4)}%` }}
          />
        ))}
      </div>
      <div className="flex items-center gap-1.5 text-xs">
        <span className="text-gray-500">{total} {total === 1 ? 'run' : 'runs'}</span>
        {attentionItems.map(({ phase, count }) => (
          <span key={phase} className={TEXT_COLORS[phase] || 'text-gray-400'}>
            &middot; {count} {phase === 'WaitingForInput' ? 'waiting' : phase.toLowerCase()}
          </span>
        ))}
      </div>
    </div>
  )
}

function stripPrefix(runName: string, pipelineName: string): string {
  return runName.startsWith(pipelineName + '-') ? runName.slice(pipelineName.length + 1) : runName
}

export default function PipelineList() {
  const [pipelines, setPipelines] = useState<Pipeline[]>([])
  const [runs, setRuns] = useState<Record<string, PipelineRun[]>>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [activeOnly, setActiveOnly] = useState(false)
  const [pages, setPages] = useState<Record<string, number>>({})
  const [yamlModal, setYamlModal] = useState<{ name: string; namespace: string } | null>(null)
  const [yamlContent, setYamlContent] = useState('')
  const [yamlLoading, setYamlLoading] = useState(false)

  const openYaml = useCallback(async (namespace: string, name: string) => {
    setYamlModal({ namespace, name })
    setYamlLoading(true)
    try {
      const res = await fetch(`/api/pipelines/${namespace}/${name}/yaml`)
      if (!res.ok) throw new Error(await res.text())
      setYamlContent(await res.text())
    } catch {
      setYamlContent('# Failed to load pipeline YAML')
    } finally {
      setYamlLoading(false)
    }
  }, [])

  useEffect(() => {
    const load = async () => {
      try {
        const res = await fetch('/api/pipelines')
        if (!res.ok) throw new Error(await res.text())
        const pList: Pipeline[] = await res.json()
        setPipelines(pList)

        const runMap: Record<string, PipelineRun[]> = {}
        await Promise.all(pList.map(async (p) => {
          const rRes = await fetch(`/api/pipelines/${p.namespace}/${p.name}/runs`)
          if (rRes.ok) {
            runMap[`${p.namespace}/${p.name}`] = await rRes.json()
          }
        }))
        setRuns(runMap)
      } catch (e) {
        setError(e instanceof Error ? e.message : 'Failed to load')
      } finally {
        setLoading(false)
      }
    }
    load()
    const id = setInterval(load, 3_000)
    return () => clearInterval(id)
  }, [])

  if (loading) return <div className="text-gray-400 py-12">Loading pipelines...</div>
  if (error) return <div className="text-red-400 py-12">{error}</div>
  if (!pipelines.length) return <div className="text-gray-500 py-12">No pipelines found.</div>

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold">Pipelines</h1>
        <div className="flex items-center gap-4">
          <button
            onClick={() => { setActiveOnly(!activeOnly); setPages({}) }}
            className={`flex items-center gap-2 px-3 py-1.5 rounded-full text-xs font-medium border transition-colors ${
              activeOnly
                ? 'bg-indigo-500/15 text-indigo-400 border-indigo-500/30'
                : 'bg-gray-800/50 text-gray-500 border-gray-700 hover:text-gray-400 hover:border-gray-600'
            }`}
          >
            <span className={`inline-block w-1.5 h-1.5 rounded-full ${activeOnly ? 'bg-indigo-400' : 'bg-gray-600'}`} />
            Active only
          </button>
          <Link to="/pipelines/new"
            className="px-4 py-2 rounded bg-indigo-600 text-white text-sm font-medium hover:bg-indigo-500 transition-colors">
            Create Pipeline
          </Link>
        </div>
      </div>
      <div className="space-y-6">
        {pipelines.map(p => {
          const key = `${p.namespace}/${p.name}`
          const allRuns = runs[key] ?? []
          const filtered = activeOnly ? allRuns.filter(r => !TERMINAL.has(r.phase)) : allRuns
          const page = pages[key] ?? 0
          const totalPages = Math.ceil(filtered.length / PAGE_SIZE)
          const pageRuns = filtered.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE)

          return (
            <div key={key} className="bg-gray-900 border border-gray-800 rounded-lg overflow-hidden">
              {/* Pipeline header */}
              <Link
                to={`/pipelines/${p.namespace}/${p.name}`}
                className="block p-5 hover:bg-gray-800/50 transition-colors"
              >
                <div className="flex items-start justify-between gap-4">
                  <div className="min-w-0">
                    <h2 className="text-lg font-medium text-white">{p.name}</h2>
                    <p className="text-sm text-gray-400 mt-1">
                      {p.triggerType === 'Spot' ? (
                        <span className="text-gray-500">{p.triggerInfo}</span>
                      ) : (
                        <><span className="text-gray-500">{p.triggerType}:</span> {p.triggerInfo}</>
                      )}
                    </p>
                    {p.triggerJql && (
                      <p className="mt-1.5 text-xs text-gray-500 font-mono bg-gray-800/50 rounded px-2 py-1 inline-block max-w-xl truncate" title={p.triggerJql}>
                        {p.triggerJql}
                      </p>
                    )}
                  </div>
                  <div className="flex items-center gap-5 flex-shrink-0">
                    <button
                      title="View pipeline YAML"
                      onClick={(e) => { e.preventDefault(); openYaml(p.namespace, p.name) }}
                      className="flex items-center gap-1.5 px-2 py-1 rounded text-xs text-gray-500 hover:text-indigo-400 hover:bg-indigo-500/10 transition-colors"
                    >
                      <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" d="m17.25 6.75 4.5 4.5-4.5 4.5m-10.5 0L2.25 11.25l4.5-4.5m7.5-3-4.5 16.5" />
                      </svg>
                      YAML
                    </button>
                    <HealthBar runs={allRuns} />
                  </div>
                </div>
              </Link>

              {/* Run list */}
              {pageRuns.length > 0 && (
                <div className="border-t border-gray-800 divide-y divide-gray-800/50">
                  {pageRuns.map(r => {
                    const issueLabel = r.issueKey || (r.issueNumber ? `#${r.issueNumber}` : '')
                    const title = r.issueTitle || r.description || 'Spot run'
                    const runId = stripPrefix(r.name, p.name)
                    const isActive = !TERMINAL.has(r.phase)

                    return (
                      <div key={r.name} className="flex items-center gap-4 px-5 py-2.5 hover:bg-gray-800/30 transition-colors">
                        {/* Status badge */}
                        <div className="flex-shrink-0 w-36">
                          <StatusBadge status={r.phase} />
                        </div>

                        {/* Issue / content */}
                        <div className="flex-1 min-w-0">
                          <Link to={`/runs/${r.namespace}/${r.name}`} className="group block truncate">
                            <span className="text-sm">
                              {issueLabel && <span className="text-indigo-400 group-hover:text-indigo-300 font-medium">{issueLabel}</span>}
                              {issueLabel && ' '}
                              <span className="text-gray-300 group-hover:text-white">{title}</span>
                            </span>
                          </Link>
                          <div className="flex items-center gap-2 mt-0.5 text-xs text-gray-500">
                            <span className="font-mono text-gray-600">{runId}</span>
                            {r.prAuthor && <span>by {r.prAuthor}</span>}
                            {isActive && r.currentStep && (
                              <span className="text-gray-400">&middot; {r.currentStep}</span>
                            )}
                          </div>
                        </div>

                        {/* Time */}
                        <div className="flex-shrink-0 text-xs text-gray-500 w-16 text-right">
                          <TimeAgo date={r.startedAt} />
                        </div>

                        {/* Actions */}
                        <div className="flex-shrink-0 flex items-center gap-0.5">
                          {TERMINAL.has(r.phase) ? (
                            <RetryButton namespace={r.namespace} name={r.name} small />
                          ) : (
                            <StopButton namespace={r.namespace} name={r.name} small />
                          )}
                          <DeleteButton namespace={r.namespace} name={r.name} small />
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}

              {/* Pagination */}
              {totalPages > 1 && (
                <div className="flex items-center justify-between px-5 py-2 border-t border-gray-800/50 text-xs text-gray-500">
                  <span>{filtered.length} runs</span>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => setPages(prev => ({ ...prev, [key]: page - 1 }))}
                      disabled={page === 0}
                      className="px-2 py-1 rounded hover:bg-gray-800 disabled:opacity-30 disabled:cursor-not-allowed"
                    >
                      &larr; Prev
                    </button>
                    <span>{page + 1} / {totalPages}</span>
                    <button
                      onClick={() => setPages(prev => ({ ...prev, [key]: page + 1 }))}
                      disabled={page >= totalPages - 1}
                      className="px-2 py-1 rounded hover:bg-gray-800 disabled:opacity-30 disabled:cursor-not-allowed"
                    >
                      Next &rarr;
                    </button>
                  </div>
                </div>
              )}

              {filtered.length === 0 && allRuns.length > 0 && (
                <div className="border-t border-gray-800 px-5 py-3 text-sm text-gray-500">
                  No active runs
                </div>
              )}
            </div>
          )
        })}
      </div>

      {/* YAML modal */}
      {yamlModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center animate-in fade-in duration-150">
          <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={() => setYamlModal(null)} />
          <div className="relative bg-gray-950 border border-gray-700/50 rounded-xl shadow-2xl shadow-black/40 w-full max-w-3xl max-h-[80vh] flex flex-col mx-4 ring-1 ring-white/5">
            <div className="flex items-center justify-between px-5 py-3.5 border-b border-gray-800/80">
              <div className="flex items-center gap-2.5">
                <span className="flex items-center justify-center w-6 h-6 rounded-md bg-indigo-500/15 text-indigo-400">
                  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-3.5 h-3.5">
                    <path fillRule="evenodd" d="M4.5 2A1.5 1.5 0 003 3.5v13A1.5 1.5 0 004.5 18h11a1.5 1.5 0 001.5-1.5V7.621a1.5 1.5 0 00-.44-1.06l-4.12-4.122A1.5 1.5 0 0011.378 2H4.5zm2.25 8.5a.75.75 0 000 1.5h6.5a.75.75 0 000-1.5h-6.5zm0 3a.75.75 0 000 1.5h6.5a.75.75 0 000-1.5h-6.5z" clipRule="evenodd" />
                  </svg>
                </span>
                <div>
                  <h3 className="text-sm font-medium text-gray-100">{yamlModal.name}</h3>
                  <p className="text-xs text-gray-500">{yamlModal.namespace}</p>
                </div>
              </div>
              <button
                onClick={() => setYamlModal(null)}
                className="p-1 rounded-md text-gray-500 hover:text-gray-300 hover:bg-gray-800 transition-colors"
              >
                <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-5 h-5">
                  <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
                </svg>
              </button>
            </div>
            <div className="flex-1 overflow-auto">
              {yamlLoading ? (
                <div className="flex items-center justify-center py-16">
                  <div className="flex items-center gap-3 text-gray-500 text-sm">
                    <svg className="animate-spin h-4 w-4 text-indigo-400" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                    </svg>
                    Loading pipeline spec...
                  </div>
                </div>
              ) : (
                <YamlEditor value={yamlContent} readOnly />
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
