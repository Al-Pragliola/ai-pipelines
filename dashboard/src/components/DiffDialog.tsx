import { useState, useRef, useEffect, useCallback } from 'react'

// --- Diff parsing types ---

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

// --- Diff parser ---

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

// --- File tree grouping ---

interface TreeNode {
  name: string
  path: string
  isDir: boolean
  children: TreeNode[]
  file?: DiffFile
}

function buildFileTree(files: DiffFile[]): TreeNode[] {
  const root: TreeNode = { name: '', path: '', isDir: true, children: [] }

  for (const file of files) {
    const parts = file.path.split('/')
    let current = root

    for (let i = 0; i < parts.length; i++) {
      const part = parts[i]
      const isLast = i === parts.length - 1
      const childPath = parts.slice(0, i + 1).join('/')

      let child = current.children.find(c => c.name === part)
      if (!child) {
        child = {
          name: part,
          path: childPath,
          isDir: !isLast,
          children: [],
          file: isLast ? file : undefined,
        }
        current.children.push(child)
      }
      current = child
    }
  }

  // Collapse single-child directories
  function collapse(node: TreeNode): TreeNode {
    node.children = node.children.map(collapse)
    if (node.isDir && node.children.length === 1 && node.children[0].isDir) {
      const child = node.children[0]
      return {
        ...child,
        name: node.name + '/' + child.name,
        path: child.path,
      }
    }
    return node
  }

  root.children = root.children.map(collapse)
  // Sort: directories first, then alphabetical
  function sortTree(nodes: TreeNode[]): TreeNode[] {
    return nodes.sort((a, b) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1
      return a.name.localeCompare(b.name)
    }).map(n => ({ ...n, children: sortTree(n.children) }))
  }

  return sortTree(root.children)
}

// --- Components ---

function FileTreeNode({
  node,
  selectedPath,
  onSelect,
  depth = 0,
}: {
  node: TreeNode
  selectedPath: string
  onSelect: (path: string) => void
  depth?: number
}) {
  const [expanded, setExpanded] = useState(true)
  const pl = depth * 12

  if (node.isDir) {
    return (
      <div>
        <button
          onClick={() => setExpanded(!expanded)}
          className="w-full flex items-center gap-1.5 px-2 py-1 text-xs text-gray-400 hover:bg-gray-800/50 transition-colors"
          style={{ paddingLeft: `${pl + 8}px` }}
        >
          <svg
            className={`w-3 h-3 flex-shrink-0 transition-transform ${expanded ? 'rotate-90' : ''}`}
            fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor"
          >
            <path strokeLinecap="round" strokeLinejoin="round" d="m8.25 4.5 7.5 7.5-7.5 7.5" />
          </svg>
          <svg className="w-3.5 h-3.5 flex-shrink-0 text-blue-400" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" d="M2.25 12.75V12A2.25 2.25 0 0 1 4.5 9.75h15A2.25 2.25 0 0 1 21.75 12v.75m-8.69-6.44-2.12-2.12a1.5 1.5 0 0 0-1.061-.44H4.5A2.25 2.25 0 0 0 2.25 6v12a2.25 2.25 0 0 0 2.25 2.25h15A2.25 2.25 0 0 0 21.75 18V9a2.25 2.25 0 0 0-2.25-2.25h-5.379a1.5 1.5 0 0 1-1.06-.44Z" />
          </svg>
          <span className="truncate">{node.name}</span>
        </button>
        {expanded && node.children.map(child => (
          <FileTreeNode
            key={child.path}
            node={child}
            selectedPath={selectedPath}
            onSelect={onSelect}
            depth={depth + 1}
          />
        ))}
      </div>
    )
  }

  // File node
  const file = node.file!
  const isSelected = selectedPath === file.path
  const nameColor =
    file.status === 'added' ? 'text-green-400' :
    file.status === 'deleted' ? 'text-red-400' :
    'text-gray-300'

  return (
    <button
      onClick={() => onSelect(file.path)}
      className={`w-full flex items-center gap-1.5 px-2 py-1 text-xs transition-colors ${
        isSelected ? 'bg-gray-800 text-white' : 'text-gray-400 hover:bg-gray-800/50'
      }`}
      style={{ paddingLeft: `${pl + 8}px` }}
    >
      <svg className="w-3.5 h-3.5 flex-shrink-0 text-gray-500" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m2.25 0H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z" />
      </svg>
      <span className={`truncate ${nameColor}`}>{node.name}</span>
      <span className="ml-auto flex gap-1.5 flex-shrink-0">
        {file.additions > 0 && <span className="text-green-500">+{file.additions}</span>}
        {file.deletions > 0 && <span className="text-red-500">-{file.deletions}</span>}
      </span>
    </button>
  )
}

export function FileDiff({ file, collapsed, onToggle }: { file: DiffFile; collapsed: boolean; onToggle: () => void }) {

  const statusLabel =
    file.status === 'added' ? 'New file' :
    file.status === 'deleted' ? 'Deleted' :
    file.status === 'renamed' ? `Renamed from ${file.oldPath}` :
    'Modified'

  const statusColor =
    file.status === 'added' ? 'text-green-400' :
    file.status === 'deleted' ? 'text-red-400' :
    'text-gray-400'

  return (
    <div className="border-b border-gray-800 last:border-b-0">
      <button
        onClick={onToggle}
        className="sticky top-0 z-10 w-full flex items-center justify-between px-4 py-2 bg-gray-900 border-b border-gray-800 hover:bg-gray-800/80 transition-colors cursor-pointer"
      >
        <div className="flex items-center gap-2 min-w-0">
          <svg
            className={`w-3.5 h-3.5 flex-shrink-0 text-gray-500 transition-transform ${collapsed ? '' : 'rotate-90'}`}
            fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor"
          >
            <path strokeLinecap="round" strokeLinejoin="round" d="m8.25 4.5 7.5 7.5-7.5 7.5" />
          </svg>
          <span className="font-mono text-sm text-gray-200 truncate">{file.path}</span>
          <span className={`text-xs ${statusColor}`}>{statusLabel}</span>
        </div>
        <div className="flex gap-2 text-xs flex-shrink-0">
          {file.additions > 0 && <span className="text-green-500">+{file.additions}</span>}
          {file.deletions > 0 && <span className="text-red-500">-{file.deletions}</span>}
        </div>
      </button>
      {!collapsed && (
        <div className="font-mono text-xs leading-5">
          {file.hunks.map((hunk, hi) => (
            <div key={hi}>
              {hunk.lines.map((line, li) => {
                if (line.type === 'hunk-header') {
                  return (
                    <div key={li} className="bg-blue-500/10 text-blue-400 px-4 py-0.5 select-none">
                      {line.content}
                    </div>
                  )
                }

                const bgClass =
                  line.type === 'add' ? 'bg-green-500/10' :
                  line.type === 'delete' ? 'bg-red-500/10' :
                  ''
                const textClass =
                  line.type === 'add' ? 'text-green-300' :
                  line.type === 'delete' ? 'text-red-300' :
                  'text-gray-400'
                const prefix =
                  line.type === 'add' ? '+' :
                  line.type === 'delete' ? '-' :
                  ' '

                return (
                  <div key={li} className={`flex ${bgClass} hover:brightness-125`}>
                    <span className="w-12 text-right pr-1 text-gray-600 select-none flex-shrink-0 border-r border-gray-800/50">
                      {line.oldLine ?? ''}
                    </span>
                    <span className="w-12 text-right pr-1 text-gray-600 select-none flex-shrink-0 border-r border-gray-800/50">
                      {line.newLine ?? ''}
                    </span>
                    <span className={`pl-2 pr-4 ${textClass} whitespace-pre`}>
                      <span className="select-none">{prefix}</span>{line.content}
                    </span>
                  </div>
                )
              })}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// --- Main dialog ---

export default function DiffDialog({
  diff,
  onClose,
}: {
  diff: string
  onClose: () => void
}) {
  const files = parseDiff(diff)
  const [selectedPath, setSelectedPath] = useState(files[0]?.path ?? '')
  const [collapsedFiles, setCollapsedFiles] = useState<Set<string>>(new Set())
  const diffPanelRef = useRef<HTMLDivElement>(null)
  const fileRefs = useRef<Map<string, HTMLDivElement>>(new Map())

  const totalAdditions = files.reduce((s, f) => s + f.additions, 0)
  const totalDeletions = files.reduce((s, f) => s + f.deletions, 0)
  const tree = buildFileTree(files)

  const toggleCollapsed = useCallback((path: string) => {
    setCollapsedFiles(prev => {
      const next = new Set(prev)
      if (next.has(path)) next.delete(path)
      else next.add(path)
      return next
    })
  }, [])

  const handleFileSelect = useCallback((path: string) => {
    setSelectedPath(path)
    // Expand the file if collapsed
    setCollapsedFiles(prev => {
      if (!prev.has(path)) return prev
      const next = new Set(prev)
      next.delete(path)
      return next
    })
    // Scroll to file after a tick (to allow expand to render)
    setTimeout(() => {
      const el = fileRefs.current.get(path)
      if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }, 50)
  }, [])

  // Close on Escape
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [onClose])

  return (
    <>
      {/* Backdrop */}
      <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm" onClick={onClose} />

      {/* Dialog */}
      <div className="fixed inset-4 z-50 flex flex-col bg-gray-950 rounded-xl border border-gray-800 overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-gray-800 bg-gray-900 flex-shrink-0">
          <div className="flex items-center gap-4">
            <h2 className="text-sm font-medium text-gray-200">Changes to be pushed</h2>
            <div className="flex items-center gap-3 text-xs">
              <span className="text-gray-500">{files.length} file{files.length !== 1 ? 's' : ''}</span>
              <span className="text-green-500">+{totalAdditions}</span>
              <span className="text-red-500">-{totalDeletions}</span>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded text-gray-400 hover:text-white hover:bg-gray-800 transition-colors"
          >
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Body */}
        <div className="flex flex-1 min-h-0">
          {/* File tree */}
          <div className="w-72 flex-shrink-0 border-r border-gray-800 overflow-y-auto bg-gray-950">
            <div className="py-1">
              {tree.map(node => (
                <FileTreeNode
                  key={node.path}
                  node={node}
                  selectedPath={selectedPath}
                  onSelect={handleFileSelect}
                />
              ))}
            </div>
          </div>

          {/* Diff view */}
          <div ref={diffPanelRef} className="flex-1 overflow-auto">
            {files.map(file => (
              <div
                key={file.path}
                ref={el => { if (el) fileRefs.current.set(file.path, el) }}
              >
                <FileDiff
                  file={file}
                  collapsed={collapsedFiles.has(file.path)}
                  onToggle={() => toggleCollapsed(file.path)}
                />
              </div>
            ))}
          </div>
        </div>
      </div>
    </>
  )
}
