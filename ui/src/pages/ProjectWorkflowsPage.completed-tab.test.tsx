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
  WorkflowTabContent: () => <div data-testid="workflow-tab-content" />,
}))

vi.mock('@/components/workflow/CompletedAgentsTable', () => ({
  CompletedAgentsTable: (props: any) => (
    <div data-testid="completed-agents-table">
      <div data-testid="completed-agents-count">{props.agentHistory?.length ?? 0}</div>
      <div data-testid="completed-sessions-count">{props.sessions?.length ?? 0}</div>
    </div>
  ),
}))

vi.mock('@/components/workflow/AgentLogPanel', () => ({
  AgentLogPanel: () => <div data-testid="agent-log-panel" />,
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

  it('clicking a different row in WorkflowInstanceTable shows that instances agents', async () => {
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

    // Auto-selects first instance (1 agent)
    await waitFor(() => {
      expect(screen.getByTestId('completed-agents-count').textContent).toBe('1')
    })

    // Click the second row via its short ID text
    await user.click(screen.getByText('#compins2'))

    // Now shows second instance's 3 agents
    await waitFor(() => {
      expect(screen.getByTestId('completed-agents-count').textContent).toBe('3')
    })
  })

  it('sessions are filtered to the selected completed instance', async () => {
    const user = userEvent.setup()

    const inst1 = makeCompleted('compins1', 'feature', 1)
    const inst2 = makeCompleted('compins2', 'bugfix', 1)

    useProjectWorkflow.mockReturnValue({
      data: {
        project_id: 'test-project', has_workflow: true, state: inst1,
        workflows: ['feature', 'bugfix'],
        all_workflows: { compins1: inst1, compins2: inst2 },
      },
      isLoading: false,
    })

    useProjectAgentSessions.mockReturnValue({
      data: {
        project_id: 'test-project',
        sessions: [
          {
            id: 'compins1-s0', project_id: 'test-project', ticket_id: '',
            workflow_instance_id: 'compins1', phase: 'verification', workflow: 'feature',
            agent_type: 'setup-analyzer', model_id: 'claude-sonnet-4-5', status: 'completed',
            result: 'pass', message_count: 5, restart_count: 0,
            created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T01:00:00Z',
            started_at: '2026-01-01T00:00:00Z', ended_at: '2026-01-01T01:00:00Z',
          },
          {
            id: 'compins2-s0', project_id: 'test-project', ticket_id: '',
            workflow_instance_id: 'compins2', phase: 'verification', workflow: 'bugfix',
            agent_type: 'setup-analyzer', model_id: 'claude-sonnet-4-5', status: 'completed',
            result: 'pass', message_count: 3, restart_count: 0,
            created_at: '2026-01-01T02:00:00Z', updated_at: '2026-01-01T03:00:00Z',
            started_at: '2026-01-01T02:00:00Z', ended_at: '2026-01-01T03:00:00Z',
          },
        ],
      },
      isLoading: false,
    })

    renderPage()
    await user.click(screen.getByRole('button', { name: /Completed/ }))

    // First instance selected: 1 session (compins1-s0)
    await waitFor(() => {
      expect(screen.getByTestId('completed-sessions-count').textContent).toBe('1')
    })

    // Switch to second instance
    await user.click(screen.getByText('#compins2'))

    // Second instance session shown
    await waitFor(() => {
      expect(screen.getByTestId('completed-sessions-count').textContent).toBe('1')
    })
  })

  it('shows No completed workflows message when completed tab is empty', async () => {
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

    expect(screen.getByText('No completed workflows')).toBeInTheDocument()
    expect(screen.queryByTestId('completed-agents-table')).not.toBeInTheDocument()
  })
})
