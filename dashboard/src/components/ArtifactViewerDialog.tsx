import { useState, useEffect, useCallback } from 'react'
import { PrismLight as SyntaxHighlighter } from 'react-syntax-highlighter'
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism'
import { MarkdownText } from './StreamChat'

const EXT_TO_LANG: Record<string, string> = {
  go: 'go',
  ts: 'typescript', tsx: 'typescript',
  js: 'javascript', jsx: 'javascript',
  sh: 'bash', bash: 'bash',
  yaml: 'yaml', yml: 'yaml',
  json: 'json',
  py: 'python',
  diff: 'diff',
}

function getLanguage(name: string): string | undefined {
  const ext = name.split('.').pop()?.toLowerCase() || ''
  return EXT_TO_LANG[ext]
}

function isMarkdown(name: string): boolean {
  return name.toLowerCase().endsWith('.md')
}

export default function ArtifactViewerDialog({
  namespace, runName, fileName, onClose,
}: {
  namespace: string
  runName: string
  fileName: string
  onClose: () => void
}) {
  const [content, setContent] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const fetchContent = useCallback(async () => {
    try {
      const res = await fetch(`/api/runs/${namespace}/${runName}/artifacts/view/${fileName}`)
      if (!res.ok) throw new Error(await res.text())
      setContent(await res.text())
      setError('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load file')
    } finally {
      setLoading(false)
    }
  }, [namespace, runName, fileName])

  useEffect(() => {
    fetchContent()
  }, [fetchContent])

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [onClose])

  const lang = getLanguage(fileName)
  const md = isMarkdown(fileName)

  return (
    <>
      {/* Backdrop */}
      <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm" onClick={onClose} />

      {/* Dialog */}
      <div className="fixed inset-4 z-50 flex flex-col bg-gray-950 rounded-xl border border-gray-800 overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-gray-800 bg-gray-900 flex-shrink-0">
          <span className="text-sm font-mono text-gray-300 truncate">{fileName}</span>
          <button
            onClick={onClose}
            className="p-1.5 rounded text-gray-400 hover:text-white hover:bg-gray-800 transition-colors flex-shrink-0 ml-4"
          >
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-auto p-4">
          {loading && <div className="text-sm text-gray-500">Loading...</div>}
          {error && <div className="text-sm text-red-400">{error}</div>}
          {content !== null && (
            md ? (
              <div className="px-2">
                <MarkdownText text={content} />
              </div>
            ) : lang ? (
              <SyntaxHighlighter
                language={lang}
                style={oneDark}
                showLineNumbers
                customStyle={{ margin: 0, background: 'transparent', fontSize: '0.75rem' }}
                lineNumberStyle={{ color: '#4b5563', minWidth: '2.5rem', paddingRight: '0.75rem', textAlign: 'right', userSelect: 'none' }}
              >
                {content}
              </SyntaxHighlighter>
            ) : (
              <div className="font-mono text-xs leading-relaxed">
                {content.split('\n').map((line, i) => (
                  <div key={i} className="flex hover:bg-gray-800/30">
                    <span className="text-gray-600 select-none w-10 text-right pr-3 flex-shrink-0">{i + 1}</span>
                    <span className="text-gray-300 whitespace-pre-wrap break-all">{line}</span>
                  </div>
                ))}
              </div>
            )
          )}
        </div>
      </div>
    </>
  )
}
