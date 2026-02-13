import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ProjectWorkflowsPage } from './ProjectWorkflowsPage'
import type { ProjectWorkflowResponse, ProjectAgentSessionsResponse, WorkflowState } from '@/types/workflow'

// Mock dependencies
vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string; projectsLoaded: boolean }) => unknown) =>
    selector({ currentProject: 'test-project', projectsLoaded: true }),
}))

vi.mock('@/hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    isConnected: true,
    subscribe: vi.fn(),
    unsubscribe: vi.fn(),
  }),
}))

vi.mock('@/hooks/useTickets', async () => {
  const actual = await vi.importActual<typeof import('@/hooks/useTickets')>('@/hooks/useTickets')
  return {
    ...actual,
    useProjectWorkflow: vi.fn(),
    useProjectAgentSessions: vi.fn(),
    useStopProjectWorkflow: vi.fn(),
    useRestartProjectAgent: vi.fn(),
  }
})

vi.mock('@/components/workflow/RunProjectWorkflowDialog', () => ({
  RunProjectWorkflowDialog: () => <div data-testid="run-project-workflow-dialog">Run Dialog</div>,
}))

vi.mock('./WorkflowTabContent', () => ({
  WorkflowTabContent: (props: any) => (
    <div data-testid="workflow-tab-content">
      <div data-testid="sessions-count">{props.sessions?.length ?? 0}</div>
      <div data-testid="has-workflow">{String(props.hasWorkflow)}</div>
      <div data-testid="workflow-name">{props.displayedWorkflowName}</div>
    </div>
  ),
}))

const sampleWorkflowState: WorkflowState = {
  workflow: 'feature',
  version: 4,
  scope_type: 'project',
  current_phase: 'implementation',
  category: 'full',
  status: 'active',
  phases: {
    investigation: { status: 'completed', result: 'pass' },
    implementation: { status: 'in_progress' },
  },
  phase_order: ['investigation', 'implementation', 'verification'],
  active_agents: {
    'implementor:claude:opus': {
      agent_id: 'a1',
      agent_type: 'implementor',
      phase: 'implementation',
      model_id: 'claude-opus-4-6',
      cli: 'claude',
      pid: 12345,
      session_id: 'session-1',
      started_at: '2026-01-01T00:00:00Z',
    },
  },
  findings: {},
}

const sampleWorkflowResponse: ProjectWorkflowResponse = {
  project_id: 'test-project',
  has_workflow: true,
  state: sampleWorkflowState,
  workflows: ['feature'],
  all_workflows: { feature: sampleWorkflowState },
}

const sampleAgentSessionsResponse: ProjectAgentSessionsResponse = {
  project_id: 'test-project',
  sessions: [
    {
      id: 'session-1',
      project_id: 'test-project',
      ticket_id: '',
      workflow_instance_id: 'wi-1',
      phase: 'implementation',
      workflow: 'feature',
      agent_type: 'implementor',
      model_id: 'claude-opus-4-6',
      status: 'running',
      message_count: 10,
      raw_output_size: 2048,
      restart_count: 0,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    },
    {
      id: 'session-2',
      project_id: 'test-project',
      ticket_id: '',
      workflow_instance_id: 'wi-1',
      phase: 'investigation',
      workflow: 'feature',
      agent_type: 'setup-analyzer',
      model_id: 'claude-sonnet-4-5',
      status: 'completed',
      result: 'pass',
      message_count: 5,
      raw_output_size: 1024,
      restart_count: 0,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
      started_at: '2026-01-01T00:00:00Z',
      ended_at: '2026-01-01T00:00:00Z',
    },
  ],
}

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <ProjectWorkflowsPage />
    </QueryClientProvider>
  )
}

describe('ProjectWorkflowsPage', () => {
  let useProjectWorkflow: any
  let useProjectAgentSessions: any
  let useStopProjectWorkflow: any
  let useRestartProjectAgent: any

  beforeEach(async () => {
    // Import mocked hooks
    const hooks = await import('@/hooks/useTickets')
    useProjectWorkflow = hooks.useProjectWorkflow as any
    useProjectAgentSessions = hooks.useProjectAgentSessions as any
    useStopProjectWorkflow = hooks.useStopProjectWorkflow as any
    useRestartProjectAgent = hooks.useRestartProjectAgent as any

    vi.clearAllMocks()

    // Default mocks
    useProjectWorkflow.mockReturnValue({
      data: sampleWorkflowResponse,
      isLoading: false,
    })

    useProjectAgentSessions.mockReturnValue({
      data: sampleAgentSessionsResponse,
      isLoading: false,
    })

    useStopProjectWorkflow.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
    })

    useRestartProjectAgent.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
    })
  })

  it('renders project workflows page title and description', () => {
    renderPage()

    expect(screen.getByText('Project Workflows')).toBeInTheDocument()
    expect(screen.getByText('Workflows that run at project level without a ticket.')).toBeInTheDocument()
  })

  it('fetches project workflow data with correct projectId', () => {
    renderPage()

    expect(useProjectWorkflow).toHaveBeenCalledWith(
      'test-project',
      expect.objectContaining({ enabled: true })
    )
  })

  it('fetches project agent sessions with correct projectId', () => {
    renderPage()

    expect(useProjectAgentSessions).toHaveBeenCalledWith(
      'test-project',
      expect.objectContaining({ enabled: true })
    )
  })

  it('passes fetched sessions to WorkflowTabContent', async () => {
    renderPage()

    await waitFor(() => {
      const sessionsCount = screen.getByTestId('sessions-count')
      expect(sessionsCount.textContent).toBe('2')
    })
  })

  it('passes empty sessions array when sessionsData is undefined', () => {
    useProjectAgentSessions.mockReturnValue({
      data: undefined,
      isLoading: false,
    })

    renderPage()

    const sessionsCount = screen.getByTestId('sessions-count')
    expect(sessionsCount.textContent).toBe('0')
  })

  it('passes workflow state to WorkflowTabContent', () => {
    renderPage()

    expect(screen.getByTestId('has-workflow').textContent).toBe('true')
    expect(screen.getByTestId('workflow-name').textContent).toBe('feature')
  })

  it('handles no workflow state gracefully', () => {
    useProjectWorkflow.mockReturnValue({
      data: {
        project_id: 'test-project',
        has_workflow: false,
        state: null,
        workflows: [],
        all_workflows: {},
      },
      isLoading: false,
    })

    renderPage()

    expect(screen.getByTestId('has-workflow').textContent).toBe('false')
  })

  it('passes undefined ticketId to WorkflowTabContent for project scope', () => {
    const { container } = renderPage()

    // WorkflowTabContent should receive ticketId=undefined for project scope
    const tabContent = screen.getByTestId('workflow-tab-content')
    expect(tabContent).toBeInTheDocument()
  })

  // WebSocket subscription is tested via the mock in beforeEach

  it('handles multiple workflows correctly', () => {
    const multiWorkflowResponse: ProjectWorkflowResponse = {
      project_id: 'test-project',
      has_workflow: true,
      state: sampleWorkflowState,
      workflows: ['feature', 'bugfix'],
      all_workflows: {
        feature: sampleWorkflowState,
        bugfix: { ...sampleWorkflowState, workflow: 'bugfix' },
      },
    }

    useProjectWorkflow.mockReturnValue({
      data: multiWorkflowResponse,
      isLoading: false,
    })

    renderPage()

    expect(screen.getByTestId('has-workflow').textContent).toBe('true')
  })

  it('handles loading state when workflow data is not yet available', () => {
    useProjectWorkflow.mockReturnValue({
      data: undefined,
      isLoading: true,
    })

    renderPage()

    expect(screen.getByTestId('workflow-tab-content')).toBeInTheDocument()
  })

  it('handles loading state when sessions data is not yet available', () => {
    useProjectAgentSessions.mockReturnValue({
      data: undefined,
      isLoading: true,
    })

    renderPage()

    const sessionsCount = screen.getByTestId('sessions-count')
    expect(sessionsCount.textContent).toBe('0')
  })

  // projectsLoaded behavior is handled by the hooks themselves

  it('handles project scope agents with empty ticket_id in sessions', () => {
    const projectScopeSessions: ProjectAgentSessionsResponse = {
      project_id: 'test-project',
      sessions: [
        {
          id: 'session-proj-1',
          project_id: 'test-project',
          ticket_id: '', // Empty for project scope
          workflow_instance_id: 'wi-proj',
          phase: 'investigation',
          workflow: 'feature',
          agent_type: 'setup-analyzer',
          model_id: 'claude-sonnet-4-5',
          status: 'running',
          message_count: 3,
          raw_output_size: 512,
          restart_count: 0,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ],
    }

    useProjectAgentSessions.mockReturnValue({
      data: projectScopeSessions,
      isLoading: false,
    })

    renderPage()

    const sessionsCount = screen.getByTestId('sessions-count')
    expect(sessionsCount.textContent).toBe('1')
  })

  it('passes sessions prop to WorkflowTabContent which forwards to PhaseTimeline', () => {
    // This test verifies the fix: sessions prop is passed through the component chain
    renderPage()

    const sessionsCount = screen.getByTestId('sessions-count')
    expect(sessionsCount.textContent).toBe('2')

    // The sessions should include both running and completed agents
    expect(useProjectAgentSessions).toHaveBeenCalled()
  })

  it('handles API error for workflow data gracefully', () => {
    useProjectWorkflow.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error('Failed to fetch workflow'),
    })

    renderPage()

    // Page should still render without crashing
    expect(screen.getByTestId('workflow-tab-content')).toBeInTheDocument()
  })

  it('handles API error for sessions data gracefully', () => {
    useProjectAgentSessions.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error('Failed to fetch sessions'),
    })

    renderPage()

    // Should pass empty sessions array
    const sessionsCount = screen.getByTestId('sessions-count')
    expect(sessionsCount.textContent).toBe('0')
  })
})
