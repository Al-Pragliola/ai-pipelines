import { PrismLight as SyntaxHighlighter } from 'react-syntax-highlighter'
import go from 'react-syntax-highlighter/dist/esm/languages/prism/go'
import typescript from 'react-syntax-highlighter/dist/esm/languages/prism/typescript'
import javascript from 'react-syntax-highlighter/dist/esm/languages/prism/javascript'
import bash from 'react-syntax-highlighter/dist/esm/languages/prism/bash'
import yaml from 'react-syntax-highlighter/dist/esm/languages/prism/yaml'
import json from 'react-syntax-highlighter/dist/esm/languages/prism/json'
import python from 'react-syntax-highlighter/dist/esm/languages/prism/python'
import diff from 'react-syntax-highlighter/dist/esm/languages/prism/diff'

import {
  type StreamEntry,
  tryParseLines,
  isStreamJson,
  ChatMessage,
  ResultSummary,
} from './StreamChat'

SyntaxHighlighter.registerLanguage('go', go)
SyntaxHighlighter.registerLanguage('typescript', typescript)
SyntaxHighlighter.registerLanguage('ts', typescript)
SyntaxHighlighter.registerLanguage('javascript', javascript)
SyntaxHighlighter.registerLanguage('js', javascript)
SyntaxHighlighter.registerLanguage('bash', bash)
SyntaxHighlighter.registerLanguage('sh', bash)
SyntaxHighlighter.registerLanguage('shell', bash)
SyntaxHighlighter.registerLanguage('yaml', yaml)
SyntaxHighlighter.registerLanguage('yml', yaml)
SyntaxHighlighter.registerLanguage('json', json)
SyntaxHighlighter.registerLanguage('python', python)
SyntaxHighlighter.registerLanguage('py', python)
SyntaxHighlighter.registerLanguage('diff', diff)

export default function LogViewer({ logs, isActive = false }: { logs: string; isActive?: boolean }) {
  if (!isStreamJson(logs)) {
    // Plain text logs (shell, checkout, push steps)
    const lines = logs.split('\n')
    return (
      <div className="font-mono text-xs leading-relaxed">
        {lines.map((line, i) => (
          <div key={i} className="flex hover:bg-gray-800/30">
            <span className="text-gray-600 select-none w-10 text-right pr-3 flex-shrink-0">{i + 1}</span>
            <span className="text-gray-300 whitespace-pre-wrap break-all">{line}</span>
          </div>
        ))}
      </div>
    )
  }

  // Stream JSON — chat view
  const entries: StreamEntry[] = tryParseLines(logs)

  const hasResult = entries.some(e => e.type === 'result')

  return (
    <div className="space-y-4">
      {entries.map((entry, i) => {
        if (entry.type === 'system') return null
        if (entry.type === 'result') return <ResultSummary key={i} entry={entry} />
        if (entry.type === 'assistant' || entry.type === 'user') {
          return <ChatMessage key={i} entry={entry} />
        }
        return null
      })}
      {isActive && !hasResult && (
        <div className="flex items-center gap-3 px-3 py-2">
          <div className="w-7 h-7 rounded-full bg-purple-500/20 text-purple-400 flex items-center justify-center text-xs font-bold animate-pulse">AI</div>
          <div className="text-sm text-gray-400 animate-pulse">Thinking...</div>
        </div>
      )}
    </div>
  )
}
