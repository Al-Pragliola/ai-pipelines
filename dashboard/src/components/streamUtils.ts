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
  content?: string
  artifacts?: string[]
}

export interface ContentBlock {
  type: string
  text?: string
  name?: string
  input?: Record<string, unknown>
  content?: string | ContentBlock[]
  tool_use_id?: string
}

export interface TodoItem {
  content: string
  status: string
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
