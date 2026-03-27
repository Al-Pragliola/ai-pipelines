import { useEffect, useState } from 'react'
import type { IssueHistoryRecord, Pipeline, PendingIssue } from '../types/api'
import StatusBadge from '../components/StatusBadge'
import TimeAgo from '../components/TimeAgo'

export default function IssueHistory() {
  const [records, setRecords] = useState<IssueHistoryRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  // Pipeline selector
  const [pipelines, setPipelines] = useState<Pipeline[]>([])
  const [selectedPipeline, setSelectedPipeline] = useState('')

  // Manual skip form
  const [skipKey, setSkipKey] = useState('')
  const [skipping, setSkipping] = useState(false)

  // Pending issues
  const [pending, setPending] = useState<PendingIssue[]>([])
  const [pendingLoading, setPendingLoading] = useState(false)
  const [pendingError, setPendingError] = useState('')

  const load = async () => {
    try {
      const res = await fetch('/api/history')
      if (!res.ok) throw new Error(await res.text())
      setRecords(await res.json())
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  // Load pipelines
  useEffect(() => {
    fetch('/api/pipelines')
      .then(res => res.ok ? res.json() : [])
      .then(setPipelines)
      .catch(() => {})
  }, [])

  // Load pending issues when pipeline changes
  useEffect(() => {
    if (!selectedPipeline) {
      setPending([])
      return
    }
    const [ns, name] = selectedPipeline.split('/')
    setPendingLoading(true)
    setPendingError('')
    fetch(`/api/pipelines/${ns}/${name}/pending-issues`)
      .then(res => {
        if (!res.ok) throw new Error(res.statusText)
        return res.json()
      })
      .then(setPending)
      .catch(e => setPendingError(e instanceof Error ? e.message : 'Failed to load'))
      .finally(() => setPendingLoading(false))
  }, [selectedPipeline])

  const selectedPipelineObj = pipelines.find(
    p => `${p.namespace}/${p.name}` === selectedPipeline,
  )
  const triggerType = selectedPipelineObj?.triggerType

  const handleSkip = async (issueKey: string) => {
    if (!selectedPipeline) return
    const [ns, name] = selectedPipeline.split('/')
    setSkipping(true)
    try {
      const res = await fetch('/api/history', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          pipelineNamespace: ns,
          pipelineName: name,
          issueKey,
        }),
      })
      if (!res.ok) throw new Error(await res.text())
      // Add to records list
      setRecords(prev => [{
        pipelineNamespace: ns,
        pipelineName: name,
        issueKey,
        phase: 'Skipped',
        runName: '',
        completedAt: new Date().toISOString(),
      }, ...prev])
      // Remove from pending list
      setPending(prev => prev.filter(p => p.key !== issueKey))
      setSkipKey('')
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Failed to skip issue')
    } finally {
      setSkipping(false)
    }
  }

  const handleClear = async (r: IssueHistoryRecord) => {
    if (!confirm(`Re-enable ${r.issueKey} for pipeline ${r.pipelineName}?`)) return
    try {
      const res = await fetch(
        `/api/history/${r.pipelineNamespace}/${r.pipelineName}/${encodeURIComponent(r.issueKey)}`,
        { method: 'DELETE' },
      )
      if (!res.ok) throw new Error(await res.text())
      setRecords(prev => prev.filter(x =>
        !(x.pipelineNamespace === r.pipelineNamespace &&
          x.pipelineName === r.pipelineName &&
          x.issueKey === r.issueKey),
      ))
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Failed to clear')
    }
  }

  if (loading) return <div className="text-gray-400 py-12">Loading history...</div>
  if (error) return <div className="text-red-400 py-12">{error}</div>

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-semibold">Issue History</h1>
          <p className="text-sm text-gray-400 mt-1">
            Completed issues are skipped during polling. Clear an entry to allow re-processing.
          </p>
        </div>
        <span className="text-sm text-gray-500">{records.length} entries</span>
      </div>

      {/* Skip Issue Section */}
      <div className="bg-gray-900 border border-gray-800 rounded-lg p-5 mb-6">
        <h2 className="text-sm font-medium text-gray-300 mb-3">Skip an Issue</h2>

        {/* Pipeline selector */}
        <div className="flex flex-wrap items-end gap-3">
          <div className="flex-1 min-w-[200px]">
            <label className="block text-xs text-gray-500 mb-1">Pipeline</label>
            <select
              value={selectedPipeline}
              onChange={e => setSelectedPipeline(e.target.value)}
              className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-1.5 text-sm text-gray-200 focus:outline-none focus:border-indigo-500"
            >
              <option value="">Select a pipeline...</option>
              {pipelines.map(p => (
                <option key={`${p.namespace}/${p.name}`} value={`${p.namespace}/${p.name}`}>
                  {p.namespace}/{p.name} ({p.triggerType})
                </option>
              ))}
            </select>
          </div>

          {/* Manual issue key input */}
          <div className="flex-1 min-w-[160px]">
            <label className="block text-xs text-gray-500 mb-1">
              Issue Key
              {triggerType === 'GitHub' && <span className="text-gray-600 ml-1">e.g. #42</span>}
              {triggerType === 'Jira' && <span className="text-gray-600 ml-1">e.g. PROJ-123</span>}
            </label>
            <input
              type="text"
              value={skipKey}
              onChange={e => setSkipKey(e.target.value)}
              disabled={!selectedPipeline}
              placeholder={triggerType === 'GitHub' ? '#42' : triggerType === 'Jira' ? 'PROJ-123' : 'Issue key'}
              className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-1.5 text-sm text-gray-200 placeholder-gray-600 focus:outline-none focus:border-indigo-500 disabled:opacity-50"
              onKeyDown={e => {
                if (e.key === 'Enter' && skipKey.trim()) handleSkip(skipKey.trim())
              }}
            />
          </div>

          <button
            onClick={() => handleSkip(skipKey.trim())}
            disabled={!selectedPipeline || !skipKey.trim() || skipping}
            className="px-4 py-1.5 rounded bg-gray-700 text-gray-200 text-sm font-medium hover:bg-gray-600 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {skipping ? 'Skipping...' : 'Skip'}
          </button>
        </div>

        {/* Pending issues */}
        {selectedPipeline && (
          <div className="mt-4 pt-4 border-t border-gray-800">
            <h3 className="text-xs font-medium text-gray-400 mb-2">
              Pending Issues
              {!pendingLoading && <span className="text-gray-600 ml-1">({pending.length})</span>}
            </h3>

            {pendingLoading && (
              <div className="text-gray-500 text-xs py-2 animate-pulse">Loading pending issues...</div>
            )}

            {pendingError && (
              <div className="text-red-400 text-xs py-2">{pendingError}</div>
            )}

            {!pendingLoading && !pendingError && pending.length === 0 && (
              <div className="text-gray-600 text-xs py-2">No pending issues for this pipeline.</div>
            )}

            {pending.length > 0 && (
              <div className="space-y-1">
                {pending.map(issue => (
                  <div
                    key={issue.key}
                    className="flex items-center justify-between bg-gray-800/50 rounded px-3 py-2 group"
                  >
                    <div className="flex items-center gap-3 min-w-0">
                      <span className="text-sm font-medium text-indigo-400 shrink-0">{issue.key}</span>
                      <span className="text-sm text-gray-400 truncate">{issue.title}</span>
                    </div>
                    <button
                      onClick={() => handleSkip(issue.key)}
                      disabled={skipping}
                      className="text-xs text-gray-500 hover:text-yellow-400 transition-colors opacity-0 group-hover:opacity-100 shrink-0 ml-2 disabled:opacity-50"
                    >
                      Skip
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </div>

      {/* History table */}
      {records.length === 0 ? (
        <div className="text-gray-500 py-12">No completed issues recorded yet.</div>
      ) : (
        <div className="bg-gray-900 border border-gray-800 rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="border-b border-gray-800">
              <tr className="text-gray-400 text-left">
                <th className="px-5 py-3 font-medium">Issue</th>
                <th className="px-3 py-3 font-medium">Pipeline</th>
                <th className="px-3 py-3 font-medium">Status</th>
                <th className="px-3 py-3 font-medium">Run</th>
                <th className="px-3 py-3 font-medium">Completed</th>
                <th className="px-3 py-3 font-medium"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-800/50">
              {records.map(r => (
                <tr key={`${r.pipelineNamespace}/${r.pipelineName}/${r.issueKey}`}
                    className="hover:bg-gray-800/30 transition-colors">
                  <td className="px-5 py-3 text-white font-medium">{r.issueKey}</td>
                  <td className="px-3 py-3 text-gray-400">
                    <span className="text-gray-500">{r.pipelineNamespace}/</span>{r.pipelineName}
                  </td>
                  <td className="px-3 py-3"><StatusBadge status={r.phase} /></td>
                  <td className="px-3 py-3 text-gray-400 text-xs">{r.runName || '—'}</td>
                  <td className="px-3 py-3 text-gray-500 text-xs">
                    <TimeAgo date={r.completedAt} />
                  </td>
                  <td className="px-3 py-3 text-right">
                    <button
                      onClick={() => handleClear(r)}
                      className="text-xs text-red-400 hover:text-red-300 transition-colors"
                    >
                      Clear
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
