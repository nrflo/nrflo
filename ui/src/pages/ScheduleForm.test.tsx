import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithQuery } from '@/test/utils'
import { ScheduleForm } from './ScheduleForm'
import * as workflowsApi from '@/api/workflows'
import type { WorkflowDefSummary } from '@/types/workflow'
import type { ScheduledTask } from '@/types/schedules'

const mockCreateMutate = vi.fn()
const mockUpdateMutate = vi.fn()

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
    enabled: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('ScheduleForm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(workflowsApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
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
