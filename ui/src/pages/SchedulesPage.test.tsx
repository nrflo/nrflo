import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { SchedulesPage } from './SchedulesPage'
import type { ScheduledTask } from '@/types/schedules'

const mockUseScheduledTasks = vi.fn()
const mockDeleteMutate = vi.fn()
const mockUpdateMutate = vi.fn()
const mockRunNowMutate = vi.fn()
const mockUseIsAdmin = vi.fn().mockReturnValue(true)

vi.mock('@/hooks/useScheduledTasks', () => ({
  useScheduledTasks: () => mockUseScheduledTasks(),
  useDeleteScheduledTask: () => ({ mutate: mockDeleteMutate, isPending: false }),
  useUpdateScheduledTask: () => ({ mutate: mockUpdateMutate, isPending: false }),
  useRunScheduleNow: () => ({ mutate: mockRunNowMutate, isPending: false }),
}))

vi.mock('@/stores/authStore', () => ({
  useIsAdmin: () => mockUseIsAdmin(),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) =>
    selector({ currentProject: 'test-project', projectsLoaded: true })
  ),
}))

vi.mock('./ScheduleForm', () => ({
  ScheduleForm: ({ open, editTarget }: { open: boolean; editTarget?: ScheduledTask }) =>
    open ? <div data-testid="schedule-form">{editTarget ? 'Edit Form' : 'Create Form'}</div> : null,
}))

vi.mock('./ScheduleRunsDialog', () => ({
  ScheduleRunsDialog: ({ open, task }: { open: boolean; task: ScheduledTask }) =>
    open ? <div data-testid="runs-dialog">{task.name}</div> : null,
}))

function makeTask(overrides: Partial<ScheduledTask> = {}): ScheduledTask {
  return {
    id: 'task-1',
    project_id: 'test-project',
    name: 'Daily Build',
    description: '',
    cron_expression: '0 9 * * 1-5',
    workflows: ['feature'],
    workflow_chain_ids: [],
    enabled: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <SchedulesPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('SchedulesPage - Render States', () => {
  beforeEach(() => { vi.clearAllMocks(); mockUseIsAdmin.mockReturnValue(true) })

  it('renders loading spinner', () => {
    mockUseScheduledTasks.mockReturnValue({ data: undefined, isLoading: true, error: null })
    renderPage()
    expect(screen.getByRole('status', { name: /loading/i })).toBeInTheDocument()
  })

  it('renders empty state', () => {
    mockUseScheduledTasks.mockReturnValue({ data: [], isLoading: false, error: null })
    renderPage()
    expect(screen.getByText(/No schedules found/)).toBeInTheDocument()
  })

  it('renders error message', () => {
    mockUseScheduledTasks.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Network error'),
    })
    renderPage()
    expect(screen.getByText('Network error')).toBeInTheDocument()
  })

  it('renders task rows and task count', () => {
    mockUseScheduledTasks.mockReturnValue({
      data: [makeTask(), makeTask({ id: 'task-2', name: 'Weekly Report' })],
      isLoading: false,
      error: null,
    })
    renderPage()
    expect(screen.getByText('Daily Build')).toBeInTheDocument()
    expect(screen.getByText('Weekly Report')).toBeInTheDocument()
    expect(screen.getByText('2 tasks')).toBeInTheDocument()
  })

  it('shows singular task count for single item', () => {
    mockUseScheduledTasks.mockReturnValue({ data: [makeTask()], isLoading: false, error: null })
    renderPage()
    expect(screen.getByText('1 task')).toBeInTheDocument()
  })
})

describe('SchedulesPage - Actions', () => {
  beforeEach(() => { vi.clearAllMocks(); mockUseIsAdmin.mockReturnValue(true) })

  it('clicking New Schedule opens ScheduleForm in create mode', async () => {
    const user = userEvent.setup()
    mockUseScheduledTasks.mockReturnValue({ data: [], isLoading: false, error: null })
    renderPage()

    await user.click(screen.getByRole('button', { name: /New Schedule/ }))
    expect(screen.getByTestId('schedule-form')).toHaveTextContent('Create Form')
  })

  it('clicking Edit button opens ScheduleForm in edit mode', async () => {
    const user = userEvent.setup()
    mockUseScheduledTasks.mockReturnValue({ data: [makeTask()], isLoading: false, error: null })
    renderPage()

    await user.click(screen.getByTitle('Edit'))
    expect(screen.getByTestId('schedule-form')).toHaveTextContent('Edit Form')
  })

  it('clicking View runs opens ScheduleRunsDialog with task name', async () => {
    const user = userEvent.setup()
    mockUseScheduledTasks.mockReturnValue({
      data: [makeTask({ name: 'My Task' })],
      isLoading: false,
      error: null,
    })
    renderPage()

    await user.click(screen.getByTitle('View runs'))
    expect(screen.getByTestId('runs-dialog')).toHaveTextContent('My Task')
  })

  it('clicking Delete opens confirm dialog', async () => {
    const user = userEvent.setup()
    mockUseScheduledTasks.mockReturnValue({ data: [makeTask()], isLoading: false, error: null })
    renderPage()

    await user.click(screen.getByTitle('Delete'))
    expect(screen.getByText('Delete Schedule')).toBeInTheDocument()
    expect(screen.getByText(/Are you sure you want to delete this schedule/)).toBeInTheDocument()
  })

  it('confirming delete calls deleteMutation.mutate with task id', async () => {
    const user = userEvent.setup()
    mockUseScheduledTasks.mockReturnValue({
      data: [makeTask({ id: 'task-abc' })],
      isLoading: false,
      error: null,
    })
    renderPage()

    await user.click(screen.getByTitle('Delete'))
    // Use selector to target the confirm button (has text content) vs trash icon button (has title attr)
    await user.click(screen.getByText('Delete', { selector: 'button' }))
    expect(mockDeleteMutate).toHaveBeenCalledWith('task-abc', expect.any(Object))
  })

  it('toggling enabled calls updateMutation with negated enabled', async () => {
    const user = userEvent.setup()
    mockUseScheduledTasks.mockReturnValue({
      data: [makeTask({ id: 'task-1', enabled: true })],
      isLoading: false,
      error: null,
    })
    renderPage()

    await user.click(screen.getByRole('switch'))
    expect(mockUpdateMutate).toHaveBeenCalledWith({ id: 'task-1', data: { enabled: false } })
  })

  it('clicking Run now calls runNowMutation.mutate with task id', async () => {
    const user = userEvent.setup()
    mockUseScheduledTasks.mockReturnValue({
      data: [makeTask({ id: 'task-xyz' })],
      isLoading: false,
      error: null,
    })
    renderPage()

    await user.click(screen.getByTitle('Run now'))
    expect(mockRunNowMutate).toHaveBeenCalledWith('task-xyz')
  })
})

describe('SchedulesPage - Viewer Role', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseIsAdmin.mockReturnValue(false)
    mockUseScheduledTasks.mockReturnValue({ data: [makeTask()], isLoading: false, error: null })
  })

  it('shows ReadOnlyHint banner', () => {
    renderPage()
    expect(screen.getByText('Read-only — admin required to make changes.')).toBeInTheDocument()
  })

  it('hides New Schedule button', () => {
    renderPage()
    expect(screen.queryByRole('button', { name: /New Schedule/i })).not.toBeInTheDocument()
  })

  it('hides Edit, Run now, and Delete per-row buttons', () => {
    renderPage()
    expect(screen.queryByTitle('Edit')).not.toBeInTheDocument()
    expect(screen.queryByTitle('Run now')).not.toBeInTheDocument()
    expect(screen.queryByTitle('Delete')).not.toBeInTheDocument()
  })

  it('still renders task rows', () => {
    renderPage()
    expect(screen.getByText('Daily Build')).toBeInTheDocument()
  })

  it('still shows View runs button per row', () => {
    renderPage()
    expect(screen.getByTitle('View runs')).toBeInTheDocument()
  })
})

describe('SchedulesPage - Triggers column', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseIsAdmin.mockReturnValue(true)
  })

  it('renders "Triggers" column header', () => {
    mockUseScheduledTasks.mockReturnValue({ data: [makeTask()], isLoading: false, error: null })
    renderPage()
    expect(screen.getByText('Triggers')).toBeInTheDocument()
  })

  it('shows "X wf" badge when task has workflows', () => {
    mockUseScheduledTasks.mockReturnValue({
      data: [makeTask({ workflows: ['feature', 'bugfix'], workflow_chain_ids: [] })],
      isLoading: false,
      error: null,
    })
    renderPage()
    expect(screen.getByText('2 wf')).toBeInTheDocument()
  })

  it('shows "X ch" badge when task has workflow_chain_ids', () => {
    mockUseScheduledTasks.mockReturnValue({
      data: [makeTask({ workflows: [], workflow_chain_ids: ['chain-1', 'chain-2', 'chain-3'] })],
      isLoading: false,
      error: null,
    })
    renderPage()
    expect(screen.getByText('3 ch')).toBeInTheDocument()
  })

  it('shows both wf and ch badges when task has both', () => {
    mockUseScheduledTasks.mockReturnValue({
      data: [makeTask({ workflows: ['feature'], workflow_chain_ids: ['chain-1'] })],
      isLoading: false,
      error: null,
    })
    renderPage()
    expect(screen.getByText('1 wf')).toBeInTheDocument()
    expect(screen.getByText('1 ch')).toBeInTheDocument()
  })

  it('shows "—" when task has neither workflows nor chains', () => {
    mockUseScheduledTasks.mockReturnValue({
      data: [makeTask({ workflows: [], workflow_chain_ids: [] })],
      isLoading: false,
      error: null,
    })
    renderPage()
    // Triggers column and Last Run column both show "—"
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(1)
    // Neither badge should appear
    expect(screen.queryByText(/\d+ wf/)).not.toBeInTheDocument()
    expect(screen.queryByText(/\d+ ch/)).not.toBeInTheDocument()
  })
})
