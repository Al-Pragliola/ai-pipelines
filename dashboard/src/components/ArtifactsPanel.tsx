import { useEffect, useState, useCallback } from 'react'
import ArtifactViewerDialog from './ArtifactViewerDialog'

const TEXT_EXTENSIONS = new Set([
  'go', 'ts', 'tsx', 'js', 'jsx', 'py', 'rs', 'java', 'c', 'cpp', 'h', 'hpp',
  'cs', 'rb', 'php', 'swift', 'kt', 'scala', 'r', 'lua', 'pl', 'ex', 'exs',
  'json', 'yaml', 'yml', 'toml', 'ini', 'cfg', 'conf', 'env', 'properties',
  'md', 'txt', 'rst', 'csv', 'log', 'tsv',
  'html', 'htm', 'css', 'scss', 'less', 'svg', 'xml',
  'sh', 'bash', 'zsh', 'fish', 'ps1', 'bat', 'cmd',
  'sql', 'graphql', 'proto', 'diff', 'patch',
])

function isViewableFile(name: string): boolean {
  const lower = name.toLowerCase()
  const baseName = lower.split('/').pop() || ''
  if (['makefile', 'dockerfile', '.gitignore', '.editorconfig'].includes(baseName)) return true
  const ext = baseName.split('.').pop() || ''
  if (ext === baseName) return false
  return TEXT_EXTENSIONS.has(ext)
}

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
  const [viewingFile, setViewingFile] = useState<string | null>(null)

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
            <div className="flex items-center gap-3">
              {isViewableFile(a.name) && (
                <button
                  onClick={() => setViewingFile(a.name)}
                  className="text-gray-500 hover:text-indigo-400 transition-colors"
                  title="View file"
                >
                  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M2.036 12.322a1.012 1.012 0 0 1 0-.639C3.423 7.51 7.36 4.5 12 4.5c4.638 0 8.573 3.007 9.963 7.178.07.207.07.431 0 .639C20.577 16.49 16.64 19.5 12 19.5c-4.638 0-8.573-3.007-9.963-7.178Z" />
                    <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z" />
                  </svg>
                </button>
              )}
              <span className="text-gray-500 text-xs tabular-nums">{formatSize(a.size)}</span>
            </div>
          </li>
        ))}
      </ul>
      {viewingFile && (
        <ArtifactViewerDialog
          namespace={namespace}
          runName={runName}
          fileName={viewingFile}
          onClose={() => setViewingFile(null)}
        />
      )}
    </div>
  )
}
