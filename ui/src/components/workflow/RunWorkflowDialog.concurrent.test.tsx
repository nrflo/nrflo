import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RunWorkflowDialog } from './RunWorkflowDialog'
import { renderWithQuery } from '@/test/utils'
import * as workflowApi from '@/api/workflows'
import * as agentDefsApi from '@/api/agentDefs'
import { ApiError } from '@/api/client'
import type { WorkflowDefSummary } from '@/types/workflow'

const mockMutateAsync = vi.fn()

vi.mock('@/hooks/useTickets', () => ({
  useRunWorkflow: () => ({
    mutateAsync: mockMutateAsync,
    isPending: false,
    isError: false,
    error: null,
  }),
}))

vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn(),
}))

vi.mock('@/api/agentDefs', () => ({
  listAgentDefs: vi.fn(),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) =>
    selector({ currentProject: 'test-project', projectsLoaded: true })
  ),
}))

const featureWorkflow: WorkflowDefSummary = {
  description: 'Feature workflow',
  scope_type: 'ticket',
  phases: [{ id: 'implementor', agent: 'implementor', layer: 0 }],
}

const CONCURRENT_ERROR = 'concurrent ticket workflows without worktrees: use force to override'

describe('RunWorkflowDialog — concurrent workflow warning', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({ feature: featureWorkflow })
    vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue([])
  })

  const renderDialog = (props: Partial<React.ComponentProps<typeof RunWorkflowDialog>> = {}) => {
    const onClose = vi.fn()
    const result = renderWithQuery(
      <RunWorkflowDialog open={true} onClose={onClose} ticketId="TEST-1" {...props} />
    )
    return { onClose, ...result }
  }

  /** Wait for workflow to load and Run button to be enabled */
  async function waitForRunEnabled() {
    const runBtn = await screen.findByRole('button', { name: /^run$/i })
    await waitFor(() => expect(runBtn).not.toBeDisabled())
    return runBtn
  }

  it('shows warning banner when 409 concurrent-workflow error received', async () => {
    const user = userEvent.setup()
    mockMutateAsync.mockRejectedValueOnce(new ApiError(409, CONCURRENT_ERROR))
    renderDialog()

    await user.click(await waitForRunEnabled())

    expect(await screen.findByText(/concurrent workflows without worktree isolation/i)).toBeInTheDocument()
    expect(screen.getByText(/git worktrees are disabled/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /proceed anyway/i })).toBeInTheDocument()
  })

  it('clicking "Proceed Anyway" re-submits with force: true', async () => {
    const user = userEvent.setup()
    mockMutateAsync
      .mockRejectedValueOnce(new ApiError(409, CONCURRENT_ERROR))
      .mockResolvedValueOnce({ instance_id: 'inst-1', status: 'running' })
    renderDialog()

    await user.click(await waitForRunEnabled())
    await screen.findByRole('button', { name: /proceed anyway/i })
    await user.click(screen.getByRole('button', { name: /proceed anyway/i }))

    await waitFor(() => expect(mockMutateAsync).toHaveBeenCalledTimes(2))
    expect(mockMutateAsync.mock.calls[1][0].params.force).toBe(true)
  })

  it('force re-submit preserves original workflow selection', async () => {
    const user = userEvent.setup()
    mockMutateAsync
      .mockRejectedValueOnce(new ApiError(409, CONCURRENT_ERROR))
      .mockResolvedValueOnce({ instance_id: 'inst-1', status: 'running' })
    renderDialog()

    await user.click(await waitForRunEnabled())
    await screen.findByRole('button', { name: /proceed anyway/i })
    await user.click(screen.getByRole('button', { name: /proceed anyway/i }))

    await waitFor(() => expect(mockMutateAsync).toHaveBeenCalledTimes(2))
    expect(mockMutateAsync.mock.calls[1][0].params.workflow).toBe('feature')
  })

  it('does not show concurrent warning for non-concurrent 409 errors', async () => {
    const user = userEvent.setup()
    mockMutateAsync.mockRejectedValueOnce(new ApiError(409, 'already running'))
    renderDialog()

    await user.click(await waitForRunEnabled())

    await waitFor(() => expect(mockMutateAsync).toHaveBeenCalled())
    expect(screen.queryByText(/concurrent workflows without worktree isolation/i)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /proceed anyway/i })).not.toBeInTheDocument()
  })

  it('Cancel button inside warning calls onClose', async () => {
    const user = userEvent.setup()
    mockMutateAsync.mockRejectedValueOnce(new ApiError(409, CONCURRENT_ERROR))
    const { onClose } = renderDialog()

    await user.click(await waitForRunEnabled())
    await screen.findByRole('button', { name: /proceed anyway/i })

    // The warning section has its own Cancel (distinct from DialogFooter Cancel)
    // Both call onClose — click the first Cancel visible in the warning area
    const cancelBtns = screen.getAllByRole('button', { name: /^cancel$/i })
    await user.click(cancelBtns[0])

    expect(onClose).toHaveBeenCalled()
  })

  it('warning resets when dialog closes and reopens', async () => {
    const user = userEvent.setup()
    mockMutateAsync.mockRejectedValueOnce(new ApiError(409, CONCURRENT_ERROR))
    const { rerender } = renderWithQuery(
      <RunWorkflowDialog open={true} onClose={vi.fn()} ticketId="TEST-1" />
    )

    await user.click(await waitForRunEnabled())
    await screen.findByText(/concurrent workflows without worktree isolation/i)

    // Close, then reopen
    rerender(<RunWorkflowDialog open={false} onClose={vi.fn()} ticketId="TEST-1" />)
    rerender(<RunWorkflowDialog open={true} onClose={vi.fn()} ticketId="TEST-1" />)

    expect(screen.queryByText(/concurrent workflows without worktree isolation/i)).not.toBeInTheDocument()
  })
})
