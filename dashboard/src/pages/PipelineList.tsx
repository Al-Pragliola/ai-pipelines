import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import type { Pipeline, PipelineRun } from '../types/api'
import StatusBadge from '../components/StatusBadge'
import TimeAgo from '../components/TimeAgo'
import RetryButton from '../components/RetryButton'
import StopButton from '../components/StopButton'
import DeleteButton from '../components/DeleteButton'
import YamlEditor from '../components/YamlEditor'

const TERMINAL = new Set(['Succeeded', 'Failed', 'Stopped', 'Deleting'])
const PAGE_SIZE = 10

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
              <Link
                to={`/pipelines/${p.namespace}/${p.name}`}
                className="block p-5 hover:bg-gray-800/50 transition-colors"
              >
                <div className="flex items-center justify-between">
                  <div>
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
                  <div className="flex items-center gap-3">
                    <button
                      title="View YAML"
                      onClick={(e) => { e.preventDefault(); openYaml(p.namespace, p.name) }}
                      className="group p-1.5 rounded-md text-gray-500 hover:text-indigo-400 hover:bg-indigo-500/10 transition-all duration-150"
                    >
                      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor" className="w-5 h-5 transition-transform duration-150 group-hover:scale-110">
                        <path d="M12 15a3 3 0 100-6 3 3 0 000 6z" />
                        <path fillRule="evenodd" d="M1.323 11.447C2.811 6.976 7.028 3.75 12.001 3.75c4.97 0 9.185 3.223 10.675 7.69.12.362.12.752 0 1.113-1.487 4.471-5.705 7.697-10.677 7.697-4.97 0-9.186-3.223-10.675-7.69a1.762 1.762 0 010-1.113zM17.25 12a5.25 5.25 0 11-10.5 0 5.25 5.25 0 0110.5 0z" clipRule="evenodd" />
                      </svg>
                    </button>
                    <div className="text-right">
                      <div className="text-sm text-gray-400">
                        {p.activeRuns > 0 ? (
                          <span className="text-blue-400">{p.activeRuns} running</span>
                        ) : (
                          <span>idle</span>
                        )}
                      </div>
                      <div className="text-xs text-gray-500 mt-1">{p.totalRuns} total runs</div>
                    </div>
                  </div>
                </div>
              </Link>
              {pageRuns.length > 0 && (
                <div className="border-t border-gray-800">
                  <table className="w-full text-sm">
                    <tbody className="divide-y divide-gray-800/50">
                      {pageRuns.map(r => (
                        <tr key={r.name} className="hover:bg-gray-800/30 transition-colors">
                          <td className="px-5 py-2.5">
                            <Link to={`/runs/${r.namespace}/${r.name}`} className="text-indigo-400 hover:text-indigo-300">
                              {r.name}
                            </Link>
                          </td>
                          <td className="px-3 py-2.5 text-gray-400">
                            {r.issueKey || r.issueNumber ? (
                              <>{r.issueKey || `#${r.issueNumber}`} <span className="text-gray-500 truncate max-w-48 inline-block align-bottom">{r.issueTitle}</span></>
                            ) : (
                              <span className="text-gray-500 truncate max-w-48 inline-block align-bottom">{r.description || 'Spot run'}</span>
                            )}
                          </td>
                          <td className="px-3 py-2.5"><StatusBadge status={r.phase} /></td>
                          <td className="px-3 py-2.5 text-gray-500 text-xs">{r.currentStep || '—'}</td>
                          <td className="px-3 py-2.5 text-gray-500 text-xs"><TimeAgo date={r.startedAt} /></td>
                          <td className="px-3 py-2.5 text-right">
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
                    <div className="flex items-center justify-between px-5 py-2 border-t border-gray-800/50 text-xs text-gray-500">
                      <span>{filtered.length} runs</span>
                      <div className="flex items-center gap-2">
                        <button
                          onClick={() => setPages(p => ({ ...p, [key]: page - 1 }))}
                          disabled={page === 0}
                          className="px-2 py-1 rounded hover:bg-gray-800 disabled:opacity-30 disabled:cursor-not-allowed"
                        >
                          ← Prev
                        </button>
                        <span>{page + 1} / {totalPages}</span>
                        <button
                          onClick={() => setPages(p => ({ ...p, [key]: page + 1 }))}
                          disabled={page >= totalPages - 1}
                          className="px-2 py-1 rounded hover:bg-gray-800 disabled:opacity-30 disabled:cursor-not-allowed"
                        >
                          Next →
                        </button>
                      </div>
                    </div>
                  )}
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
