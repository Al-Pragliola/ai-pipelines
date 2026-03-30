export interface DiffLine {
  type: 'add' | 'delete' | 'context' | 'hunk-header'
  content: string
  oldLine?: number
  newLine?: number
}

export interface DiffHunk {
  header: string
  oldStart: number
  newStart: number
  lines: DiffLine[]
}

export interface DiffFile {
  path: string
  status: 'added' | 'deleted' | 'modified' | 'renamed'
  oldPath?: string
  additions: number
  deletions: number
  hunks: DiffHunk[]
}

export function parseDiff(raw: string): DiffFile[] {
  const files: DiffFile[] = []
  // Split by "diff --git" lines
  const blocks = raw.split(/^diff --git /m).filter(Boolean)

  for (const block of blocks) {
    const lines = block.split('\n')
    // First line: "a/path b/path"
    const headerMatch = lines[0].match(/a\/(.+?) b\/(.+)/)
    if (!headerMatch) continue

    const oldPath = headerMatch[1]
    const newPath = headerMatch[2]

    // Determine status from diff header lines
    let status: DiffFile['status'] = 'modified'
    for (const line of lines.slice(1, 6)) {
      if (line.startsWith('new file')) { status = 'added'; break }
      if (line.startsWith('deleted file')) { status = 'deleted'; break }
      if (line.startsWith('rename from')) { status = 'renamed'; break }
    }

    const file: DiffFile = {
      path: newPath,
      oldPath: oldPath !== newPath ? oldPath : undefined,
      status,
      additions: 0,
      deletions: 0,
      hunks: [],
    }

    // Parse hunks
    let currentHunk: DiffHunk | null = null
    let oldLine = 0
    let newLine = 0

    for (const line of lines) {
      const hunkMatch = line.match(/^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@(.*)/)
      if (hunkMatch) {
        currentHunk = {
          header: line,
          oldStart: parseInt(hunkMatch[1], 10),
          newStart: parseInt(hunkMatch[2], 10),
          lines: [],
        }
        oldLine = currentHunk.oldStart
        newLine = currentHunk.newStart
        file.hunks.push(currentHunk)
        currentHunk.lines.push({ type: 'hunk-header', content: line })
        continue
      }

      if (!currentHunk) continue

      if (line.startsWith('+')) {
        currentHunk.lines.push({ type: 'add', content: line.slice(1), newLine })
        newLine++
        file.additions++
      } else if (line.startsWith('-')) {
        currentHunk.lines.push({ type: 'delete', content: line.slice(1), oldLine })
        oldLine++
        file.deletions++
      } else if (line.startsWith(' ')) {
        currentHunk.lines.push({ type: 'context', content: line.slice(1), oldLine, newLine })
        oldLine++
        newLine++
      }
      // Skip "\ No newline at end of file" and other meta lines
    }

    files.push(file)
  }

  return files
}
