import Markdown from 'react-markdown'
import { PrismLight as SyntaxHighlighter } from 'react-syntax-highlighter'
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism'

export interface StreamEntry {
  type: string
  subtype?: string
  message?: {
    role?: string
    content?: ContentBlock[]
  }
  tool_use_result?: Record<string, unknown>
  result?: string
  total_cost_usd?: number
  num_turns?: number
  duration_ms?: number
}

export interface ContentBlock {
  type: string
  text?: string
  name?: string
  input?: Record<string, unknown>
  content?: string | ContentBlock[]
  tool_use_id?: string
}

export function tryParseLines(raw: string): StreamEntry[] {
  const trimmed = raw.trimStart()

  // Handle JSON array format (e.g. [{"type":"text",...},...]  )
  if (trimmed.startsWith('[')) {
    try {
      const arr = JSON.parse(trimmed)
      if (Array.isArray(arr)) {
        // Wrap bare content blocks into a synthetic assistant message
        if (arr.length > 0 && arr[0].type === 'text') {
          return [{
            type: 'assistant',
            message: { role: 'assistant', content: arr },
          }]
        }
        return arr
      }
    } catch {
      // fall through to line-by-line
    }
  }

  // Line-delimited JSON (one object per line)
  const entries: StreamEntry[] = []
  for (const line of raw.split('\n')) {
    if (!line.trim()) continue
    try {
      entries.push(JSON.parse(line))
    } catch {
      // not JSON, skip
    }
  }
  return entries
}

export function isStreamJson(raw: string): boolean {
  const first = raw.trimStart()
  return first.startsWith('{"type":') || first.startsWith('[{"type":')
}

export interface TodoItem {
  content: string
  status: string
}

export function TodoList({ todos }: { todos: TodoItem[] }) {
  return (
    <div className="space-y-1">
      {todos.map((todo, i) => (
        <div key={i} className="flex items-start gap-2">
          {todo.status === 'completed' ? (
            <span className="text-green-400 mt-0.5">✓</span>
          ) : todo.status === 'in_progress' ? (
            <span className="text-blue-400 mt-0.5 animate-pulse">●</span>
          ) : (
            <span className="text-gray-500 mt-0.5">○</span>
          )}
          <span className={todo.status === 'completed' ? 'text-gray-400' : 'text-gray-300'}>
            {todo.content}
          </span>
        </div>
      ))}
    </div>
  )
}

export function ToolCall({ name, input }: { name: string; input: Record<string, unknown> }) {
  if (name === 'TodoWrite' || name === 'TodoRead') {
    const todos = (input.todos as TodoItem[] | undefined) || []
    return (
      <div className="bg-gray-800/50 rounded px-3 py-2 text-xs font-mono border-l-2 border-indigo-500/50">
        <span className="text-indigo-400 font-semibold">Tasks</span>
        <div className="mt-1">
          <TodoList todos={todos} />
        </div>
      </div>
    )
  }

  let summary = ''
  if (name === 'Read' || name === 'Write' || name === 'Edit') {
    summary = String(input.file_path || '')
  } else if (name === 'Bash') {
    summary = String(input.command || '')
  } else if (name === 'Grep') {
    summary = `/${input.pattern || ''}/ ${input.path || ''}`
  } else if (name === 'Glob') {
    summary = String(input.pattern || '')
  } else {
    summary = JSON.stringify(input, null, 2)
  }
  return (
    <div className="bg-gray-800/50 rounded px-3 py-2 text-xs font-mono border-l-2 border-indigo-500/50">
      <span className="text-indigo-400 font-semibold">{name}</span>
      <pre className="text-gray-400 mt-1 whitespace-pre-wrap break-all">{summary}</pre>
    </div>
  )
}

export function extractText(content: string | ContentBlock[] | undefined): string {
  if (!content) return ''
  if (typeof content === 'string') return content
  return content
    .map(b => b.type === 'text' && b.text ? b.text : '')
    .filter(Boolean)
    .join('\n')
}

export function ToolResult({ content }: { content: string | ContentBlock[] | undefined }) {
  if (!content) return null
  const text = extractText(content)
  if (!text) return null
  const lines = text.split('\n')
  const truncated = lines.length > 15
  const display = truncated ? lines.slice(0, 15).join('\n') + '\n...' : text
  return (
    <div className="bg-gray-900/50 rounded px-3 py-2 text-xs font-mono border-l-2 border-gray-600/50">
      <pre className="text-gray-500 whitespace-pre-wrap break-all">{display}</pre>
    </div>
  )
}

export function normalizeMarkdown(text: string): string {
  // Normalize double-backtick inline code wrapping: `` `code` `` → `code`
  let result = text.replace(/`{2,}\s*`([^`\n]+)`\s*`{2,}/g, '`$1`')

  // Fenced code blocks where content is wrapped in extra backticks
  result = result.replace(/(```\w*\n)`([\s\S]*?)`(\n```)/g, '$1$2$3')

  return result
}

export function MarkdownText({ text }: { text: string }) {
  return (
    <div className="text-sm text-gray-200 prose prose-invert prose-sm max-w-none">
      <Markdown
        components={{
          code({ className, children }) {
            const match = /language-(\w+)/.exec(className || '')
            let code = String(children).replace(/\n$/, '')
            code = code.replace(/^`+/, '').replace(/`+$/, '')
            if (match) {
              return (
                <SyntaxHighlighter
                  style={oneDark}
                  language={match[1]}
                  PreTag="div"
                  customStyle={{ margin: 0, borderRadius: '0.375rem', fontSize: '0.75rem' }}
                >
                  {code}
                </SyntaxHighlighter>
              )
            }
            return (
              <code className="bg-gray-800 px-1.5 py-0.5 rounded text-xs font-mono text-gray-300">
                {code}
              </code>
            )
          },
          pre({ children }) {
            return <>{children}</>
          },
        }}
      >
        {normalizeMarkdown(text)}
      </Markdown>
    </div>
  )
}

export function ChatMessage({ entry }: { entry: StreamEntry }) {
  if (!entry.message?.content) return null

  const isAssistant = entry.message.role === 'assistant'

  return (
    <div className={`flex gap-3 ${isAssistant ? '' : 'opacity-70'}`}>
      <div className={`flex-shrink-0 w-7 h-7 rounded-full flex items-center justify-center text-xs font-bold mt-0.5 ${
        isAssistant ? 'bg-indigo-500/20 text-indigo-400' : 'bg-gray-700/50 text-gray-400'
      }`}>
        {isAssistant ? 'AI' : '>'}
      </div>
      <div className="flex-1 space-y-2 min-w-0">
        {entry.message.content.map((block, i) => {
          if (block.type === 'text' && block.text) {
            return <MarkdownText key={i} text={block.text} />
          }
          if (block.type === 'tool_use' && block.name) {
            return <ToolCall key={i} name={block.name} input={block.input || {}} />
          }
          if (block.type === 'tool_result') {
            return <ToolResult key={i} content={block.content} />
          }
          return null
        })}
      </div>
    </div>
  )
}

export function ResultSummary({ entry }: { entry: StreamEntry }) {
  return (
    <div className="flex gap-3">
      <div className="flex-shrink-0 w-7 h-7 rounded-full flex items-center justify-center text-xs font-bold bg-green-500/20 text-green-400 mt-0.5">
        ✓
      </div>
      <div className="flex-1 space-y-1">
        {entry.result && (
          <MarkdownText text={entry.result} />
        )}
        <div className="flex gap-4 text-xs text-gray-500">
          {entry.duration_ms != null && <span>{(entry.duration_ms / 1000).toFixed(1)}s</span>}
          {entry.num_turns != null && <span>{entry.num_turns} turns</span>}
          {entry.total_cost_usd != null && <span>${entry.total_cost_usd.toFixed(4)}</span>}
        </div>
      </div>
    </div>
  )
}
