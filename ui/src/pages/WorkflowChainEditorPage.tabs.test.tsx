import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { WorkflowChainEditorPage } from './WorkflowChainEditorPage'
import type { WorkflowChainWithSteps, WorkflowChainRun } from '@/types/workflowChain'

// Mocks for editor hooks
const mockUseWorkflowChain = vi.fn()
const mockUseIsAdmin = vi.fn().mockReturnValue(true)

// Mocks for runs tab hooks
const mockChainRunsList = vi.fn()

vi.mock('@/hooks/useWorkflowChains', () => ({
  useWorkflowChain: (id: string) => mockUseWorkflowChain(id),
  useUpdateWorkflowChain: () => ({ mutate: vi.fn(), isPending: false }),
  useAppendStep: () => ({ mutate: vi.fn(), isPending: false }),
  useUpdateStep: () => ({ mutate: vi.fn(), isPending: false }),
  useDeleteStep: () => ({ mutate: vi.fn(), isPending: false }),
  useReorderSteps: () => ({ mutate: vi.fn(), isPending: false }),
  useChainRunsList: () => mockChainRunsList(),
  useChainRun: () => ({ data: undefined, isLoading: false }),
  useStartChainRun: () => ({ mutate: vi.fn(), isPending: false, error: null }),
  useCancelChainRun: () => ({ mutate: vi.fn(), isPending: false }),
}))

vi.mock('@/stores/authStore', () => ({
  useIsAdmin: () => mockUseIsAdmin(),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) =>
    selector({ currentProject: 'test-project', projectsLoaded: true })
  ),
}))

vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn().mockResolvedValue({ feature: {}, bugfix: {} }),
}))

function makeChain(overrides: Partial<WorkflowChainWithSteps> = {}): WorkflowChainWithSteps {
  return {
    id: 'chain-1',
    project_id: 'test-project',
    name: 'Test Chain',
    description: '',
    steps: [],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeRun(overrides: Partial<WorkflowChainRun> = {}): WorkflowChainRun {
  return {
    id: 'run-1',
    project_id: 'test-project',
    chain_id: 'chain-1',
    status: 'completed',
    initial_instructions: '',
    triggered_by: 'admin',
    current_position: 0,
    started_at: '2026-01-01T00:00:00Z',
    completed_at: '2026-01-01T00:01:00Z',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:01:00Z',
    ...overrides,
  }
}

function renderEditor(id = 'chain-1') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[`/workflow-chains/${id}`]}>
        <Routes>
          <Route path="/workflow-chains/:id" element={<WorkflowChainEditorPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('WorkflowChainEditorPage - Tab bar', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseIsAdmin.mockReturnValue(true)
    mockUseWorkflowChain.mockReturnValue({ data: makeChain(), isLoading: false })
    mockChainRunsList.mockReturnValue({ data: [], isLoading: false, error: null })
  })

  it('Definition tab is active by default showing chain details form', () => {
    renderEditor()
    // "Chain Details" section only renders in definition tab
    expect(screen.getByText('Chain Details')).toBeInTheDocument()
    // Runs tab content is not visible
    expect(screen.queryByText(/No runs yet/)).not.toBeInTheDocument()
  })

  it('clicking Runs tab shows runs content', async () => {
    const user = userEvent.setup()
    renderEditor()

    // Tab buttons render lowercase text; capitalize is CSS-only
    await user.click(screen.getByRole('button', { name: 'runs' }))
    expect(screen.getByText(/No runs yet/)).toBeInTheDocument()
  })

  it('clicking back to Definition tab hides runs content', async () => {
    const user = userEvent.setup()
    renderEditor()

    await user.click(screen.getByRole('button', { name: 'runs' }))
    expect(screen.getByText(/No runs yet/)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'definition' }))
    expect(screen.queryByText(/No runs yet/)).not.toBeInTheDocument()
    expect(screen.getByText('Chain Details')).toBeInTheDocument()
  })

  it('Runs tab shows run count from list', async () => {
    const user = userEvent.setup()
    mockChainRunsList.mockReturnValue({
      data: [makeRun(), makeRun({ id: 'run-2' })],
      isLoading: false,
      error: null,
    })
    renderEditor()

    await user.click(screen.getByRole('button', { name: 'runs' }))
    expect(screen.getByText('2 runs')).toBeInTheDocument()
  })
})
