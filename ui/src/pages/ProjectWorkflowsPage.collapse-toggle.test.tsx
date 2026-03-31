import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ProjectWorkflowsPage } from './ProjectWorkflowsPage'
import type { ProjectWorkflowResponse, ProjectAgentSessionsResponse, WorkflowState } from '@/types/workflow'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string; projects: unknown[]; projectsLoaded: boolean }) => unknown) =>
    selector({ currentProject: 'test-project', projects: [], projectsLoaded: true }),
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
    useTakeControlProject: vi.fn(),
    useExitInteractiveProject: vi.fn(),
    useResumeSessionProject: vi.fn(),
    useDeleteProjectWorkflowInstance: vi.fn(),
  }
})

vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn().mockResolvedValue({
    feature: { description: 'Feature', scope_type: 'project', phases: [] },
  }),
}))

vi.mock('./WorkflowTabContent', () => ({
  WorkflowTabContent: () => <div data-testid="workflow-tab-content" />,
}))

// CompletedAgentsTable mock that exposes onAgentSelect via a test button
vi.mock('@/components/workflow/CompletedAgentsTable', () => ({
  CompletedAgentsTable: (props: any) => (
    <div data-testid="completed-agents-table">
      <button
        data-testid="select-agent-btn"
        onClick={() => props.onAgentSelect({ phaseName: 'investigation', historyEntry: { agent_id: 'h1', agent_type: 'setup-analyzer', phase: 'investigation', result: 'pass', duration_sec: 60 } })}
      >
        Select Agent
      </button>
    </div>
  ),
}))

vi.mock('@/components/workflow/AgentLogPanel', () => ({
  AgentLogPanel: () => <div data-testid="agent-log-panel" />,
}))

vi.mock('@/components/workflow/AgentTerminalDialog', () => ({
  AgentTerminalDialog: () => null,
}))

const completedWorkflowState: WorkflowState = {
  workflow: 'feature',
  instance_id: 'inst-completed',
  version: 4,
  scope_type: 'project',
  current_phase: 'investigation',
  status: 'completed',
  phases: { investigation: { status: 'completed', result: 'pass' } },
  phase_order: ['investigation'],
  active_agents: {},
  agent_history: [
    {
      agent_id: 'h1',
      agent_type: 'setup-analyzer',
      phase: 'investigation',
      session_id: 'sess-hist',
      result: 'pass',
      duration_sec: 60,
    },
  ],
  findings: {},
}

const workflowResponse: ProjectWorkflowResponse = {
  project_id: 'test-project',
  has_workflow: true,
  state: completedWorkflowState,
  workflows: ['feature'],
  all_workflows: { 'inst-completed': completedWorkflowState },
}

const sessionsResponse: ProjectAgentSessionsResponse = {
  project_id: 'test-project',
  sessions: [],
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <ProjectWorkflowsPage />
    </QueryClientProvider>
  )
}

describe('ProjectWorkflowsPage - completed tab collapse toggle', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    const hooks = await import('@/hooks/useTickets')
    const noopMutation = { mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false, isError: false, error: null }

    ;(hooks.useProjectWorkflow as any).mockReturnValue({ data: workflowResponse, isLoading: false })
    ;(hooks.useProjectAgentSessions as any).mockReturnValue({ data: sessionsResponse, isLoading: false })
    ;(hooks.useRunProjectWorkflow as any).mockReturnValue(noopMutation)
    ;(hooks.useStopProjectWorkflow as any).mockReturnValue(noopMutation)
    ;(hooks.useRetryFailedProjectAgent as any).mockReturnValue(noopMutation)
    ;(hooks.useTakeControlProject as any).mockReturnValue(noopMutation)
    ;(hooks.useExitInteractiveProject as any).mockReturnValue(noopMutation)
    ;(hooks.useResumeSessionProject as any).mockReturnValue(noopMutation)
    ;(hooks.useDeleteProjectWorkflowInstance as any).mockReturnValue(noopMutation)
  })

  it('toggle not visible on completed tab when no agent is selected', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: /completed/i }))

    expect(screen.queryByTitle('Collapse agent log')).not.toBeInTheDocument()
    expect(screen.queryByTitle('Expand agent log')).not.toBeInTheDocument()
  })

  it('toggle appears after selecting an agent from CompletedAgentsTable', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: /completed/i }))
    await user.click(screen.getByTestId('select-agent-btn'))

    expect(screen.getByTitle('Collapse agent log')).toBeInTheDocument()
  })

  it('toggle shows "Expand agent log" title when panel is initially collapsed', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: /completed/i }))
    await user.click(screen.getByTestId('select-agent-btn'))
    // Panel starts expanded, click to collapse
    await user.click(screen.getByTitle('Collapse agent log'))

    expect(screen.getByTitle('Expand agent log')).toBeInTheDocument()
  })

  it('clicking toggle calls setLogPanelCollapsed (toggle title flips)', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: /completed/i }))
    await user.click(screen.getByTestId('select-agent-btn'))

    // Initially expanded
    expect(screen.getByTitle('Collapse agent log')).toBeInTheDocument()

    // Click to collapse
    await user.click(screen.getByTitle('Collapse agent log'))
    expect(screen.getByTitle('Expand agent log')).toBeInTheDocument()

    // Click to expand again
    await user.click(screen.getByTitle('Expand agent log'))
    expect(screen.getByTitle('Collapse agent log')).toBeInTheDocument()
  })
})
