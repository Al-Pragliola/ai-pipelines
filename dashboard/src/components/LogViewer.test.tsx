import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import LogViewer from './LogViewer'

describe('LogViewer - artifact file list', () => {
  it('shows artifact file list for completed workflow steps', () => {
    // Workflow steps produce structured artifacts in NDJSON logs.
    // When the result entry contains an artifacts array, LogViewer
    // should display the list of artifact files after completion.
    const logsWithArtifacts = [
      '{"type":"system","content":"Starting workflow"}',
      '{"type":"assistant","content":"Working on the task..."}',
      '{"type":"result","subtype":"success","content":"Done","artifacts":["CLAUDE.md","src/index.ts","tests/index.test.ts"]}',
    ].join('\n')

    render(<LogViewer logs={logsWithArtifacts} isActive={false} />)

    // Should display the artifact files
    expect(screen.getByText(/CLAUDE\.md/)).toBeInTheDocument()
    expect(screen.getByText(/src\/index\.ts/)).toBeInTheDocument()
    expect(screen.getByText(/tests\/index\.test\.ts/)).toBeInTheDocument()
  })

  it('shows artifact section header when artifacts are present', () => {
    const logsWithArtifacts = [
      '{"type":"system","content":"Starting"}',
      '{"type":"result","subtype":"success","content":"Complete","artifacts":["output.txt"]}',
    ].join('\n')

    render(<LogViewer logs={logsWithArtifacts} isActive={false} />)

    // Should have a section or label for artifacts
    expect(screen.getByText(/artifacts/i)).toBeInTheDocument()
  })

  it('does not show artifact section when no artifacts are present', () => {
    const logsWithoutArtifacts = [
      '{"type":"system","content":"Starting"}',
      '{"type":"result","subtype":"success","content":"Done"}',
    ].join('\n')

    render(<LogViewer logs={logsWithoutArtifacts} isActive={false} />)

    expect(screen.queryByText(/artifacts/i)).not.toBeInTheDocument()
  })

  it('does not show artifacts for plain text logs', () => {
    const plainLogs = 'Step 1: checkout\nStep 2: build\nDone'

    render(<LogViewer logs={plainLogs} isActive={false} />)

    expect(screen.queryByText(/artifacts/i)).not.toBeInTheDocument()
  })
})
