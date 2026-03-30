import { useEffect, useState, useRef } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import { PrismLight as SyntaxHighlighter } from 'react-syntax-highlighter'
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism'
import type { PipelineRun, StepStatus, SelectedRepo } from '../types/api'
import StatusBadge from '../components/StatusBadge'
import LogViewer from '../components/LogViewer'
import DiffDialog from '../components/DiffDialog'
import ChatDialog from '../components/ChatDialog'
import RetryButton from '../components/RetryButton'
import StopButton from '../components/StopButton'
import DeleteButton from '../components/DeleteButton'

function formatDuration(sec: number): string {
  if (sec < 60) return `${sec}s`
  const min = Math.floor(sec / 60)
  return `${min}m ${sec % 60}s`
}

function duration(durationSeconds?: number): string {
  if (durationSeconds == null) return '—'
  return formatDuration(durationSeconds)
}

const ACTIVE_PHASES = new Set(['Running', 'Initializing'])

function StepCard({ step, namespace, runName }: { step: StepStatus; namespace: string; runName: string }) {
  const [logs, setLogs] = useState<string | null>(null)
  const [loadingLogs, setLoadingLogs] = useState(false)
  const [open, setOpen] = useState(false)
  const logRef = useRef<HTMLDivElement>(null)
  const wasAtBottom = useRef(true)

  const doFetch = async () => {
    try {
      const res = await fetch(`/api/runs/${namespace}/${runName}/steps/${step.name}/logs`)
      if (!res.ok) throw new Error(await res.text())
      const el = logRef.current
      if (el) {
        wasAtBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40
      }
      setLogs(await res.text())
      if (wasAtBottom.current) {
        setTimeout(() => logRef.current?.scrollTo(0, logRef.current.scrollHeight), 50)
      }
    } catch {
      setLogs('Failed to load logs')
    }
  }

  const toggle = async () => {
    if (open) {
      setOpen(false)
      setLogs(null)
      return
    }
    setOpen(true)
    setLoadingLogs(true)
    await doFetch()
    setLoadingLogs(false)
  }

  // Auto-refresh logs while the step is active and the panel is open
  useEffect(() => {
    if (!open || !ACTIVE_PHASES.has(step.phase)) return
    const id = setInterval(doFetch, 3_000)
    return () => clearInterval(id)
  }, [open, step.phase, namespace, runName, step.name])

  return (
    <div className="bg-gray-900 border border-gray-800 rounded-lg overflow-hidden">
      <div className="flex items-center justify-between px-4 py-3">
        <div className="flex items-center gap-3">
          <StepIcon type={step.type} />
          <div>
            <span className="font-medium text-white">{step.name}</span>
            <span className="text-xs text-gray-500 ml-2">{step.type}</span>
          </div>
        </div>
        <div className="flex items-center gap-4">
          {step.attempt > 1 && (
            <span className="text-xs text-yellow-400">attempt {step.attempt}</span>
          )}
          <span className="text-xs text-gray-500 tabular-nums">{duration(step.durationSeconds)}</span>
          <StatusBadge status={step.phase} />
          <button
            onClick={toggle}
            disabled={loadingLogs || !step.jobName}
            className="text-xs px-3 py-1 rounded bg-gray-800 text-gray-300 hover:bg-gray-700 hover:text-white transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
          >
            {loadingLogs ? 'Loading...' : open ? 'Hide logs' : 'Logs'}
          </button>
        </div>
      </div>
      {step.message && (
        <div className="px-4 pb-2 text-xs text-gray-500">{step.message}</div>
      )}
      {open && logs !== null && (
        <div
          ref={logRef}
          className="bg-black/50 p-4 overflow-auto max-h-[32rem] border-t border-gray-800"
        >
          <LogViewer logs={logs} isActive={ACTIVE_PHASES.has(step.phase) && (step.type === 'ai' || step.type === 'triage')} />
        </div>
      )}
    </div>
  )
}

function StepIcon({ type }: { type: string }) {
  const cls = "w-5 h-5 text-gray-500"
  switch (type) {
    case 'git-checkout':
    case 'git-push':
      return (
        <svg className={cls} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" d="M17.25 6.75 22.5 12l-5.25 5.25m-10.5 0L1.5 12l5.25-5.25m7.5-3-4.5 16.5" />
        </svg>
      )
    case 'ai':
    case 'triage':
      return (
        <svg className={cls} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" d="M9.813 15.904 9 18.75l-.813-2.846a4.5 4.5 0 0 0-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 0 0 3.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 0 0 3.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 0 0-3.09 3.09ZM18.259 8.715 18 9.75l-.259-1.035a3.375 3.375 0 0 0-2.455-2.456L14.25 6l1.036-.259a3.375 3.375 0 0 0 2.455-2.456L18 2.25l.259 1.035a3.375 3.375 0 0 0 2.455 2.456L21.75 6l-1.036.259a3.375 3.375 0 0 0-2.455 2.456Z" />
        </svg>
      )
    case 'shell':
      return (
        <svg className={cls} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" d="m6.75 7.5 3 2.25-3 2.25m4.5 0h3M4.5 19.5h15a2.25 2.25 0 0 0 2.25-2.25V6.75A2.25 2.25 0 0 0 19.5 4.5h-15A2.25 2.25 0 0 0 2.25 6.75v10.5A2.25 2.25 0 0 0 4.5 19.5Z" />
        </svg>
      )
    default:
      return <div className="w-5 h-5 rounded-full bg-gray-800" />
  }
}

function RepoSelector({ run, onSelected }: { run: PipelineRun; onSelected: () => void }) {
  const [repos, setRepos] = useState<SelectedRepo[]>([])
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  // Fetch pipeline to get candidate repos
  useEffect(() => {
    const load = async () => {
      try {
        const res = await fetch(`/api/pipelines/${run.namespace}/${run.pipeline}/repos`)
        if (res.ok) {
          setRepos(await res.json())
        }
      } catch {
        // Fallback: let user type repo manually
      }
    }
    load()
  }, [run.namespace, run.pipeline])

  const selectRepo = async (repo: SelectedRepo) => {
    setSubmitting(true)
    setError('')
    try {
      const res = await fetch(`/api/runs/${run.namespace}/${run.name}/select-repo`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(repo),
      })
      if (!res.ok) throw new Error(await res.text())
      onSelected()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to select repo')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="bg-orange-500/10 border border-orange-500/30 rounded-lg p-5">
      <h3 className="text-orange-400 font-medium mb-2">Waiting for repo selection</h3>
      {run.triageResult && (
        <div className="mb-4 text-sm">
          <p className="text-gray-400">
            AI suggested <span className="text-white font-medium">{run.triageResult.repo}</span>
            {' '}with <span className="text-orange-400">{(parseFloat(run.triageResult.confidence) * 100).toFixed(0)}%</span> confidence
          </p>
          {run.triageResult.reasoning && (
            <p className="text-gray-500 mt-1 text-xs">{run.triageResult.reasoning}</p>
          )}
        </div>
      )}
      {error && <p className="text-red-400 text-sm mb-3">{error}</p>}
      <div className="flex flex-wrap gap-2">
        {/* If triage suggested a repo, show it as first option */}
        {run.triageResult?.repo && (() => {
          const [owner, name] = run.triageResult.repo.split('/')
          return (
            <button
              onClick={() => selectRepo({ owner, name })}
              disabled={submitting}
              className="px-4 py-2 rounded bg-orange-500/20 text-orange-300 hover:bg-orange-500/30 border border-orange-500/30 transition-colors disabled:opacity-50 text-sm"
            >
              Use AI suggestion: {run.triageResult.repo}
            </button>
          )
        })()}
        {repos.filter(r => run.triageResult?.repo !== `${r.owner}/${r.name}`).map(repo => (
          <button
            key={`${repo.owner}/${repo.name}`}
            onClick={() => selectRepo(repo)}
            disabled={submitting}
            className="px-4 py-2 rounded bg-gray-800 text-gray-300 hover:bg-gray-700 border border-gray-700 transition-colors disabled:opacity-50 text-sm"
          >
            {repo.owner}/{repo.name}
          </button>
        ))}
      </div>
    </div>
  )
}

function StepApproval({ run, onApproved }: { run: PipelineRun; onApproved: () => void }) {
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const [diff, setDiff] = useState<string | null>(null)
  const [diffLoading, setDiffLoading] = useState(true)
  const [diffError, setDiffError] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [chatOpen, setChatOpen] = useState(false)

  // Fetch diff (re-runs when diffKey changes, e.g. after chat edits)
  const [diffKey, setDiffKey] = useState(0)
  const [refreshing, setRefreshing] = useState(false)
  useEffect(() => {
    let cancelled = false
    let timer: ReturnType<typeof setTimeout>
    let retries = 0
    const maxRetries = 15 // ~30 seconds
    setDiff(null)
    setDiffLoading(true)
    setDiffError('')
    const fetchDiff = async () => {
      try {
        const res = await fetch(`/api/runs/${run.namespace}/${run.name}/diff`)
        if (!res.ok) {
          if (res.status === 404 && retries < maxRetries) {
            retries++
            if (!cancelled) timer = setTimeout(fetchDiff, 2000)
            return
          }
          if (res.status === 404) {
            throw new Error('Diff preview not available — the diff job may not exist or failed to start')
          }
          throw new Error(await res.text())
        }
        const text = await res.text()
        if (!cancelled) {
          setDiff(text)
          setDiffLoading(false)
        }
      } catch (e) {
        if (!cancelled) {
          setDiffError(e instanceof Error ? e.message : 'Failed to load diff')
          setDiffLoading(false)
        }
      }
    }
    fetchDiff()
    return () => { cancelled = true; clearTimeout(timer) }
  }, [run.namespace, run.name, diffKey])

  const refreshDiff = async () => {
    setRefreshing(true)
    try {
      await fetch(`/api/runs/${run.namespace}/${run.name}/diff/refresh`, { method: 'POST' })
    } catch {
      // best effort
    }
    setRefreshing(false)
    setDiffKey(k => k + 1)
  }

  const approve = async () => {
    setSubmitting(true)
    setError('')
    try {
      const res = await fetch(`/api/runs/${run.namespace}/${run.name}/approve`, {
        method: 'POST',
      })
      if (!res.ok) throw new Error(await res.text())
      onApproved()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to approve')
    } finally {
      setSubmitting(false)
    }
  }

  // Diff stats
  const hasDiff = diff && diff.trim() !== ''
  const fileCount = hasDiff ? (diff.match(/^diff --git /gm) ?? []).length : 0

  return (
    <div className="bg-orange-500/10 border border-orange-500/30 rounded-lg overflow-hidden">
      {/* Header */}
      <div className="px-5 pt-5 pb-3">
        <div className="flex items-center gap-3">
          <svg className="w-5 h-5 text-orange-400" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m0-10.036A11.959 11.959 0 0 1 3.598 6 11.99 11.99 0 0 0 3 9.75c0 5.592 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.75h-.152c-3.196 0-6.1-1.25-8.25-3.286Z" />
          </svg>
          <h3 className="text-orange-400 font-medium">Approval required</h3>
        </div>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 mt-2 ml-8 text-sm">
          {run.resolvedRepo && (
            <div className="flex items-center gap-1.5 text-gray-400">
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M20.25 6.375c0 2.278-3.694 4.125-8.25 4.125S3.75 8.653 3.75 6.375m16.5 0c0-2.278-3.694-4.125-8.25-4.125S3.75 4.097 3.75 6.375m16.5 0v11.25c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125V6.375" />
              </svg>
              <span className="text-gray-200">{run.resolvedRepo.owner}/{run.resolvedRepo.name}</span>
            </div>
          )}
          {run.branch && (
            <div className="flex items-center gap-1.5 text-gray-400">
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M17.25 6.75 22.5 12l-5.25 5.25m-10.5 0L1.5 12l5.25-5.25m7.5-3-4.5 16.5" />
              </svg>
              <code className="text-xs bg-gray-800/50 px-1.5 py-0.5 rounded text-gray-200">{run.branch}</code>
            </div>
          )}
          <div className="flex items-center gap-1.5 text-gray-400">
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M7.5 21 3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5" />
            </svg>
            <span>push to <span className="text-gray-200">{run.resolvedRepo?.forkOwner || run.resolvedRepo?.owner}/{run.resolvedRepo?.name}</span></span>
          </div>
        </div>
      </div>

      {/* Inline diff viewer */}
      <div className="px-5 pb-3">
        {diffLoading && (
          <div className="text-sm text-gray-500 py-2 animate-pulse">Loading diff preview...</div>
        )}
        {diffError && (
          <div className="flex items-center gap-3 py-2">
            <span className="text-sm text-yellow-500">{diffError}</span>
            <button
              onClick={refreshDiff}
              disabled={refreshing}
              className="px-3 py-1 text-xs rounded bg-gray-800 text-gray-300 hover:bg-gray-700 border border-gray-700 transition-colors disabled:opacity-50"
            >
              {refreshing ? 'Refreshing...' : 'Refresh diff'}
            </button>
          </div>
        )}
        {hasDiff && (
          <div className="rounded-lg overflow-hidden border border-gray-700">
            <div className="bg-gray-800 px-4 py-2 text-xs text-gray-400 flex justify-between items-center">
              <span>{fileCount} file{fileCount !== 1 ? 's' : ''} changed</span>
              <span className="tabular-nums">{diff.split('\n').length} lines</span>
            </div>
            <div className="overflow-auto max-h-[32rem]">
              <SyntaxHighlighter
                language="diff"
                style={oneDark}
                customStyle={{ margin: 0, borderRadius: 0, fontSize: '0.75rem' }}
                showLineNumbers
              >
                {diff}
              </SyntaxHighlighter>
            </div>
          </div>
        )}
        {!diffLoading && !diffError && !hasDiff && (
          <div className="text-sm text-gray-500 py-2">No changes detected in workspace.</div>
        )}
      </div>

      {/* Footer: approve left, chat center, review changes right */}
      {error && <p className="text-red-400 text-sm px-5 pb-2">{error}</p>}
      <div className="flex items-center justify-between px-5 py-3 border-t border-orange-500/20 bg-orange-500/5">
        <button
          onClick={approve}
          disabled={submitting}
          className="px-5 py-2 rounded-lg bg-green-600 text-white text-sm font-medium hover:bg-green-500 transition-colors disabled:opacity-50"
        >
          {submitting ? 'Approving...' : 'Approve & Push'}
        </button>
        <button
          onClick={() => setChatOpen(true)}
          className="flex items-center gap-2 px-4 py-2 rounded-lg bg-indigo-500/20 border border-indigo-500/30 text-sm text-indigo-200 hover:bg-indigo-500/30 hover:border-indigo-500/50 transition-colors"
        >
          <svg className="w-4 h-4 text-indigo-400" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" d="M7.5 8.25h9m-9 3H12m-9.75 1.51c0 1.6 1.123 2.994 2.707 3.227 1.087.16 2.185.283 3.293.369V21l4.076-4.076a1.526 1.526 0 0 1 1.037-.443 48.282 48.282 0 0 0 5.68-.494c1.584-.233 2.707-1.626 2.707-3.228V6.741c0-1.602-1.123-2.995-2.707-3.228A48.394 48.394 0 0 0 12 3c-2.392 0-4.744.175-7.043.513C3.373 3.746 2.25 5.14 2.25 6.741v6.018Z" />
          </svg>
          Chat about changes
        </button>
        {hasDiff && (
          <button
            onClick={() => setDialogOpen(true)}
            className="flex items-center gap-2 px-4 py-2 rounded-lg bg-gray-800 border border-gray-700 text-sm text-gray-200 hover:bg-gray-700 hover:border-gray-600 transition-colors"
          >
            <svg className="w-4 h-4 text-gray-400" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M13.5 6H5.25A2.25 2.25 0 0 0 3 8.25v10.5A2.25 2.25 0 0 0 5.25 21h10.5A2.25 2.25 0 0 0 18 18.75V10.5m-10.5 6L21 3m0 0h-5.25M21 3v5.25" />
            </svg>
            Review Changes
          </button>
        )}
      </div>

      {dialogOpen && diff && <DiffDialog diff={diff} onClose={() => setDialogOpen(false)} />}
      {chatOpen && (
        <ChatDialog
          namespace={run.namespace}
          runName={run.name}
          onClose={() => {
            setChatOpen(false)
            setDiffKey(k => k + 1)
          }}
        />
      )}
    </div>
  )
}

export default function RunDetail() {
  const { namespace, name } = useParams<{ namespace: string; name: string }>()
  const navigate = useNavigate()
  const [run, setRun] = useState<PipelineRun | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const load = async () => {
    try {
      const res = await fetch(`/api/runs/${namespace}/${name}`)
      if (!res.ok) throw new Error(await res.text())
      setRun(await res.json())
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
    // Don't poll in terminal states or during step-approval (chat dialog may be open)
    const isStable = run?.phase === 'Succeeded' || run?.phase === 'Failed' || run?.phase === 'Stopped' || run?.phase === 'Deleting'
      || (run?.phase === 'WaitingForInput' && run?.waitingFor === 'step-approval')
    if (isStable) return
    const id = setInterval(load, 3_000)
    return () => clearInterval(id)
  }, [namespace, name, run?.phase, run?.waitingFor])

  if (loading) return <div className="text-gray-400 py-12">Loading run...</div>
  if (error) return <div className="text-red-400 py-12">{error}</div>
  if (!run) return null

  const issueRef = run.issueKey || (run.issueNumber ? `#${run.issueNumber}` : '')
  const isSpotRun = !issueRef
  const isTerminal = run.phase === 'Succeeded' || run.phase === 'Failed' || run.phase === 'Stopped' || run.phase === 'Deleting'

  return (
    <div>
      <div className="flex items-center gap-3 mb-6">
        <Link to="/" className="text-gray-400 hover:text-white transition-colors">&larr;</Link>
        <Link to={`/pipelines/${namespace}/${run.pipeline}`} className="text-gray-400 hover:text-white transition-colors">{run.pipeline}</Link>
        <span className="text-gray-600">/</span>
        <h1 className="text-2xl font-semibold">{run.name}</h1>
        <StatusBadge status={run.phase} />
        <span className="ml-auto flex items-center gap-1">
          {!isTerminal && (
            <StopButton namespace={namespace!} name={name!} onStopped={load} />
          )}
          {isTerminal && (
            <RetryButton namespace={namespace!} name={name!} />
          )}
          <DeleteButton namespace={namespace!} name={name!} onDeleted={() => navigate(`/pipelines/${namespace}/${run.pipeline}`)} />
        </span>
      </div>

      <div className="flex flex-wrap gap-6 mb-6 text-sm text-gray-400">
        <div>
          {isSpotRun ? (
            <>
              <span className="text-gray-500">Task:</span>{' '}
              <span className="text-gray-200">{run.description || 'Spot run'}</span>
            </>
          ) : (
            <>
              <span className="text-gray-500">Issue:</span>{' '}
              <span className="text-gray-200">{issueRef} {run.issueTitle}</span>
            </>
          )}
        </div>
        {run.resolvedRepo && (
          <div>
            <span className="text-gray-500">Repo:</span>{' '}
            <span className="text-gray-200">{run.resolvedRepo.owner}/{run.resolvedRepo.name}</span>
          </div>
        )}
        <div>
          <span className="text-gray-500">Branch:</span>{' '}
          <code className="text-xs bg-gray-800 px-2 py-0.5 rounded text-gray-300">{run.branch}</code>
        </div>
        <div>
          <span className="text-gray-500">Duration:</span>{' '}
          <span className="tabular-nums">{duration(run.durationSeconds)}</span>
        </div>
      </div>

      {run.phase === 'WaitingForInput' && run.waitingFor === 'step-approval' && (
        <div className="mb-6">
          <StepApproval run={run} onApproved={load} />
        </div>
      )}

      {run.phase === 'WaitingForInput' && run.waitingFor !== 'step-approval' && (
        <div className="mb-6">
          <RepoSelector run={run} onSelected={load} />
        </div>
      )}

      <div className="space-y-3">
        {run.steps.map(s => (
          <StepCard key={`${s.name}-${s.attempt}`} step={s} namespace={namespace!} runName={name!} />
        ))}
      </div>
    </div>
  )
}
