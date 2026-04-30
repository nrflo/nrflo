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

vi.mock('@/hooks/useScheduledTasks', () => ({
  useScheduledTasks: () => mockUseScheduledTasks(),
  useDeleteScheduledTask: () => ({ mutate: mockDeleteMutate, isPending: false }),
  useUpdateScheduledTask: () => ({ mutate: mockUpdateMutate, isPending: false }),
  useRunScheduleNow: () => ({ mutate: mockRunNowMutate, isPending: false }),
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
  beforeEach(() => vi.clearAllMocks())

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
  beforeEach(() => vi.clearAllMocks())

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
