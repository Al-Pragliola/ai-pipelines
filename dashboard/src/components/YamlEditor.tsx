import { useRef, useEffect } from 'react'
import { EditorState } from '@codemirror/state'
import { EditorView, lineNumbers, keymap } from '@codemirror/view'
import { defaultKeymap, indentWithTab } from '@codemirror/commands'
import { syntaxHighlighting, indentOnInput, HighlightStyle } from '@codemirror/language'
import { yaml as yamlLang } from '@codemirror/lang-yaml'
import { linter, lintGutter, type Diagnostic } from '@codemirror/lint'
import { tags } from '@lezer/highlight'
import yamlParser from 'js-yaml'

// Tailwind-aligned syntax highlighting
const highlightStyle = HighlightStyle.define([
  { tag: tags.keyword, color: 'rgb(129 140 248)' },         // indigo-400
  { tag: tags.atom, color: 'rgb(251 191 36)' },              // amber-400
  { tag: tags.bool, color: 'rgb(251 191 36)' },              // amber-400
  { tag: tags.number, color: 'rgb(251 146 60)' },            // orange-400
  { tag: tags.string, color: 'rgb(74 222 128)' },            // green-400
  { tag: tags.special(tags.string), color: 'rgb(74 222 128)' },
  { tag: tags.propertyName, color: 'rgb(96 165 250)' },      // blue-400
  { tag: tags.tagName, color: 'rgb(96 165 250)' },           // blue-400
  { tag: tags.comment, color: 'rgb(107 114 128)', fontStyle: 'italic' }, // gray-500
  { tag: tags.meta, color: 'rgb(156 163 175)' },             // gray-400
  { tag: tags.punctuation, color: 'rgb(156 163 175)' },      // gray-400
  { tag: tags.operator, color: 'rgb(156 163 175)' },         // gray-400
  { tag: tags.null, color: 'rgb(251 191 36)' },              // amber-400
])

// Editor chrome matching the dashboard's dark theme
const theme = EditorView.theme({
  '&': {
    fontSize: '13px',
    backgroundColor: 'transparent',
    color: 'rgb(229 231 235)',                                // gray-200
  },
  '.cm-content': {
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
    padding: '16px 0',
    caretColor: 'rgb(129 140 248)',                           // indigo-400
  },
  '.cm-gutters': {
    backgroundColor: 'rgb(17 24 39)',                         // gray-900
    borderRight: '1px solid rgb(31 41 55)',                   // gray-800
    color: 'rgb(75 85 99)',                                   // gray-600
    minWidth: '3.5em',
  },
  '.cm-lineNumbers .cm-gutterElement': {
    padding: '0 8px 0 12px',
  },
  '.cm-activeLineGutter': {
    backgroundColor: 'rgb(31 41 55)',                         // gray-800
    color: 'rgb(156 163 175)',                                // gray-400
  },
  '.cm-activeLine': {
    backgroundColor: 'rgb(31 41 55 / 0.4)',                  // gray-800/40
  },
  '.cm-selectionBackground': {
    backgroundColor: 'rgb(99 102 241 / 0.2) !important',     // indigo-500/20
  },
  '&.cm-focused .cm-selectionBackground': {
    backgroundColor: 'rgb(99 102 241 / 0.25) !important',    // indigo-500/25
  },
  '&.cm-focused': {
    outline: 'none',
  },
  '.cm-cursor': {
    borderLeftColor: 'rgb(129 140 248)',                      // indigo-400
    borderLeftWidth: '2px',
  },
  // Lint styles
  '.cm-lintRange-error': {
    backgroundImage: 'none',
    textDecoration: 'underline wavy rgb(248 113 113)',        // red-400
    textUnderlineOffset: '3px',
  },
  '.cm-lintRange-warning': {
    backgroundImage: 'none',
    textDecoration: 'underline wavy rgb(251 191 36)',         // amber-400
    textUnderlineOffset: '3px',
  },
  '.cm-lint-marker-error': {
    content: '"●"',
    color: 'rgb(248 113 113)',                                // red-400
  },
  '.cm-lint-marker-warning': {
    content: '"●"',
    color: 'rgb(251 191 36)',                                 // amber-400
  },
  '.cm-tooltip': {
    backgroundColor: 'rgb(17 24 39)',                         // gray-900
    border: '1px solid rgb(55 65 81)',                        // gray-700
    borderRadius: '6px',
    color: 'rgb(209 213 219)',                                // gray-300
    fontSize: '12px',
    boxShadow: '0 4px 6px -1px rgb(0 0 0 / 0.3)',
  },
  '.cm-tooltip .cm-diagnostic': {
    padding: '6px 10px',
    borderLeft: 'none',
  },
  '.cm-tooltip .cm-diagnostic-error': {
    borderLeft: '3px solid rgb(248 113 113)',                 // red-400
  },
  '.cm-tooltip .cm-diagnostic-warning': {
    borderLeft: '3px solid rgb(251 191 36)',                  // amber-400
  },
  '.cm-panels': {
    backgroundColor: 'rgb(17 24 39)',                         // gray-900
    color: 'rgb(209 213 219)',                                // gray-300
  },
}, { dark: true })

const yamlLinter = linter((view) => {
  const diagnostics: Diagnostic[] = []
  const text = view.state.doc.toString()
  if (!text.trim()) return diagnostics

  try {
    const parsed = yamlParser.load(text)

    if (parsed && typeof parsed === 'object') {
      const cr = parsed as Record<string, unknown>
      if (!cr.apiVersion)
        diagnostics.push({ from: 0, to: 0, severity: 'warning', message: 'Missing apiVersion' })
      if (!cr.kind)
        diagnostics.push({ from: 0, to: 0, severity: 'warning', message: 'Missing kind' })
      const meta = cr.metadata as Record<string, unknown> | undefined
      if (!meta?.name)
        diagnostics.push({ from: 0, to: 0, severity: 'warning', message: 'Missing metadata.name' })
      const spec = cr.spec as Record<string, unknown> | undefined
      if (!spec) {
        diagnostics.push({ from: 0, to: 0, severity: 'warning', message: 'Missing spec' })
      } else {
        if (!spec.trigger) diagnostics.push({ from: 0, to: 0, severity: 'warning', message: 'Missing spec.trigger' })
        if (!spec.ai) diagnostics.push({ from: 0, to: 0, severity: 'warning', message: 'Missing spec.ai' })
        if (!spec.steps) diagnostics.push({ from: 0, to: 0, severity: 'warning', message: 'Missing spec.steps' })
      }
    }
  } catch (e) {
    if (e instanceof yamlParser.YAMLException && e.mark) {
      const lineNum = Math.min(e.mark.line + 1, view.state.doc.lines)
      const line = view.state.doc.line(lineNum)
      const pos = Math.min(line.from + e.mark.column, line.to)
      diagnostics.push({
        from: pos,
        to: Math.min(pos + 1, line.to),
        severity: 'error',
        message: e.reason || e.message,
      })
    }
  }

  return diagnostics
})

interface YamlEditorProps {
  value: string
  onChange?: (value: string) => void
  readOnly?: boolean
}

export default function YamlEditor({ value, onChange, readOnly }: YamlEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const onChangeRef = useRef(onChange)
  onChangeRef.current = onChange

  useEffect(() => {
    if (!containerRef.current) return

    const extensions = [
        lineNumbers(),
        syntaxHighlighting(highlightStyle),
        yamlLang(),
        theme,
    ]

    if (readOnly) {
      extensions.push(EditorView.editable.of(false), EditorState.readOnly.of(true))
    } else {
      extensions.push(
        indentOnInput(),
        keymap.of([...defaultKeymap, indentWithTab]),
        lintGutter(),
        yamlLinter,
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            onChangeRef.current?.(update.state.doc.toString())
          }
        }),
      )
    }

    const state = EditorState.create({ doc: value, extensions })

    const view = new EditorView({ state, parent: containerRef.current })
    viewRef.current = view

    return () => {
      view.destroy()
      viewRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    const view = viewRef.current
    if (!view) return
    const currentDoc = view.state.doc.toString()
    if (currentDoc !== value) {
      view.dispatch({
        changes: { from: 0, to: currentDoc.length, insert: value },
      })
    }
  }, [value])

  return <div ref={containerRef} />
}
