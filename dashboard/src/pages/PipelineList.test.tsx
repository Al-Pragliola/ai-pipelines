import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import PipelineList from './PipelineList'

function makePRReviewPipeline() {
  return {
    name: 'pr-review-pipeline',
    namespace: 'default',
    triggerType: 'PR Review',
    triggerInfo: 'my-org/my-repo (reviewer: review-bot)',
    pollInterval: '30s',
    activeRuns: 1,
    totalRuns: 5,
  }
}

function makePRReviewRun() {
  return {
    name: 'pr-review-run-1',
    namespace: 'default',
    pipeline: 'pr-review-pipeline',
    issueNumber: 0,
    issueKey: '#PR-42',
    issueTitle: 'Add feature X',
    phase: 'Running',
    currentStep: 'review',
    branch: 'ai/review-pr-42',
    prNumber: 42,
    prAuthor: 'contributor',
    prTitle: 'Add feature X',
    startedAt: '2026-01-01T00:00:00Z',
    finishedAt: null,
    steps: [],
  }
}

function renderPipelineList(pipelines: Record<string, unknown>[], runsMap: Record<string, unknown[]>) {
  // Mock fetch: first call returns pipelines, subsequent calls return runs per pipeline
  const fetchMock = vi.spyOn(globalThis, 'fetch')

  // /api/pipelines
  fetchMock.mockResolvedValueOnce({
    ok: true,
    json: () => Promise.resolve(pipelines),
    text: () => Promise.resolve(JSON.stringify(pipelines)),
  } as Response)

  // /api/pipelines/{ns}/{name}/runs for each pipeline
  for (const p of pipelines) {
    const key = `${(p as { namespace: string }).namespace}/${(p as { name: string }).name}`
    fetchMock.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(runsMap[key] ?? []),
      text: () => Promise.resolve(JSON.stringify(runsMap[key] ?? [])),
    } as Response)
  }

  return render(
    <MemoryRouter initialEntries={['/']}>
      <PipelineList />
    </MemoryRouter>
  )
}

beforeEach(() => {
  vi.restoreAllMocks()
})

describe('PipelineList - PR Review trigger display', () => {
  it('shows "PR Review" as the trigger type for PR review pipelines', async () => {
    const pipeline = makePRReviewPipeline()
    renderPipelineList([pipeline], {})

    // The trigger type should display "PR Review:" followed by the trigger info
    expect(await screen.findByText(/PR Review/)).toBeInTheDocument()
  })

  it('shows owner/repo and reviewer in trigger info', async () => {
    const pipeline = makePRReviewPipeline()
    renderPipelineList([pipeline], {})

    expect(await screen.findByText(/my-org\/my-repo/)).toBeInTheDocument()
    expect(screen.getByText(/review-bot/)).toBeInTheDocument()
  })

  it('shows PR number and author in run list for PR review runs', async () => {
    const pipeline = makePRReviewPipeline()
    const run = makePRReviewRun()
    const key = `${pipeline.namespace}/${pipeline.name}`

    renderPipelineList([pipeline], { [key]: [run] })

    // The run row should display PR reference (#PR-42) instead of generic issue ref
    expect(await screen.findByText(/#PR-42/)).toBeInTheDocument()
  })

  it('displays PR author when available in run data', async () => {
    const pipeline = makePRReviewPipeline()
    const run = makePRReviewRun()
    const key = `${pipeline.namespace}/${pipeline.name}`

    renderPipelineList([pipeline], { [key]: [run] })

    // The run should show the PR author (contributor) somewhere in the row.
    // This will fail until the frontend displays prAuthor for PR review runs.
    await screen.findByText(/#PR-42/)
    expect(screen.getByText(/contributor/)).toBeInTheDocument()
  })
})
