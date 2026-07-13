import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PlanApprovalBanner } from './PlanApprovalBanner'
import { ApiError } from '@/api/client'
import type { PlanDraft } from '@/types/plan'

vi.mock('sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

vi.mock('@/hooks/usePlan', () => ({
  usePlan: vi.fn(),
  useApprovePlan: vi.fn(),
  useCancelPlan: vi.fn(),
}))

import { toast } from 'sonner'
import { usePlan, useApprovePlan, useCancelPlan } from '@/hooks/usePlan'

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
    },
    templates: [],
    ...overrides,
  }
}

describe('PlanApprovalBanner', () => {
  const approveMutate = vi.fn()
  const cancelMutate = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useApprovePlan).mockReturnValue({ mutate: approveMutate, isPending: false } as any)
    vi.mocked(useCancelPlan).mockReturnValue({ mutate: cancelMutate, isPending: false } as any)
  })

  it('renders the manifest layers and nodes from the plan draft', () => {
    vi.mocked(usePlan).mockReturnValue({ data: makeDraft(), isLoading: false } as any)
    render(<PlanApprovalBanner instanceId="inst-1" status="waiting_approval" />)

    expect(screen.getByText('Layer 0')).toBeInTheDocument()
    expect(screen.getByText('analyzer')).toBeInTheDocument()
    expect(screen.getByText('setup-analyzer')).toBeInTheDocument()
    expect(screen.getByText('Investigate the codebase')).toBeInTheDocument()
  })

  it('shows a loading spinner while the draft is loading', () => {
    vi.mocked(usePlan).mockReturnValue({ data: undefined, isLoading: true } as any)
    render(<PlanApprovalBanner instanceId="inst-1" status="planning" />)
    expect(screen.queryByText('Layer 0')).not.toBeInTheDocument()
  })

  it('shows a fallback message when there is no manifest yet', () => {
    vi.mocked(usePlan).mockReturnValue({ data: makeDraft({ manifest: undefined }), isLoading: false } as any)
    render(<PlanApprovalBanner instanceId="inst-1" status="planning" />)
    expect(screen.getByText('No plan draft yet.')).toBeInTheDocument()
  })

  it('Approve calls the mutation pinned to head.latest_revision', async () => {
    const user = userEvent.setup()
    vi.mocked(usePlan).mockReturnValue({ data: makeDraft(), isLoading: false } as any)
    render(<PlanApprovalBanner instanceId="inst-1" status="waiting_approval" />)

    await user.click(screen.getByRole('button', { name: /Approve \(rev 2\)/ }))

    expect(approveMutate).toHaveBeenCalledWith(
      { instanceId: 'inst-1', params: { revision: 2 } },
      expect.objectContaining({ onError: expect.any(Function) })
    )
  })

  it('Cancel Plan opens a confirm dialog and calls the mutation on confirm', async () => {
    const user = userEvent.setup()
    vi.mocked(usePlan).mockReturnValue({ data: makeDraft(), isLoading: false } as any)
    render(<PlanApprovalBanner instanceId="inst-1" status="waiting_approval" />)

    await user.click(screen.getByRole('button', { name: /Cancel Plan/ }))
    expect(screen.getByText('Cancel this draft plan? The workflow instance will remain parked until a new plan is drafted.')).toBeInTheDocument()

    const confirmButtons = screen.getAllByRole('button', { name: 'Cancel Plan' })
    await user.click(confirmButtons[confirmButtons.length - 1])
    expect(cancelMutate).toHaveBeenCalledWith(
      { instanceId: 'inst-1' },
      expect.objectContaining({ onError: expect.any(Function) })
    )
  })

  it('disables Approve/Cancel when the head is no longer a draft', () => {
    vi.mocked(usePlan).mockReturnValue({
      data: makeDraft({ head: { ...makeDraft().head!, status: 'approved' } }),
      isLoading: false,
    } as any)
    render(<PlanApprovalBanner instanceId="inst-1" status="waiting_approval" />)

    expect(screen.getByRole('button', { name: /Approve/ })).toBeDisabled()
    expect(screen.getByRole('button', { name: /Cancel Plan/ })).toBeDisabled()
  })

  it('a 409 error from approve surfaces the stale-plan message via toast, not a generic error', () => {
    vi.mocked(usePlan).mockReturnValue({ data: makeDraft(), isLoading: false } as any)
    approveMutate.mockImplementation((_vars, opts) => {
      opts.onError(new ApiError(409, 'revision mismatch'))
    })
    render(<PlanApprovalBanner instanceId="inst-1" status="waiting_approval" />)

    screen.getByRole('button', { name: /Approve/ }).click()

    expect(toast.error).toHaveBeenCalledWith(
      'The plan changed since this page loaded — reload to see the latest revision.'
    )
    expect(toast.error).not.toHaveBeenCalledWith(expect.stringContaining('Failed to approve plan'))
  })

  it('a non-409 error from approve surfaces a generic failure message', () => {
    vi.mocked(usePlan).mockReturnValue({ data: makeDraft(), isLoading: false } as any)
    approveMutate.mockImplementation((_vars, opts) => {
      opts.onError(new Error('network down'))
    })
    render(<PlanApprovalBanner instanceId="inst-1" status="waiting_approval" />)

    screen.getByRole('button', { name: /Approve/ }).click()

    expect(toast.error).toHaveBeenCalledWith('Failed to approve plan: network down')
  })
})
