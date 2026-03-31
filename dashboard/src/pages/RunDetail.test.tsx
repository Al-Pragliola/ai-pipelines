import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import RunDetail from './RunDetail'

// The API will return workflow info on steps. We use plain objects
// to simulate the API response (which may have fields not yet in the TS types).
function makeRunWithWorkflow() {
  return {
    name: 'run-1',
    namespace: 'default',
    pipeline: 'my-pipeline',
    issueNumber: 0,
    issueKey: 'GH-1',
    issueTitle: 'Fix bug',
    phase: 'Succeeded',
    currentStep: '',
    branch: 'ai/fix-bug',
    startedAt: '2026-01-01T00:00:00Z',
    finishedAt: '2026-01-01T00:05:00Z',
    durationSeconds: 300,
    steps: [
      {
        name: 'checkout',
        type: 'git-checkout',
        phase: 'Succeeded',
        startedAt: '2026-01-01T00:00:00Z',
        finishedAt: '2026-01-01T00:00:10Z',
        durationSeconds: 10,
        jobName: 'job-checkout',
        attempt: 1,
        message: '',
      },
      {
        name: 'code',
        type: 'ai',
        phase: 'Succeeded',
        startedAt: '2026-01-01T00:00:10Z',
        finishedAt: '2026-01-01T00:04:00Z',
        durationSeconds: 230,
        jobName: 'job-code',
        attempt: 1,
        message: '',
        // Workflow info — these fields will be added to the API response
        workflowRepo: 'org/workflows',
        workflowPath: 'coding/v1',
      },
      {
        name: 'push',
        type: 'git-push',
        phase: 'Succeeded',
        startedAt: '2026-01-01T00:04:00Z',
        finishedAt: '2026-01-01T00:05:00Z',
        durationSeconds: 60,
        jobName: 'job-push',
        attempt: 1,
        message: '',
      },
    ],
  }
}

function makeRunWithoutWorkflow() {
  return {
    name: 'run-2',
    namespace: 'default',
    pipeline: 'my-pipeline',
    issueNumber: 0,
    issueKey: 'GH-2',
    issueTitle: 'Another bug',
    phase: 'Succeeded',
    currentStep: '',
    branch: 'ai/another',
    startedAt: '2026-01-01T00:00:00Z',
    finishedAt: '2026-01-01T00:02:00Z',
    durationSeconds: 120,
    steps: [
      {
        name: 'inline-ai',
        type: 'ai',
        phase: 'Succeeded',
        startedAt: null,
        finishedAt: null,
        jobName: 'job-inline',
        attempt: 1,
        message: '',
      },
    ],
  }
}

function renderRunDetail(run: Record<string, unknown>) {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue({
    ok: true,
    json: () => Promise.resolve(run),
    text: () => Promise.resolve(JSON.stringify(run)),
  } as Response)

  return render(
    <MemoryRouter initialEntries={[`/runs/${(run as { namespace: string }).namespace}/${(run as { name: string }).name}`]}>
      <Routes>
        <Route path="/runs/:namespace/:name" element={<RunDetail />} />
      </Routes>
    </MemoryRouter>
  )
}

beforeEach(() => {
  vi.restoreAllMocks()
})

describe('RunDetail - workflow info display', () => {
  it('shows workflow repo and path for steps that use a workflow', async () => {
    renderRunDetail(makeRunWithWorkflow())

    // The "code" step uses a workflow — its workflow repo should be displayed
    expect(await screen.findByText(/org\/workflows/)).toBeInTheDocument()
    expect(screen.getByText(/coding\/v1/)).toBeInTheDocument()
  })

  it('does not show workflow info for steps without a workflow', async () => {
    renderRunDetail(makeRunWithoutWorkflow())

    await screen.findByText('inline-ai')
    // No workflow badge or info should appear for a plain step
    expect(screen.queryByText(/workflow/i)).not.toBeInTheDocument()
  })
})

describe('StepCard - workflow badge', () => {
  it('shows a workflow badge when the step uses a workflow', async () => {
    renderRunDetail(makeRunWithWorkflow())

    // Wait for the step to render
    await screen.findByText('code')

    // A badge/indicator should mark this step as using a workflow
    const workflowBadges = screen.getAllByText(/workflow/i)
    expect(workflowBadges.length).toBeGreaterThan(0)
  })

  it('does not show a workflow badge for inline prompt steps', async () => {
    renderRunDetail(makeRunWithoutWorkflow())

    await screen.findByText('inline-ai')
    expect(screen.queryByText(/workflow/i)).not.toBeInTheDocument()
  })
})

// --- PR Review trigger tests ---

function makePRReviewRun() {
  return {
    name: 'pr-review-run',
    namespace: 'default',
    pipeline: 'pr-review-pipeline',
    issueNumber: 0,
    issueKey: '#PR-42',
    issueTitle: 'Add feature X',
    phase: 'Running',
    currentStep: 'review',
    branch: 'ai/review-pr-42',
    prNumber: 42,
    prTitle: 'Add feature X',
    prBody: 'This PR adds feature X to the system',
    prAuthor: 'contributor',
    baseBranch: 'main',
    headBranch: 'feature-x',
    startedAt: '2026-01-01T00:00:00Z',
    finishedAt: null,
    durationSeconds: 60,
    steps: [
      {
        name: 'review',
        type: 'ai',
        phase: 'Running',
        startedAt: '2026-01-01T00:00:00Z',
        finishedAt: null,
        durationSeconds: 60,
        jobName: 'job-review',
        attempt: 1,
        message: '',
      },
    ],
  }
}

describe('RunDetail - PR review metadata display', () => {
  it('shows PR number for PR review runs', async () => {
    renderRunDetail(makePRReviewRun())

    // The run detail should show the PR number (e.g., #42 or PR #42)
    expect(await screen.findByText(/#42|PR.*42/)).toBeInTheDocument()
  })

  it('shows PR author for PR review runs', async () => {
    renderRunDetail(makePRReviewRun())

    // The run detail should display the PR author
    await screen.findByText(/review/)
    expect(screen.getByText(/contributor/)).toBeInTheDocument()
  })

  it('shows PR title for PR review runs', async () => {
    renderRunDetail(makePRReviewRun())

    // The PR title should be displayed
    expect(await screen.findByText(/Add feature X/)).toBeInTheDocument()
  })

  it('links to the GitHub PR when PR metadata is present', async () => {
    renderRunDetail(makePRReviewRun())

    // Wait for render
    await screen.findByText(/Add feature X/)

    // There should be a link to the GitHub PR.
    // The link href should point to the PR on GitHub.
    const prLinks = document.querySelectorAll('a[href*="github.com"]')
    expect(prLinks.length).toBeGreaterThan(0)
  })

  it('shows base and head branch info for PR review runs', async () => {
    renderRunDetail(makePRReviewRun())

    await screen.findByText(/review/)
    // Base and head branch should be visible
    expect(screen.getByText(/main/)).toBeInTheDocument()
    expect(screen.getByText(/feature-x/)).toBeInTheDocument()
  })
})
