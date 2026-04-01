import { useEffect, useState, useCallback } from 'react'

interface Artifact {
  name: string
  size: number
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

const TERMINAL_PHASES = new Set(['Succeeded', 'Failed', 'Stopped', 'Deleting'])

export default function ArtifactsPanel({ namespace, runName, phase }: { namespace: string; runName: string; phase: string }) {
  const [artifacts, setArtifacts] = useState<Artifact[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [downloading, setDownloading] = useState(false)
  const [error, setError] = useState('')

  const fetchArtifacts = useCallback(async () => {
    try {
      const res = await fetch(`/api/runs/${namespace}/${runName}/artifacts`)
      if (!res.ok) {
        if (res.status === 404 || res.status === 503) {
          // Pod not ready or no workspace — keep current state, retry on next poll
          return
        }
        throw new Error(await res.text())
      }
      setArtifacts(await res.json())
      setError('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load artifacts')
    } finally {
      setLoading(false)
    }
  }, [namespace, runName])

  useEffect(() => {
    fetchArtifacts()
    if (TERMINAL_PHASES.has(phase)) return
    const id = setInterval(fetchArtifacts, 5_000)
    return () => clearInterval(id)
  }, [fetchArtifacts, phase])

  const handleDownload = async () => {
    setDownloading(true)
    try {
      const res = await fetch(`/api/runs/${namespace}/${runName}/artifacts/download`)
      if (!res.ok) throw new Error(await res.text())
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `${runName}-artifacts.tar.gz`
      a.click()
      URL.revokeObjectURL(url)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Download failed')
    } finally {
      setDownloading(false)
    }
  }

  // Don't render anything until loaded
  if (loading) return null
  // Don't render if no artifacts
  if (!artifacts || artifacts.length === 0) return null
  if (error) return null

  return (
    <div className="bg-gray-900 border border-gray-800 rounded-lg overflow-hidden mt-6">
      <div className="flex items-center justify-between px-5 py-3 border-b border-gray-800">
        <h3 className="text-sm font-medium text-gray-300">Artifacts</h3>
        <button
          onClick={handleDownload}
          disabled={downloading}
          className="text-xs px-3 py-1.5 bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed text-white rounded transition-colors"
        >
          {downloading ? 'Downloading...' : 'Download All (.tar.gz)'}
        </button>
      </div>
      <ul className="divide-y divide-gray-800">
        {artifacts.map(a => (
          <li key={a.name} className="flex items-center justify-between px-5 py-2 text-sm">
            <span className="text-gray-300 font-mono text-xs">{a.name}</span>
            <span className="text-gray-500 text-xs tabular-nums">{formatSize(a.size)}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}
