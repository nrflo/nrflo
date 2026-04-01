import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ProjectWorkflowsPage } from './ProjectWorkflowsPage'
import type { WorkflowState } from '@/types/workflow'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string; projects: unknown[]; projectsLoaded: boolean }) => unknown) =>
    selector({
      currentProject: 'test-project',
      projects: [{ id: 'test-project', name: 'Test Project', root_path: '/test', default_branch: null, created_at: '', updated_at: '' }],
      projectsLoaded: true,
    }),
}))

vi.mock('@/hooks/useTickets', async () => {
  const actual = await vi.importActual<typeof import('@/hooks/useTickets')>('@/hooks/useTickets')
  return {
    ...actual,
    useProjectWorkflow: vi.fn(),
    useProjectAgentSessions: vi.fn(),
    useRunProjectWorkflow: vi.fn(),
    useStopProjectWorkflow: vi.fn(),
    useRetryFailedProjectAgent: vi.fn(),
    useDeleteProjectWorkflowInstance: vi.fn(),
  }
})

vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn().mockResolvedValue({
    feature: { description: 'Feature', scope_type: 'project', phases: [{ id: 'setup', agent: 'setup', layer: 0 }] },
  }),
}))

vi.mock('@/api/agentDefs', () => ({
  listAgentDefs: vi.fn().mockResolvedValue([
    { id: 'setup', model: 'sonnet', timeout: 300, prompt: 'test', workflow_id: 'feature', project_id: 'test-project', created_at: '', updated_at: '' },
  ]),
}))

vi.mock('./WorkflowTabContent', () => ({
  WorkflowTabContent: ({ selectedWorkflow }: any) => (
    <div data-testid="workflow-tab-content" data-selected={selectedWorkflow ?? ''} />
  ),
}))


// IDs exactly 8 chars so shortId === full ID (e.g. '#compins1')
const makeCompleted = (id: string, workflow: string, agentCount: number): WorkflowState => ({
  workflow,
  instance_id: id,
  version: 4,
  scope_type: 'project',
  current_phase: 'verification',
  status: 'completed',
  completed_at: '2026-01-01T05:00:00Z',
  total_duration_sec: 3600,
  phases: { verification: { status: 'completed', result: 'pass' } },
  phase_order: ['verification'],
  active_agents: {},
  agent_history: Array.from({ length: agentCount }, (_, i) => ({
    agent_id: `${id}-a${i}`,
    agent_type: 'setup-analyzer',
    phase: 'verification',
    session_id: `${id}-s${i}`,
    model_id: 'claude-sonnet-4-5',
    result: 'pass' as const,
    started_at: '2026-01-01T00:00:00Z',
    ended_at: '2026-01-01T01:00:00Z',
  })),
  findings: {},
})

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <ProjectWorkflowsPage />
    </QueryClientProvider>
  )
}

describe('ProjectWorkflowsPage — Completed tab instance selection', () => {
  let useProjectWorkflow: any
  let useProjectAgentSessions: any
  let useRunProjectWorkflow: any
  let useStopProjectWorkflow: any
  let useRetryFailedProjectAgent: any
  let useDeleteProjectWorkflowInstance: any

  beforeEach(async () => {
    const hooks = await import('@/hooks/useTickets')
    useProjectWorkflow = hooks.useProjectWorkflow as any
    useProjectAgentSessions = hooks.useProjectAgentSessions as any
    useRunProjectWorkflow = hooks.useRunProjectWorkflow as any
    useStopProjectWorkflow = hooks.useStopProjectWorkflow as any
    useRetryFailedProjectAgent = hooks.useRetryFailedProjectAgent as any
    useDeleteProjectWorkflowInstance = hooks.useDeleteProjectWorkflowInstance as any

    vi.clearAllMocks()

    useProjectAgentSessions.mockReturnValue({ data: { project_id: 'test-project', sessions: [] }, isLoading: false })
    useRunProjectWorkflow.mockReturnValue({ mutateAsync: vi.fn(), isPending: false, isError: false, error: null })
    useStopProjectWorkflow.mockReturnValue({ mutate: vi.fn(), isPending: false })
    useRetryFailedProjectAgent.mockReturnValue({ mutate: vi.fn(), isPending: false, variables: null })
    useDeleteProjectWorkflowInstance.mockReturnValue({ mutate: vi.fn(), isPending: false })
  })

  it('renders WorkflowTabContent when completed instances exist', async () => {
    const user = userEvent.setup()

    const inst1 = makeCompleted('compins1', 'feature', 1)
    const inst2 = makeCompleted('compins2', 'bugfix', 3)

    useProjectWorkflow.mockReturnValue({
      data: {
        project_id: 'test-project', has_workflow: true, state: inst1,
        workflows: ['feature', 'bugfix'],
        all_workflows: { compins1: inst1, compins2: inst2 },
      },
      isLoading: false,
    })

    renderPage()
    await user.click(screen.getByRole('button', { name: /Completed/ }))

    await waitFor(() => {
      expect(screen.getByTestId('workflow-tab-content')).toBeInTheDocument()
    })
  })

  it('shows WorkflowInstanceTable rows alongside WorkflowTabContent', async () => {
    const user = userEvent.setup()

    const inst1 = makeCompleted('compins1', 'feature', 1)
    const inst2 = makeCompleted('compins2', 'bugfix', 3)

    useProjectWorkflow.mockReturnValue({
      data: {
        project_id: 'test-project', has_workflow: true, state: inst1,
        workflows: ['feature', 'bugfix'],
        all_workflows: { compins1: inst1, compins2: inst2 },
      },
      isLoading: false,
    })

    renderPage()
    await user.click(screen.getByRole('button', { name: /Completed/ }))

    await waitFor(() => {
      expect(screen.getByTestId('workflow-tab-content')).toBeInTheDocument()
      expect(screen.getByText('#compins1')).toBeInTheDocument()
      expect(screen.getByText('#compins2')).toBeInTheDocument()
    })
  })

  it('clicking a WorkflowInstanceTable row updates the selected instance', async () => {
    const user = userEvent.setup()

    const inst1 = makeCompleted('compins1', 'feature', 1)
    const inst2 = makeCompleted('compins2', 'bugfix', 3)

    useProjectWorkflow.mockReturnValue({
      data: {
        project_id: 'test-project', has_workflow: true, state: inst1,
        workflows: ['feature', 'bugfix'],
        all_workflows: { compins1: inst1, compins2: inst2 },
      },
      isLoading: false,
    })

    renderPage()
    await user.click(screen.getByRole('button', { name: /Completed/ }))

    // Initially no explicit selection (page resolves to first instance internally)
    await waitFor(() => {
      expect(screen.getByTestId('workflow-tab-content')).toHaveAttribute('data-selected', '')
    })

    // Click second instance row
    await user.click(screen.getByText('#compins2'))

    // WorkflowTabContent now receives compins2 as selectedWorkflow
    await waitFor(() => {
      expect(screen.getByTestId('workflow-tab-content')).toHaveAttribute('data-selected', 'compins2')
    })
  })

  it('shows empty state when completed tab has no instances', async () => {
    const user = userEvent.setup()

    useProjectWorkflow.mockReturnValue({
      data: {
        project_id: 'test-project', has_workflow: false, state: null,
        workflows: [], all_workflows: {},
      },
      isLoading: false,
    })

    renderPage()
    await user.click(screen.getByRole('button', { name: /Completed/ }))

    expect(screen.getByTestId('workflow-tab-content')).toBeInTheDocument()
  })
})
