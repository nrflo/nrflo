import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { WorkflowChainEditorPage } from './WorkflowChainEditorPage'
import { StepRow } from './WorkflowChainStepRow'
import type { WorkflowChainWithSteps, WorkflowChainStep } from '@/types/workflowChain'
import type { StepEdit } from './WorkflowChainStepRow'

const mockUseWorkflowChain = vi.fn()
const mockUpdateChainMutate = vi.fn()
const mockAppendMutate = vi.fn()
const mockUpdateStepMutate = vi.fn()
const mockDeleteStepMutate = vi.fn()
const mockReorderMutate = vi.fn()
const mockUseIsAdmin = vi.fn().mockReturnValue(true)

vi.mock('@/hooks/useWorkflowChains', () => ({
  useWorkflowChain: (id: string) => mockUseWorkflowChain(id),
  useUpdateWorkflowChain: () => ({ mutate: mockUpdateChainMutate, isPending: false }),
  useAppendStep: () => ({ mutate: mockAppendMutate, isPending: false }),
  useUpdateStep: () => ({ mutate: mockUpdateStepMutate, isPending: false }),
  useDeleteStep: () => ({ mutate: mockDeleteStepMutate, isPending: false }),
  useReorderSteps: () => ({ mutate: mockReorderMutate, isPending: false }),
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

function makeStep(overrides: Partial<WorkflowChainStep> = {}): WorkflowChainStep {
  return {
    id: 'step-1',
    project_id: 'test-project',
    chain_id: 'chain-1',
    position: 0,
    workflow_name: 'feature',
    scope_type: 'project',
    base_instructions: '',
    require_ticket_handoff: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeChainWithSteps(
  steps: WorkflowChainStep[] = [],
  overrides: Partial<WorkflowChainWithSteps> = {}
): WorkflowChainWithSteps {
  return {
    id: 'chain-1',
    project_id: 'test-project',
    name: 'My Test Chain',
    description: 'A description',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    steps,
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

describe('WorkflowChainEditorPage - Render', () => {
  beforeEach(() => { vi.clearAllMocks(); mockUseIsAdmin.mockReturnValue(true) })

  it('renders loading state', () => {
    mockUseWorkflowChain.mockReturnValue({ data: undefined, isLoading: true })
    renderEditor()
    expect(screen.getByRole('status', { name: /loading/i })).toBeInTheDocument()
  })

  it('renders chain name in heading', () => {
    mockUseWorkflowChain.mockReturnValue({
      data: makeChainWithSteps([]),
      isLoading: false,
    })
    renderEditor()
    expect(screen.getByRole('heading', { name: 'My Test Chain' })).toBeInTheDocument()
  })

  it('renders step rows with step labels', () => {
    const steps = [
      makeStep({ id: 'step-1', position: 0 }),
      makeStep({ id: 'step-2', position: 1, scope_type: 'ticket' }),
    ]
    mockUseWorkflowChain.mockReturnValue({ data: makeChainWithSteps(steps), isLoading: false })
    renderEditor()
    expect(screen.getByText('Step 0')).toBeInTheDocument()
    expect(screen.getByText('Step 1')).toBeInTheDocument()
  })

  it('shows empty state when chain has no steps', () => {
    mockUseWorkflowChain.mockReturnValue({ data: makeChainWithSteps([]), isLoading: false })
    renderEditor()
    expect(screen.getByText(/No steps yet/)).toBeInTheDocument()
  })
})

describe('WorkflowChainEditorPage - Step Reordering', () => {
  beforeEach(() => { vi.clearAllMocks(); mockUseIsAdmin.mockReturnValue(true) })

  it('clicking Move down on step 0 calls reorderSteps with swapped ids', async () => {
    const user = userEvent.setup()
    const steps = [
      makeStep({ id: 'step-1', position: 0 }),
      makeStep({ id: 'step-2', position: 1, scope_type: 'ticket' }),
    ]
    mockUseWorkflowChain.mockReturnValue({ data: makeChainWithSteps(steps), isLoading: false })
    renderEditor()

    const downButtons = screen.getAllByTitle('Move down')
    await user.click(downButtons[0])

    expect(mockReorderMutate).toHaveBeenCalledWith(
      { chainId: 'chain-1', data: { ordered_step_ids: ['step-2', 'step-1'] } },
      expect.any(Object)
    )
  })
})

describe('WorkflowChainEditorPage - Add / Delete Steps', () => {
  beforeEach(() => { vi.clearAllMocks(); mockUseIsAdmin.mockReturnValue(true) })

  it('clicking Add step calls appendStep mutation with project scope for first step', async () => {
    const user = userEvent.setup()
    mockUseWorkflowChain.mockReturnValue({ data: makeChainWithSteps([]), isLoading: false })
    renderEditor()

    await user.click(screen.getByRole('button', { name: /Add step/ }))
    expect(mockAppendMutate).toHaveBeenCalledWith(
      {
        chainId: 'chain-1',
        data: expect.objectContaining({
          scope_type: 'project',
          base_instructions: '',
          require_ticket_handoff: false,
        }),
      },
      expect.any(Object)
    )
  })

  it('clicking Delete step button opens confirm dialog', async () => {
    const user = userEvent.setup()
    const steps = [makeStep({ id: 'step-1' })]
    mockUseWorkflowChain.mockReturnValue({ data: makeChainWithSteps(steps), isLoading: false })
    renderEditor()

    await user.click(screen.getByTitle('Delete step'))
    expect(screen.getByText('Delete Step')).toBeInTheDocument()
    expect(screen.getByText(/Are you sure you want to delete this step/)).toBeInTheDocument()
  })
})

// StepRow unit tests (stateless component, no providers needed)
describe('StepRow - Validation', () => {
  function makeEdit(overrides: Partial<StepEdit> = {}): StepEdit {
    return {
      workflow_name: 'feature',
      scope_type: 'project',
      base_instructions: '',
      require_ticket_handoff: false,
      ...overrides,
    }
  }

  const baseStepProps = {
    step: makeStep(),
    total: 1,
    workflowOptions: [{ value: 'feature', label: 'feature' }],
    isAdmin: true,
    isPendingReorder: false,
    onEdit: vi.fn(),
    onSave: vi.fn(),
    onMoveUp: vi.fn(),
    onMoveDown: vi.fn(),
    onDelete: vi.fn(),
    isSavingStep: false,
  }

  it('step 0 with ticket scope shows error and disables Save step button', () => {
    render(
      <StepRow
        {...baseStepProps}
        step={makeStep({ id: 'step-1' })}
        edit={makeEdit({ scope_type: 'ticket' })}
        index={0}
      />
    )
    expect(screen.getByText('Step 0 must be project-scoped')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Save step/ })).toBeDisabled()
  })

  it('require_ticket_handoff toggle is disabled when scope is project', () => {
    render(
      <StepRow
        {...baseStepProps}
        step={makeStep({ id: 'step-1' })}
        edit={makeEdit({ scope_type: 'project', require_ticket_handoff: false })}
        index={1}
      />
    )
    expect(screen.getByRole('switch')).toBeDisabled()
  })

  it('ticket handoff with project scope shows error and disables Save', () => {
    render(
      <StepRow
        {...baseStepProps}
        step={makeStep({ id: 'step-1' })}
        edit={makeEdit({ scope_type: 'project', require_ticket_handoff: true })}
        index={1}
      />
    )
    expect(screen.getByText('Ticket handoff requires ticket scope')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Save step/ })).toBeDisabled()
  })

  it('valid step (scope=project, no handoff) enables Save button', () => {
    render(
      <StepRow
        {...baseStepProps}
        step={makeStep({ id: 'step-1' })}
        edit={makeEdit({ scope_type: 'project', require_ticket_handoff: false })}
        index={0}
      />
    )
    expect(screen.queryByText('Step 0 must be project-scoped')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Save step/ })).not.toBeDisabled()
  })
})
