import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { ScheduleRunsDialog } from './ScheduleRunsDialog'
import type { ScheduledTask, ScheduleRun } from '@/types/schedules'

const mockUseScheduleRuns = vi.fn()

vi.mock('@/hooks/useScheduledTasks', () => ({
  useScheduleRuns: (taskId: string, page: number) => mockUseScheduleRuns(taskId, page),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) =>
    selector({ currentProject: 'test-project', projectsLoaded: true })
  ),
}))

const defaultTask: ScheduledTask = {
  id: 'task-1',
  project_id: 'test-project',
  name: 'Daily Build',
  description: '',
  cron_expression: '0 9 * * 1-5',
  workflows: ['feature'],
  enabled: true,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

function makeRun(overrides: Partial<ScheduleRun> = {}): ScheduleRun {
  return {
    id: 'run-1',
    scheduled_task_id: 'task-1',
    project_id: 'test-project',
    triggered_at: '2026-01-01T09:00:00Z',
    status: 'triggered',
    workflows: [{ workflow: 'feature', instance_id: 'inst-1' }],
    ...overrides,
  }
}

function renderDialog(task: ScheduledTask = defaultTask, onClose = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <ScheduleRunsDialog open={true} onClose={onClose} task={task} />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('ScheduleRunsDialog', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders loading spinner while fetching', () => {
    mockUseScheduleRuns.mockReturnValue({ data: undefined, isLoading: true })
    renderDialog()
    expect(screen.getByRole('status', { name: /loading/i })).toBeInTheDocument()
  })

  it('renders empty state when no runs', () => {
    mockUseScheduleRuns.mockReturnValue({ data: [], isLoading: false })
    renderDialog()
    expect(screen.getByText('No runs yet')).toBeInTheDocument()
  })

  it('renders run row with status and workflow name', () => {
    mockUseScheduleRuns.mockReturnValue({
      data: [makeRun({ status: 'failed' })],
      isLoading: false,
    })
    renderDialog()
    expect(screen.getByText('Failed')).toBeInTheDocument()
    expect(screen.getByText('feature')).toBeInTheDocument()
  })

  it('shows task name and Run History heading in dialog header', () => {
    mockUseScheduleRuns.mockReturnValue({ data: [], isLoading: false })
    renderDialog({ ...defaultTask, name: 'My Scheduled Task' })
    expect(screen.getByText('Run History')).toBeInTheDocument()
    expect(screen.getByText('My Scheduled Task')).toBeInTheDocument()
  })

  it('shows workflow instance link when instance_id is present', () => {
    mockUseScheduleRuns.mockReturnValue({
      data: [makeRun({ workflows: [{ workflow: 'feature', instance_id: 'inst-abc' }] })],
      isLoading: false,
    })
    renderDialog()
    const link = screen.getByRole('link', { name: /view/i })
    expect(link).toHaveAttribute('href', '/project-workflows?instance_id=inst-abc')
  })

  it('clicking next button increments page and calls hook with new page', async () => {
    const user = userEvent.setup()
    // 20 items triggers "has more" condition
    mockUseScheduleRuns.mockReturnValue({
      data: Array.from({ length: 20 }, (_, i) =>
        makeRun({ id: `run-${i}`, workflows: [] })
      ),
      isLoading: false,
    })
    renderDialog()

    expect(mockUseScheduleRuns).toHaveBeenCalledWith('task-1', 0)

    // buttons: [0]=X close, [1]=prev (disabled), [2]=next (enabled)
    const buttons = screen.getAllByRole('button')
    const prevBtn = buttons[buttons.length - 2]
    const nextBtn = buttons[buttons.length - 1]
    expect(prevBtn).toBeDisabled()
    expect(nextBtn).not.toBeDisabled()

    await user.click(nextBtn)
    expect(mockUseScheduleRuns).toHaveBeenCalledWith('task-1', 1)
  })

  it('clicking prev button on page 1 decrements page', async () => {
    const user = userEvent.setup()
    mockUseScheduleRuns.mockReturnValue({
      data: Array.from({ length: 20 }, (_, i) =>
        makeRun({ id: `run-${i}`, workflows: [] })
      ),
      isLoading: false,
    })
    renderDialog()

    const buttons = screen.getAllByRole('button')
    const nextBtn = buttons[buttons.length - 1]

    // Go to page 1
    await user.click(nextBtn)
    expect(mockUseScheduleRuns).toHaveBeenCalledWith('task-1', 1)

    // Now prev should be enabled; click it
    const updatedButtons = screen.getAllByRole('button')
    const prevBtn = updatedButtons[updatedButtons.length - 2]
    expect(prevBtn).not.toBeDisabled()
    await user.click(prevBtn)
    expect(mockUseScheduleRuns).toHaveBeenCalledWith('task-1', 0)
  })

  it('shows run-level error when present', () => {
    mockUseScheduleRuns.mockReturnValue({
      data: [makeRun({ workflows: [], error: 'Something went wrong' })],
      isLoading: false,
    })
    renderDialog()
    expect(screen.getByText('Something went wrong')).toBeInTheDocument()
  })
})
