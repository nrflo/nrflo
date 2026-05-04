import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithQuery } from '@/test/utils'
import { ScheduleForm } from './ScheduleForm'
import * as workflowsApi from '@/api/workflows'
import type { WorkflowDefSummary } from '@/types/workflow'
import type { WorkflowChain } from '@/types/workflowChain'
import type { ScheduledTask } from '@/types/schedules'

const mockCreateMutate = vi.fn()
const mockUpdateMutate = vi.fn()
const mockUseWorkflowChainsList = vi.fn(() => ({ data: [] as WorkflowChain[], isLoading: false }))

vi.mock('@/hooks/useScheduledTasks', () => ({
  useCreateScheduledTask: () => ({ mutate: mockCreateMutate, isPending: false }),
  useUpdateScheduledTask: () => ({ mutate: mockUpdateMutate, isPending: false }),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) =>
    selector({ currentProject: 'test-project', projectsLoaded: true })
  ),
}))

vi.mock('@/api/workflows')

vi.mock('@/hooks/useWorkflowChains', () => ({
  useWorkflowChainsList: () => mockUseWorkflowChainsList(),
}))

const mockWorkflowDefs: Record<string, WorkflowDefSummary> = {
  'proj-workflow': { scope_type: 'project', description: '', phases: [] },
  'ticket-workflow': { scope_type: 'ticket', description: '', phases: [] },
}

function makeEditTarget(overrides: Partial<ScheduledTask> = {}): ScheduledTask {
  return {
    id: 'task-1',
    project_id: 'test-project',
    name: 'Existing Schedule',
    description: 'desc',
    cron_expression: '0 9 * * 1-5',
    workflows: ['proj-workflow'],
    workflow_chain_ids: [],
    enabled: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeChain(overrides: Partial<WorkflowChain> = {}): WorkflowChain {
  return {
    id: 'chain-1',
    project_id: 'test-project',
    name: 'My Chain',
    description: '',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('ScheduleForm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(workflowsApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
    mockUseWorkflowChainsList.mockReturnValue({ data: [], isLoading: false })
  })

  it('shows invalid cron error and disables Save button', async () => {
    const user = userEvent.setup()
    renderWithQuery(<ScheduleForm open={true} onClose={vi.fn()} />)

    await user.type(screen.getByPlaceholderText('My schedule'), 'Test')
    await user.type(screen.getByPlaceholderText('0 9 * * 1-5'), 'not-a-cron')

    expect(screen.getByText('Invalid cron expression')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Create' })).toBeDisabled()
  })

  it('shows human-readable cron description for valid expression', async () => {
    const user = userEvent.setup()
    renderWithQuery(<ScheduleForm open={true} onClose={vi.fn()} />)

    await user.type(screen.getByPlaceholderText('0 9 * * 1-5'), '* * * * *')
    expect(await screen.findByText(/every minute/i)).toBeInTheDocument()
    expect(screen.queryByText('Invalid cron expression')).not.toBeInTheDocument()
  })

  it('only shows project-scoped workflows, not ticket-scoped', async () => {
    renderWithQuery(<ScheduleForm open={true} onClose={vi.fn()} />)

    expect(await screen.findByText('proj-workflow')).toBeInTheDocument()
    expect(screen.queryByText('ticket-workflow')).not.toBeInTheDocument()
  })

  it('Create button is disabled when no workflow selected', async () => {
    const user = userEvent.setup()
    renderWithQuery(<ScheduleForm open={true} onClose={vi.fn()} />)

    await user.type(screen.getByPlaceholderText('My schedule'), 'Test')
    await user.type(screen.getByPlaceholderText('0 9 * * 1-5'), '* * * * *')
    // no workflow selected

    expect(screen.getByRole('button', { name: 'Create' })).toBeDisabled()
  })

  it('submit dispatches createScheduledTask in create mode', async () => {
    const user = userEvent.setup()
    mockCreateMutate.mockImplementation(
      (_d: unknown, opts: { onSuccess?: () => void }) => opts?.onSuccess?.()
    )
    renderWithQuery(<ScheduleForm open={true} onClose={vi.fn()} />)

    await user.type(screen.getByPlaceholderText('My schedule'), 'New Task')
    await user.type(screen.getByPlaceholderText('0 9 * * 1-5'), '* * * * *')
    await user.click(await screen.findByText('proj-workflow'))

    await user.click(screen.getByRole('button', { name: 'Create' }))

    expect(mockCreateMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'New Task',
        cron_expression: '* * * * *',
        workflows: ['proj-workflow'],
      }),
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })

  it('submit dispatches updateScheduledTask in edit mode', async () => {
    const user = userEvent.setup()
    const editTarget = makeEditTarget()
    mockUpdateMutate.mockImplementation(
      (_d: unknown, opts: { onSuccess?: () => void }) => opts?.onSuccess?.()
    )
    renderWithQuery(<ScheduleForm open={true} onClose={vi.fn()} editTarget={editTarget} />)

    await screen.findByText('proj-workflow')
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))

    expect(mockUpdateMutate).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'task-1' }),
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })

  it('shows "Edit Schedule" title in edit mode', () => {
    renderWithQuery(
      <ScheduleForm open={true} onClose={vi.fn()} editTarget={makeEditTarget()} />
    )
    expect(screen.getByText('Edit Schedule')).toBeInTheDocument()
  })

  it('shows "New Schedule" title in create mode', () => {
    renderWithQuery(<ScheduleForm open={true} onClose={vi.fn()} />)
    expect(screen.getByText('New Schedule')).toBeInTheDocument()
  })
})

describe('ScheduleForm - Workflow Chains', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(workflowsApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
    mockUseWorkflowChainsList.mockReturnValue({ data: [], isLoading: false })
  })

  it('shows "No workflow chains found" when chains list is empty', () => {
    renderWithQuery(<ScheduleForm open={true} onClose={vi.fn()} />)
    expect(screen.getByText('No workflow chains found')).toBeInTheDocument()
  })

  it('displays chains returned by useWorkflowChainsList', async () => {
    mockUseWorkflowChainsList.mockReturnValue({
      data: [makeChain({ id: 'chain-1', name: 'Deploy Pipeline' })],
      isLoading: false,
    })
    renderWithQuery(<ScheduleForm open={true} onClose={vi.fn()} />)
    expect(await screen.findByText('Deploy Pipeline')).toBeInTheDocument()
  })

  it('Create button enabled when only a chain is selected (no workflow)', async () => {
    const user = userEvent.setup()
    mockUseWorkflowChainsList.mockReturnValue({
      data: [makeChain({ id: 'chain-1', name: 'My Chain' })],
      isLoading: false,
    })
    renderWithQuery(<ScheduleForm open={true} onClose={vi.fn()} />)

    await user.type(screen.getByPlaceholderText('My schedule'), 'Test')
    await user.type(screen.getByPlaceholderText('0 9 * * 1-5'), '* * * * *')
    await user.click(await screen.findByText('My Chain'))

    expect(screen.getByRole('button', { name: 'Create' })).not.toBeDisabled()
  })

  it('shows hint when neither workflow nor chain is selected', async () => {
    const user = userEvent.setup()
    mockUseWorkflowChainsList.mockReturnValue({
      data: [makeChain()],
      isLoading: false,
    })
    renderWithQuery(<ScheduleForm open={true} onClose={vi.fn()} />)

    await user.type(screen.getByPlaceholderText('0 9 * * 1-5'), '* * * * *')
    expect(screen.getByText('Select at least one workflow or chain')).toBeInTheDocument()
  })

  it('create mutation includes workflow_chain_ids when chain selected', async () => {
    const user = userEvent.setup()
    mockUseWorkflowChainsList.mockReturnValue({
      data: [makeChain({ id: 'chain-abc', name: 'My Chain' })],
      isLoading: false,
    })
    mockCreateMutate.mockImplementation(
      (_d: unknown, opts: { onSuccess?: () => void }) => opts?.onSuccess?.()
    )
    renderWithQuery(<ScheduleForm open={true} onClose={vi.fn()} />)

    await user.type(screen.getByPlaceholderText('My schedule'), 'Chain Task')
    await user.type(screen.getByPlaceholderText('0 9 * * 1-5'), '* * * * *')
    await user.click(await screen.findByText('My Chain'))
    await user.click(screen.getByRole('button', { name: 'Create' }))

    expect(mockCreateMutate).toHaveBeenCalledWith(
      expect.objectContaining({ workflow_chain_ids: ['chain-abc'] }),
      expect.any(Object)
    )
  })

  it('edit mode initializes chain selection from editTarget.workflow_chain_ids', async () => {
    mockUseWorkflowChainsList.mockReturnValue({
      data: [makeChain({ id: 'chain-xyz', name: 'Preset Chain' })],
      isLoading: false,
    })
    const editTarget = makeEditTarget({ workflow_chain_ids: ['chain-xyz'] })
    renderWithQuery(<ScheduleForm open={true} onClose={vi.fn()} editTarget={editTarget} />)

    const chainItem = await screen.findByText('Preset Chain')
    // The parent button should have the selected styling class
    expect(chainItem.closest('button')).toHaveClass('bg-primary/10')
  })

  it('update mutation includes workflow_chain_ids', async () => {
    const user = userEvent.setup()
    mockUseWorkflowChainsList.mockReturnValue({
      data: [makeChain({ id: 'chain-xyz', name: 'Preset Chain' })],
      isLoading: false,
    })
    mockUpdateMutate.mockImplementation(
      (_d: unknown, opts: { onSuccess?: () => void }) => opts?.onSuccess?.()
    )
    const editTarget = makeEditTarget({ workflow_chain_ids: ['chain-xyz'] })
    renderWithQuery(<ScheduleForm open={true} onClose={vi.fn()} editTarget={editTarget} />)

    await screen.findByText('Preset Chain')
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))

    expect(mockUpdateMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({ workflow_chain_ids: ['chain-xyz'] }),
      }),
      expect.any(Object)
    )
  })
})
