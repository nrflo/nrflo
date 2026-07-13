import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PlanApprovalBanner } from './PlanApprovalBanner'
import type { PlanDraft } from '@/types/plan'

vi.mock('sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

vi.mock('@/hooks/usePlan', () => ({
  usePlan: vi.fn(),
  useApprovePlan: vi.fn(),
  useCancelPlan: vi.fn(),
  useRevisePlan: vi.fn(),
}))

import { usePlan, useApprovePlan, useCancelPlan, useRevisePlan } from '@/hooks/usePlan'

function makeDraft(overrides: Partial<PlanDraft> = {}): PlanDraft {
  return {
    head: {
      instance_id: 'inst-1',
      status: 'draft',
      latest_revision: 2,
      approved_revision: 0,
      goal: 'Ship the thing',
      materialized_revision: 0,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    },
    manifest: {
      version: 1,
      goal: 'Ship the thing',
      layers: [
        {
          layer: 0,
          policy: 'all',
          nodes: [{ id: 'analyzer', template: 'setup-analyzer', instructions: 'Investigate the codebase' }],
        },
      ],
      questions: [{ id: 'q1', question: 'Which database?' }],
    },
    questions: [{ id: 'q1', question: 'Which database?' }],
    templates: [],
    ...overrides,
  }
}

describe('PlanApprovalBanner — revise', () => {
  const approveMutate = vi.fn()
  const cancelMutate = vi.fn()
  const reviseMutate = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useApprovePlan).mockReturnValue({ mutate: approveMutate, isPending: false } as any)
    vi.mocked(useCancelPlan).mockReturnValue({ mutate: cancelMutate, isPending: false } as any)
    vi.mocked(useRevisePlan).mockReturnValue({ mutate: reviseMutate, isPending: false } as any)
    vi.mocked(usePlan).mockReturnValue({ data: makeDraft(), isLoading: false } as any)
  })

  it('Revise button opens the revise dialog with the pinned revision and open questions', async () => {
    const user = userEvent.setup()
    render(<PlanApprovalBanner instanceId="inst-1" status="waiting_approval" />)

    expect(screen.queryByText('Revise Plan (rev 2)')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /Revise/ }))

    expect(screen.getByText('Revise Plan (rev 2)')).toBeInTheDocument()
    const answerInput = screen.getByPlaceholderText('Your answer')
    expect(answerInput.previousSibling).toHaveTextContent('Which database?')
  })

  it('submitting the revise dialog calls useRevisePlan pinned to head.latest_revision', async () => {
    const user = userEvent.setup()
    render(<PlanApprovalBanner instanceId="inst-1" status="waiting_approval" />)

    await user.click(screen.getByRole('button', { name: /Revise/ }))
    await user.click(screen.getByRole('button', { name: /submit revision/i }))

    expect(reviseMutate).toHaveBeenCalledWith(
      { instanceId: 'inst-1', params: { revision: 2, feedback: undefined, answers: undefined } },
      expect.anything()
    )
  })

  it('closing the revise dialog unmounts it', async () => {
    const user = userEvent.setup()
    render(<PlanApprovalBanner instanceId="inst-1" status="waiting_approval" />)

    await user.click(screen.getByRole('button', { name: /Revise/ }))
    expect(screen.getByText('Revise Plan (rev 2)')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByText('Revise Plan (rev 2)')).not.toBeInTheDocument()
  })

  it('Revise is disabled when the head is no longer a draft, same as Approve/Cancel', () => {
    vi.mocked(usePlan).mockReturnValue({
      data: makeDraft({ head: { ...makeDraft().head!, status: 'approved' } }),
      isLoading: false,
    } as any)
    render(<PlanApprovalBanner instanceId="inst-1" status="waiting_approval" />)

    expect(screen.getByRole('button', { name: /Revise/ })).toBeDisabled()
    expect(screen.getByRole('button', { name: /Approve/ })).toBeDisabled()
    expect(screen.getByRole('button', { name: /Cancel Plan/ })).toBeDisabled()
  })

  it('useRevisePlan is never called while the dialog is closed', () => {
    render(<PlanApprovalBanner instanceId="inst-1" status="waiting_approval" />)
    expect(reviseMutate).not.toHaveBeenCalled()
  })
})
