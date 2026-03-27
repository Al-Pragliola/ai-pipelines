import { useState, useRef, useEffect, useCallback, useMemo, memo } from 'react'
import {
  type StreamEntry,
  tryParseLines,
  ChatMessage,
  ResultSummary,
  MarkdownText,
} from './StreamChat'
import { parseDiff, FileDiff, type DiffFile } from './DiffDialog'

interface ChatMsg {
  role: 'user' | 'assistant'
  text?: string
  entries?: StreamEntry[]
}

const ChatInput = memo(function ChatInput({ onSend, disabled }: { onSend: (msg: string) => void; disabled: boolean }) {
  const [input, setInput] = useState('')
  const inputRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    if (!disabled) inputRef.current?.focus()
  }, [disabled])

  const handleSend = () => {
    const msg = input.trim()
    if (!msg || disabled) return
    onSend(msg)
    setInput('')
  }

  return (
    <div className="border-t border-gray-800 bg-gray-900 p-3">
      <div className="flex gap-2">
        <textarea
          ref={inputRef}
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={e => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault()
              handleSend()
            }
          }}
          disabled={disabled}
          placeholder={disabled ? 'Waiting for AI...' : 'Ask about the changes or request edits...'}
          className="flex-1 bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm text-gray-200 placeholder-gray-500 resize-none focus:outline-none focus:border-indigo-500 disabled:opacity-50"
          rows={2}
        />
        <button
          onClick={handleSend}
          disabled={disabled || !input.trim()}
          className="px-4 py-2 rounded-lg bg-indigo-600 text-white text-sm font-medium hover:bg-indigo-500 transition-colors disabled:opacity-50 disabled:cursor-not-allowed self-center"
        >
          Send
        </button>
      </div>
    </div>
  )
})

export default function ChatDialog({
  namespace,
  runName,
  onClose,
}: {
  namespace: string
  runName: string
  onClose: () => void
}) {
  const [messages, setMessages] = useState<ChatMsg[]>([])
  const [isStreaming, setIsStreaming] = useState(false)
  const [streamOutput, setStreamOutput] = useState('')
  const [podStatus, setPodStatus] = useState<'creating' | 'waiting' | 'ready' | 'error'>('creating')
  const [error, setError] = useState('')
  const [diff, setDiff] = useState<string | null>(null)
  const [collapsedFiles, setCollapsedFiles] = useState<Set<string>>(new Set())
  const scrollRef = useRef<HTMLDivElement>(null)
  const closingRef = useRef(false)

  const scrollToBottom = useCallback(() => {
    setTimeout(() => scrollRef.current?.scrollTo(0, scrollRef.current.scrollHeight), 50)
  }, [])

  // Fetch diff from chat pod
  const fetchDiff = useCallback(async () => {
    try {
      const res = await fetch(`/api/runs/${namespace}/${runName}/chat/diff`)
      if (res.ok) {
        setDiff(await res.text())
      }
    } catch {
      // best effort
    }
  }, [namespace, runName])

  // Create chat pod on mount
  useEffect(() => {
    let cancelled = false
    const start = async () => {
      try {
        const res = await fetch(`/api/runs/${namespace}/${runName}/chat`, { method: 'POST' })
        if (!res.ok) throw new Error(await res.text())
        if (!cancelled) setPodStatus('waiting')
      } catch (e) {
        if (!cancelled) {
          setError(e instanceof Error ? e.message : 'Failed to create chat session')
          setPodStatus('error')
        }
      }
    }
    start()
    return () => { cancelled = true }
  }, [namespace, runName])

  // Poll until pod is actually running (not just created)
  useEffect(() => {
    if (podStatus !== 'waiting') return
    let cancelled = false
    let attempts = 0
    const poll = async () => {
      attempts++
      if (attempts > 30) { // 60s timeout
        if (!cancelled) {
          setPodStatus('error')
          setError('Chat pod failed to start (timeout)')
        }
        return
      }
      try {
        // Hit the chat/diff endpoint — it verifies the pod is Running before exec'ing
        const res = await fetch(`/api/runs/${namespace}/${runName}/chat/diff`)
        if (cancelled) return
        // 404 = chatPodName not set yet or pod not found → keep polling
        // 503 = pod exists but not Running yet → keep polling
        // Anything else (200, 500) = pod is running and accepting execs → ready
        if (res.status !== 404 && res.status !== 503) {
          setPodStatus('ready')
          return
        }
      } catch {
        // keep polling
      }
    }
    const id = setInterval(poll, 2000)
    poll()
    return () => { cancelled = true; clearInterval(id) }
  }, [podStatus, namespace, runName])

  // Poll diff during streaming so the left panel updates live
  useEffect(() => {
    if (!isStreaming) return
    const id = setInterval(fetchDiff, 3000)
    return () => clearInterval(id)
  }, [isStreaming, fetchDiff])

  // Fetch initial diff when pod is ready
  useEffect(() => {
    if (podStatus === 'ready') fetchDiff()
  }, [podStatus, fetchDiff])

  const sendMessage = useCallback(async (msg: string) => {
    if (!msg) return

    setMessages(prev => [...prev, { role: 'user', text: msg }])
    setIsStreaming(true)
    setStreamOutput('')
    scrollToBottom()

    try {
      const res = await fetch(`/api/runs/${namespace}/${runName}/chat/message`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: msg }),
      })

      if (!res.ok) {
        const errText = await res.text()
        if (res.status === 410) {
          setError(errText)
          setPodStatus('error')
          return
        }
        throw new Error(errText)
      }

      const reader = res.body?.getReader()
      if (!reader) throw new Error('No response stream')

      const decoder = new TextDecoder()
      let accumulated = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        accumulated += decoder.decode(value, { stream: true })
        setStreamOutput(accumulated)
        scrollToBottom()
      }

      const entries = tryParseLines(accumulated)
      setMessages(prev => [...prev, { role: 'assistant', entries }])
      setStreamOutput('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to send message')
    } finally {
      setIsStreaming(false)
      scrollToBottom()
      fetchDiff()
    }
  }, [namespace, runName, scrollToBottom, fetchDiff])

  const handleClose = async () => {
    if (closingRef.current) return
    closingRef.current = true

    try {
      await fetch(`/api/runs/${namespace}/${runName}/chat`, { method: 'DELETE' })
      await fetch(`/api/runs/${namespace}/${runName}/diff/refresh`, { method: 'POST' })
    } catch {
      // best effort
    }
    onClose()
  }

  // No Escape/backdrop close — chat sessions are expensive to recreate.
  // User must click the X button explicitly.

  // Parse streaming output for incremental rendering (memoize — changes with streamOutput, not on keystrokes)
  const streamEntries = useMemo(() => streamOutput ? tryParseLines(streamOutput) : [], [streamOutput])

  // Parse diff for left panel (memoize — changes only when diff text changes)
  const { diffFiles, totalAdditions, totalDeletions } = useMemo(() => {
    const files: DiffFile[] = diff && diff.trim() ? parseDiff(diff) : []
    return {
      diffFiles: files,
      totalAdditions: files.reduce((s, f) => s + f.additions, 0),
      totalDeletions: files.reduce((s, f) => s + f.deletions, 0),
    }
  }, [diff])

  const toggleCollapsed = useCallback((path: string) => {
    setCollapsedFiles(prev => {
      const next = new Set(prev)
      if (next.has(path)) next.delete(path)
      else next.add(path)
      return next
    })
  }, [])

  return (
    <>
      {/* Backdrop */}
      <div className="fixed inset-0 z-50 bg-black/70" />

      {/* Dialog */}
      <div className="fixed inset-4 z-50 flex flex-col bg-gray-950 rounded-xl border border-gray-800 overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-gray-800 bg-gray-900 flex-shrink-0">
          <div className="flex items-center gap-3">
            <svg className="w-5 h-5 text-indigo-400" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M7.5 8.25h9m-9 3H12m-9.75 1.51c0 1.6 1.123 2.994 2.707 3.227 1.087.16 2.185.283 3.293.369V21l4.076-4.076a1.526 1.526 0 0 1 1.037-.443 48.282 48.282 0 0 0 5.68-.494c1.584-.233 2.707-1.626 2.707-3.228V6.741c0-1.602-1.123-2.995-2.707-3.228A48.394 48.394 0 0 0 12 3c-2.392 0-4.744.175-7.043.513C3.373 3.746 2.25 5.14 2.25 6.741v6.018Z" />
            </svg>
            <h2 className="text-sm font-medium text-gray-200">Chat about changes</h2>
            {podStatus === 'creating' && <span className="text-xs text-yellow-400 animate-pulse">Creating session...</span>}
            {podStatus === 'waiting' && <span className="text-xs text-yellow-400 animate-pulse">Starting AI...</span>}
            {podStatus === 'ready' && <span className="text-xs text-green-400">Ready</span>}
            {diffFiles.length > 0 && (
              <div className="flex items-center gap-2 text-xs ml-2">
                <span className="text-gray-500">{diffFiles.length} file{diffFiles.length !== 1 ? 's' : ''}</span>
                <span className="text-green-500">+{totalAdditions}</span>
                <span className="text-red-500">-{totalDeletions}</span>
              </div>
            )}
          </div>
          <button
            onClick={handleClose}
            disabled={isStreaming}
            className="p-1.5 rounded text-gray-400 hover:text-white hover:bg-gray-800 transition-colors disabled:opacity-50"
          >
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Body: diff left, chat right */}
        <div className="flex flex-1 min-h-0">
          {/* Left: diff panel */}
          <div className="w-[45%] flex-shrink-0 border-r border-gray-800 overflow-auto">
            {diffFiles.length === 0 && (
              <div className="p-4 text-sm text-gray-500">
                {diff === null ? 'Loading diff...' : 'No changes yet.'}
              </div>
            )}
            {diffFiles.map(file => (
              <FileDiff
                key={file.path}
                file={file}
                collapsed={collapsedFiles.has(file.path)}
                onToggle={() => toggleCollapsed(file.path)}
              />
            ))}
          </div>

          {/* Right: chat panel */}
          <div className="flex-1 flex flex-col min-w-0">
            {/* Messages */}
            <div ref={scrollRef} className="flex-1 overflow-auto p-4 space-y-4">
              {podStatus === 'error' && (
                <div className="text-red-400 text-sm">{error}</div>
              )}

              {messages.length === 0 && podStatus === 'ready' && !isStreaming && (
                <div className="text-gray-500 text-sm py-8 text-center">
                  Ask questions about the code or request changes.
                  <br />
                  <span className="text-gray-600 text-xs">The AI has access to the workspace and can edit files.</span>
                </div>
              )}

              {messages.map((msg, i) => {
                if (msg.role === 'user') {
                  return (
                    <div key={i} className="flex justify-end">
                      <div className="max-w-[85%] bg-indigo-500/20 border border-indigo-500/30 rounded-lg px-4 py-2">
                        <MarkdownText text={msg.text!} />
                      </div>
                    </div>
                  )
                }
                return (
                  <div key={i} className="space-y-4">
                    {msg.entries?.map((entry, j) => {
                      if (entry.type === 'system') return null
                      if (entry.type === 'result') return <ResultSummary key={j} entry={entry} />
                      if (entry.type === 'assistant' || entry.type === 'user') {
                        return <ChatMessage key={j} entry={entry} />
                      }
                      return null
                    })}
                  </div>
                )
              })}

              {/* Currently streaming output */}
              {isStreaming && streamEntries.length > 0 && (
                <div className="space-y-4">
                  {streamEntries.map((entry, i) => {
                    if (entry.type === 'system') return null
                    if (entry.type === 'result') return <ResultSummary key={i} entry={entry} />
                    if (entry.type === 'assistant' || entry.type === 'user') {
                      return <ChatMessage key={i} entry={entry} />
                    }
                    return null
                  })}
                </div>
              )}

              {isStreaming && (
                <div className="flex gap-3">
                  <div className="flex-shrink-0 w-7 h-7 rounded-full flex items-center justify-center text-xs font-bold bg-indigo-500/20 text-indigo-400 mt-0.5 animate-pulse">
                    AI
                  </div>
                  <div className="text-sm text-gray-400 animate-pulse">Thinking...</div>
                </div>
              )}

              {error && podStatus !== 'error' && (
                <div className="text-red-400 text-sm">{error}</div>
              )}
            </div>

            <ChatInput onSend={sendMessage} disabled={isStreaming || podStatus !== 'ready'} />
          </div>
        </div>
      </div>
    </>
  )
}
