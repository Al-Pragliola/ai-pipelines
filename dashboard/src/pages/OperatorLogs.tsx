import { useEffect, useState, useRef } from 'react'

export default function OperatorLogs() {
  const [logs, setLogs] = useState('')
  const [lines, setLines] = useState(500)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [autoRefresh, setAutoRefresh] = useState(true)
  const bottomRef = useRef<HTMLDivElement>(null)

  const load = async () => {
    try {
      const res = await fetch(`/api/operator/logs?lines=${lines}`)
      if (!res.ok) {
        setError(await res.text())
        setLogs('')
        return
      }
      setError('')
      setLogs(await res.text())
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [lines])

  useEffect(() => {
    if (!autoRefresh) return
    const id = setInterval(load, 3000)
    return () => clearInterval(id)
  }, [autoRefresh, lines])

  useEffect(() => {
    if (autoRefresh && bottomRef.current) {
      bottomRef.current.scrollIntoView({ behavior: 'smooth' })
    }
  }, [logs, autoRefresh])

  const logLines = logs ? logs.split('\n') : []

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-2xl font-semibold">Operator Logs</h1>
        <div className="flex items-center gap-4">
          <select
            value={lines}
            onChange={e => setLines(Number(e.target.value))}
            className="bg-gray-800 border border-gray-700 rounded px-2 py-1 text-sm text-gray-300"
          >
            <option value={100}>100 lines</option>
            <option value={500}>500 lines</option>
            <option value={1000}>1000 lines</option>
            <option value={5000}>5000 lines</option>
          </select>
          <label className="flex items-center gap-2 text-sm text-gray-400 cursor-pointer">
            <input
              type="checkbox"
              checked={autoRefresh}
              onChange={e => setAutoRefresh(e.target.checked)}
              className="rounded bg-gray-800 border-gray-600"
            />
            Auto-refresh
          </label>
          <button
            onClick={load}
            className="text-sm text-indigo-400 hover:text-indigo-300 transition-colors"
          >
            Refresh
          </button>
        </div>
      </div>

      {error && (
        <div className="bg-red-900/20 border border-red-800 rounded-lg p-4 text-sm text-red-300 mb-4">
          {error}
        </div>
      )}

      {loading ? (
        <div className="text-gray-400 py-12">Loading operator logs...</div>
      ) : logLines.length === 0 && !error ? (
        <div className="text-gray-500 py-12">No logs available.</div>
      ) : (
        <div className="bg-gray-900 border border-gray-800 rounded-lg overflow-hidden">
          <div className="overflow-auto max-h-[75vh] p-4 font-mono text-xs leading-relaxed">
            {logLines.map((line, i) => (
              <div key={i} className="flex hover:bg-gray-800/30">
                <span className="text-gray-600 select-none w-12 text-right pr-3 flex-shrink-0">
                  {i + 1}
                </span>
                <span className={`whitespace-pre-wrap break-all ${
                  line.includes('"level":"error"') || line.includes('ERROR')
                    ? 'text-red-400'
                    : line.includes('"level":"info"') || line.includes('INFO')
                    ? 'text-gray-300'
                    : line.includes('"level":"debug"') || line.includes('DEBUG')
                    ? 'text-gray-500'
                    : 'text-gray-400'
                }`}>
                  {line}
                </span>
              </div>
            ))}
            <div ref={bottomRef} />
          </div>
        </div>
      )}
    </div>
  )
}
