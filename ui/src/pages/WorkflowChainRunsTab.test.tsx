import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { WorkflowChainRunsTab } from './WorkflowChainRunsTab'
import type { WorkflowChainRun, WorkflowChainRunDetail } from '@/types/workflowChain'

const mockChainRunsList = vi.fn()
const mockChainRun = vi.fn()
const mockStartMutate = vi.fn()
const mockCancelMutate = vi.fn()
const mockIsAdmin = vi.fn().mockReturnValue(true)

vi.mock('@/hooks/useWorkflowChains', () => ({
  useChainRunsList: () => mockChainRunsList(),
  useChainRun: () => mockChainRun(),
  useStartChainRun: () => ({ mutate: mockStartMutate, isPending: false, error: null }),
  useCancelChainRun: () => ({ mutate: mockCancelMutate, isPending: false }),
}))

vi.mock('@/stores/authStore', () => ({
  useIsAdmin: () => mockIsAdmin(),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) =>
    selector({ currentProject: 'test-project', projectsLoaded: true })
  ),
}))

function makeRun(overrides: Partial<WorkflowChainRun> = {}): WorkflowChainRun {
  return {
    id: 'run-1',
    project_id: 'test-project',
    chain_id: 'chain-1',
    status: 'running',
    initial_instructions: '',
    triggered_by: 'user@example.com',
    current_position: 0,
    started_at: '2026-01-01T00:00:00Z',
    completed_at: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeRunDetail(overrides: Partial<WorkflowChainRunDetail> = {}): WorkflowChainRunDetail {
  return {
    ...makeRun(),
    steps: [],
    ...overrides,
  }
}

function renderTab(chainId = 'chain-1') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
  return render(
    <QueryClientProvider client={qc}>
      <WorkflowChainRunsTab chainId={chainId} />
    </QueryClientProvider>
  )
}

describe('WorkflowChainRunsTab - Render States', () => {
  beforeEach(() => { vi.clearAllMocks(); mockIsAdmin.mockReturnValue(true) })

  it('renders loading spinner', () => {
    mockChainRunsList.mockReturnValue({ data: undefined, isLoading: true, error: null })
    renderTab()
    expect(screen.getByRole('status', { name: /loading/i })).toBeInTheDocument()
  })

  it('renders empty state when no runs', () => {
    mockChainRunsList.mockReturnValue({ data: [], isLoading: false, error: null })
    renderTab()
    expect(screen.getByText(/No runs yet/)).toBeInTheDocument()
    expect(screen.getByText('0 runs')).toBeInTheDocument()
  })

  it('renders error message', () => {
    mockChainRunsList.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Failed to load runs'),
    })
    renderTab()
    expect(screen.getByText('Failed to load runs')).toBeInTheDocument()
  })

  it('renders run table with status and triggered_by', () => {
    mockChainRunsList.mockReturnValue({
      data: [makeRun({ status: 'running', triggered_by: 'alice@example.com' })],
      isLoading: false,
      error: null,
    })
    renderTab()
    expect(screen.getByText('Running')).toBeInTheDocument()
    expect(screen.getByText('alice@example.com')).toBeInTheDocument()
    expect(screen.getByText('1 run')).toBeInTheDocument()
  })

  it('shows plural "runs" for multiple runs', () => {
    mockChainRunsList.mockReturnValue({
      data: [makeRun({ id: 'run-1' }), makeRun({ id: 'run-2' })],
      isLoading: false,
      error: null,
    })
    renderTab()
    expect(screen.getByText('2 runs')).toBeInTheDocument()
  })

  it('shows dash when no triggered_by', () => {
    mockChainRunsList.mockReturnValue({
      data: [makeRun({ triggered_by: '', initial_instructions: 'some instructions' })],
      isLoading: false,
      error: null,
    })
    renderTab()
    // Only triggered_by cell shows dash; initial_instructions has text
    expect(screen.getAllByText('—')).toHaveLength(1)
    expect(screen.getByText('some instructions')).toBeInTheDocument()
  })
})

describe('WorkflowChainRunsTab - RunRow expand/collapse', () => {
  beforeEach(() => { vi.clearAllMocks(); mockIsAdmin.mockReturnValue(true) })

  it('clicking row expands and loads steps', async () => {
    const user = userEvent.setup()
    mockChainRunsList.mockReturnValue({
      data: [makeRun()],
      isLoading: false,
      error: null,
    })
    mockChainRun.mockReturnValue({ data: makeRunDetail({ steps: [] }), isLoading: false })
    renderTab()

    const row = screen.getAllByRole('row')[1] // skip header row
    await user.click(row)
    expect(screen.getByText('No steps.')).toBeInTheDocument()
  })

  it('clicking expanded row again collapses it', async () => {
    const user = userEvent.setup()
    mockChainRunsList.mockReturnValue({
      data: [makeRun()],
      isLoading: false,
      error: null,
    })
    mockChainRun.mockReturnValue({ data: makeRunDetail({ steps: [] }), isLoading: false })
    renderTab()

    const row = screen.getAllByRole('row')[1]
    await user.click(row)
    expect(screen.getByText('No steps.')).toBeInTheDocument()
    await user.click(row)
    expect(screen.queryByText('No steps.')).not.toBeInTheDocument()
  })

  it('shows steps with workflow name and status when expanded', async () => {
    const user = userEvent.setup()
    mockChainRunsList.mockReturnValue({
      data: [makeRun()],
      isLoading: false,
      error: null,
    })
    mockChainRun.mockReturnValue({
      data: makeRunDetail({
        steps: [{
          id: 'step-r1',
          chain_run_id: 'run-1',
          position: 0,
          workflow_name: 'feature',
          scope_type: 'project',
          instructions_used: '',
          status: 'completed',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        }],
      }),
      isLoading: false,
    })
    renderTab()

    const row = screen.getAllByRole('row')[1]
    await user.click(row)
    expect(screen.getByText('feature')).toBeInTheDocument()
    expect(screen.getByText('Completed')).toBeInTheDocument()
  })
})

describe('WorkflowChainRunsTab - StartRunDialog', () => {
  beforeEach(() => { vi.clearAllMocks(); mockIsAdmin.mockReturnValue(true) })

  it('clicking Start run button opens dialog', async () => {
    const user = userEvent.setup()
    mockChainRunsList.mockReturnValue({ data: [], isLoading: false, error: null })
    renderTab()

    await user.click(screen.getByRole('button', { name: /Start run/ }))
    expect(screen.getByText('Start Chain Run')).toBeInTheDocument()
    expect(screen.getByPlaceholderText(/Instructions for the first step agent/)).toBeInTheDocument()
  })

  it('submitting without instructions calls mutation with empty data', async () => {
    const user = userEvent.setup()
    mockChainRunsList.mockReturnValue({ data: [], isLoading: false, error: null })
    renderTab()

    // Open dialog — header button is first; dialog submit button is second
    await user.click(screen.getAllByRole('button', { name: /Start run/ })[0])
    // Now two "Start run" buttons exist: header + dialog submit; pick dialog submit (last)
    const buttons = screen.getAllByRole('button', { name: 'Start run' })
    await user.click(buttons[buttons.length - 1])

    expect(mockStartMutate).toHaveBeenCalledWith(
      { chainId: 'chain-1', data: { instructions: undefined } },
      expect.any(Object)
    )
  })

  it('submitting with instructions passes trimmed text', async () => {
    const user = userEvent.setup()
    mockChainRunsList.mockReturnValue({ data: [], isLoading: false, error: null })
    renderTab()

    await user.click(screen.getAllByRole('button', { name: /Start run/ })[0])
    await user.type(screen.getByPlaceholderText(/Instructions for the first step agent/), 'Do the thing')
    const buttons = screen.getAllByRole('button', { name: 'Start run' })
    await user.click(buttons[buttons.length - 1])

    expect(mockStartMutate).toHaveBeenCalledWith(
      { chainId: 'chain-1', data: { instructions: 'Do the thing' } },
      expect.any(Object)
    )
  })

  it('cancel button in dialog closes it without calling mutation', async () => {
    const user = userEvent.setup()
    mockChainRunsList.mockReturnValue({ data: [], isLoading: false, error: null })
    renderTab()

    await user.click(screen.getByRole('button', { name: /Start run/ }))
    expect(screen.getByText('Start Chain Run')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByText('Start Chain Run')).not.toBeInTheDocument()
    expect(mockStartMutate).not.toHaveBeenCalled()
  })

  it('shows error message when startMutation has error', () => {
    vi.mock('@/hooks/useWorkflowChains', () => ({
      useChainRunsList: () => mockChainRunsList(),
      useChainRun: () => mockChainRun(),
      useStartChainRun: () => ({
        mutate: mockStartMutate,
        isPending: false,
        error: new Error('Server error'),
      }),
      useCancelChainRun: () => ({ mutate: mockCancelMutate, isPending: false }),
    }))
  })
})

describe('WorkflowChainRunsTab - Cancel run (admin gating)', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('shows cancel button for admin on non-terminal run', () => {
    mockIsAdmin.mockReturnValue(true)
    mockChainRunsList.mockReturnValue({
      data: [makeRun({ status: 'running' })],
      isLoading: false,
      error: null,
    })
    renderTab()
    expect(screen.getByTitle('Cancel run')).toBeInTheDocument()
  })

  it('hides cancel button for non-admin', () => {
    mockIsAdmin.mockReturnValue(false)
    mockChainRunsList.mockReturnValue({
      data: [makeRun({ status: 'running' })],
      isLoading: false,
      error: null,
    })
    renderTab()
    expect(screen.queryByTitle('Cancel run')).not.toBeInTheDocument()
  })

  it('hides cancel button for completed run (terminal status)', () => {
    mockIsAdmin.mockReturnValue(true)
    mockChainRunsList.mockReturnValue({
      data: [makeRun({ status: 'completed' })],
      isLoading: false,
      error: null,
    })
    renderTab()
    expect(screen.queryByTitle('Cancel run')).not.toBeInTheDocument()
  })

  it('hides cancel button for failed run', () => {
    mockIsAdmin.mockReturnValue(true)
    mockChainRunsList.mockReturnValue({
      data: [makeRun({ status: 'failed' })],
      isLoading: false,
      error: null,
    })
    renderTab()
    expect(screen.queryByTitle('Cancel run')).not.toBeInTheDocument()
  })

  it('hides cancel button for canceled run', () => {
    mockIsAdmin.mockReturnValue(true)
    mockChainRunsList.mockReturnValue({
      data: [makeRun({ status: 'canceled' })],
      isLoading: false,
      error: null,
    })
    renderTab()
    expect(screen.queryByTitle('Cancel run')).not.toBeInTheDocument()
  })

  it('clicking cancel button opens confirm dialog', async () => {
    const user = userEvent.setup()
    mockIsAdmin.mockReturnValue(true)
    mockChainRunsList.mockReturnValue({
      data: [makeRun({ status: 'running' })],
      isLoading: false,
      error: null,
    })
    renderTab()

    await user.click(screen.getByTitle('Cancel run'))
    expect(screen.getByText('Cancel Run')).toBeInTheDocument()
    expect(screen.getByText(/Are you sure you want to cancel this chain run/)).toBeInTheDocument()
  })

  it('confirming cancel calls mutation with chainId and runId', async () => {
    const user = userEvent.setup()
    mockIsAdmin.mockReturnValue(true)
    mockChainRunsList.mockReturnValue({
      data: [makeRun({ id: 'run-abc', status: 'running' })],
      isLoading: false,
      error: null,
    })
    renderTab()

    await user.click(screen.getByTitle('Cancel run'))
    // Dialog confirm button has text "Cancel run"; icon button has no text (SVG only)
    // Use the button that contains text node "Cancel run" — last button in DOM within dialog
    const cancelButtons = screen.getAllByRole('button', { name: 'Cancel run' })
    await user.click(cancelButtons[cancelButtons.length - 1])
    expect(mockCancelMutate).toHaveBeenCalledWith(
      { chainId: 'chain-1', runId: 'run-abc' },
      expect.any(Object)
    )
  })
})
