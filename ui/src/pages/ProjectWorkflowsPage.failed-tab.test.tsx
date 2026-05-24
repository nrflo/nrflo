import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
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

vi.mock('./WorkflowTabContent', () => ({
  WorkflowTabContent: ({ selectedWorkflow }: any) => (
    <div data-testid="workflow-tab-content" data-selected={selectedWorkflow ?? ''} />
  ),
}))

// IDs exactly 8 chars so shortId === full ID (e.g. '#failins1')
const makeFailed = (id: string, initializedAt: string | undefined = '2026-01-01T00:00:00Z'): WorkflowState => ({
  workflow: 'feature',
  instance_id: id,
  version: 4,
  scope_type: 'project',
  current_phase: 'impl',
  status: 'failed',
  initialized_at: initializedAt,
  phases: { impl: { status: 'error' } },
  phase_order: ['impl'],
  active_agents: {},
  agent_history: [],
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

describe('ProjectWorkflowsPage — Failed tab sorting', () => {
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

  it('renders failed instances sorted by initialized_at descending, no-date last', async () => {
    const user = userEvent.setup()

    // Supply instances out of chronological order to verify sorting
    const newest = makeFailed('newinst1', '2026-03-01T00:00:00Z')
    const middle = makeFailed('midinst1', '2026-02-01T00:00:00Z')
    const oldest = makeFailed('oldinst1', '2026-01-01T00:00:00Z')
    const noDate = makeFailed('nodatei1', undefined)

    useProjectWorkflow.mockReturnValue({
      data: {
        project_id: 'test-project', has_workflow: true, state: newest,
        workflows: ['feature'],
        // Deliberately unsorted: oldest → middle → newest → noDate
        all_workflows: {
          oldinst1: oldest,
          midinst1: middle,
          newinst1: newest,
          nodatei1: noDate,
        },
      },
      isLoading: false,
    })

    renderPage()
    await user.click(screen.getByRole('button', { name: /Failed/ }))

    const rows = await screen.findAllByTestId('instance-row')
    expect(rows).toHaveLength(4)

    // Newest first, then middle, then oldest, no-initialized_at last
    expect(rows[0]).toHaveTextContent('#newinst1')
    expect(rows[1]).toHaveTextContent('#midinst1')
    expect(rows[2]).toHaveTextContent('#oldinst1')
    expect(rows[3]).toHaveTextContent('#nodatei1')
  })

  it('shows failed instance rows alongside WorkflowTabContent', async () => {
    const user = userEvent.setup()

    const inst1 = makeFailed('failins1', '2026-01-02T00:00:00Z')
    const inst2 = makeFailed('failins2', '2026-01-01T00:00:00Z')

    useProjectWorkflow.mockReturnValue({
      data: {
        project_id: 'test-project', has_workflow: true, state: inst1,
        workflows: ['feature'],
        all_workflows: { failins1: inst1, failins2: inst2 },
      },
      isLoading: false,
    })

    renderPage()
    await user.click(screen.getByRole('button', { name: /Failed/ }))

    const rows = await screen.findAllByTestId('instance-row')
    expect(rows).toHaveLength(2)
    expect(screen.getByTestId('workflow-tab-content')).toBeInTheDocument()
    expect(screen.getByText('#failins1')).toBeInTheDocument()
    expect(screen.getByText('#failins2')).toBeInTheDocument()
  })
})
