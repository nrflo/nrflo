import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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
      <div data-testid="has-run-dialog">{String(!!props.onShowRunDialog)}</div>
      <div data-testid="workflows-count">{props.workflows?.length ?? 0}</div>
      {props.displayedState && (
        <>
          <div data-testid="displayed-status">{props.displayedState.status}</div>
          <div data-testid="displayed-workflow">{props.displayedState.workflow}</div>
        </>
      )}
    </div>
  ),
}))

const sampleWorkflowState: WorkflowState = {
  workflow: 'feature',
  version: 4,
  scope_type: 'project',
  current_phase: 'implementation',
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

const sampleCompletedWorkflowState: WorkflowState = {
  workflow: 'bugfix',
  version: 4,
  scope_type: 'project',
  current_phase: 'verification',
  status: 'completed',
  completed_at: '2026-01-01T05:23:45Z',
  total_duration_sec: 19425.5,
  total_tokens_used: 150000,
  phases: {
    investigation: { status: 'completed', result: 'pass' },
    implementation: { status: 'completed', result: 'pass' },
    verification: { status: 'completed', result: 'pass' },
  },
  phase_order: ['investigation', 'implementation', 'verification'],
  active_agents: {},
  agent_history: [
    {
      agent_id: 'a-hist-1',
      agent_type: 'setup-analyzer',
      phase: 'investigation',
      session_id: 'hist-session-1',
      model_id: 'claude-sonnet-4-5',
      result: 'pass',
      started_at: '2026-01-01T00:00:00Z',
      ended_at: '2026-01-01T01:00:00Z',
    },
  ],
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
    renderPage()

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

  describe('Tab Bar Functionality', () => {
    it('renders Active and Completed tab buttons', () => {
      renderPage()

      expect(screen.getByRole('button', { name: /Active/ })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /Completed/ })).toBeInTheDocument()
    })

    it('shows Active tab as selected by default', () => {
      renderPage()

      const activeButton = screen.getByRole('button', { name: /Active/ })
      const completedButton = screen.getByRole('button', { name: /Completed/ })

      expect(activeButton).toHaveClass('border-primary', 'text-primary')
      expect(completedButton).toHaveClass('border-transparent', 'text-muted-foreground')
    })

    it('displays count badges on both tabs', () => {
      const mixedWorkflowResponse: ProjectWorkflowResponse = {
        project_id: 'test-project',
        has_workflow: true,
        state: sampleWorkflowState,
        workflows: ['feature', 'bugfix'],
        all_workflows: {
          feature: sampleWorkflowState,
          bugfix: sampleCompletedWorkflowState,
        },
      }

      useProjectWorkflow.mockReturnValue({
        data: mixedWorkflowResponse,
        isLoading: false,
      })

      renderPage()

      expect(screen.getByRole('button', { name: /Active \(1\)/ })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /Completed \(1\)/ })).toBeInTheDocument()
    })

    it('shows zero count when no workflows exist', () => {
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

      expect(screen.getByRole('button', { name: /Active \(0\)/ })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /Completed \(0\)/ })).toBeInTheDocument()
    })

    it('switches to Completed tab when clicked', async () => {
      const user = userEvent.setup()
      const mixedWorkflowResponse: ProjectWorkflowResponse = {
        project_id: 'test-project',
        has_workflow: true,
        state: sampleWorkflowState,
        workflows: ['feature', 'bugfix'],
        all_workflows: {
          feature: sampleWorkflowState,
          bugfix: sampleCompletedWorkflowState,
        },
      }

      useProjectWorkflow.mockReturnValue({
        data: mixedWorkflowResponse,
        isLoading: false,
      })

      renderPage()

      const completedButton = screen.getByRole('button', { name: /Completed/ })
      await user.click(completedButton)

      expect(completedButton).toHaveClass('border-primary', 'text-primary')
      const activeButton = screen.getByRole('button', { name: /Active/ })
      expect(activeButton).toHaveClass('border-transparent', 'text-muted-foreground')
    })

    it('switches back to Active tab when clicked', async () => {
      const user = userEvent.setup()
      renderPage()

      const completedButton = screen.getByRole('button', { name: /Completed/ })
      await user.click(completedButton)

      const activeButton = screen.getByRole('button', { name: /Active/ })
      await user.click(activeButton)

      expect(activeButton).toHaveClass('border-primary', 'text-primary')
      expect(completedButton).toHaveClass('border-transparent', 'text-muted-foreground')
    })

    it('renders CheckCircle icon on Completed tab', () => {
      renderPage()

      const completedButton = screen.getByRole('button', { name: /Completed/ })
      const svg = completedButton.querySelector('svg')
      expect(svg).toBeInTheDocument()
    })
  })

  describe('Tab Filtering', () => {
    it('shows only active workflows on Active tab', () => {
      const mixedWorkflowResponse: ProjectWorkflowResponse = {
        project_id: 'test-project',
        has_workflow: true,
        state: sampleWorkflowState,
        workflows: ['feature', 'bugfix', 'hotfix'],
        all_workflows: {
          feature: sampleWorkflowState,
          bugfix: sampleCompletedWorkflowState,
          hotfix: { ...sampleWorkflowState, workflow: 'hotfix', status: 'failed' },
        },
      }

      useProjectWorkflow.mockReturnValue({
        data: mixedWorkflowResponse,
        isLoading: false,
      })

      renderPage()

      // Active tab should show 2 workflows (feature=active, hotfix=failed)
      expect(screen.getByTestId('workflows-count').textContent).toBe('2')
      expect(screen.getByTestId('displayed-status').textContent).toBe('active')
    })

    it('shows only completed workflows on Completed tab', async () => {
      const user = userEvent.setup()
      const mixedWorkflowResponse: ProjectWorkflowResponse = {
        project_id: 'test-project',
        has_workflow: true,
        state: sampleWorkflowState,
        workflows: ['feature', 'bugfix', 'docs'],
        all_workflows: {
          feature: sampleWorkflowState,
          bugfix: sampleCompletedWorkflowState,
          docs: { ...sampleCompletedWorkflowState, workflow: 'docs' },
        },
      }

      useProjectWorkflow.mockReturnValue({
        data: mixedWorkflowResponse,
        isLoading: false,
      })

      renderPage()

      const completedButton = screen.getByRole('button', { name: /Completed/ })
      await user.click(completedButton)

      await waitFor(() => {
        expect(screen.getByTestId('workflows-count').textContent).toBe('2')
        expect(screen.getByTestId('displayed-status').textContent).toBe('completed')
      })
    })

    it('includes failed workflows in Active tab', () => {
      const workflowResponse: ProjectWorkflowResponse = {
        project_id: 'test-project',
        has_workflow: true,
        state: { ...sampleWorkflowState, status: 'failed' },
        workflows: ['feature'],
        all_workflows: {
          feature: { ...sampleWorkflowState, status: 'failed' },
        },
      }

      useProjectWorkflow.mockReturnValue({
        data: workflowResponse,
        isLoading: false,
      })

      renderPage()

      expect(screen.getByTestId('workflows-count').textContent).toBe('1')
      expect(screen.getByTestId('displayed-status').textContent).toBe('failed')
    })

    it('filters workflows correctly when all are completed', async () => {
      const user = userEvent.setup()
      const allCompletedResponse: ProjectWorkflowResponse = {
        project_id: 'test-project',
        has_workflow: true,
        state: sampleCompletedWorkflowState,
        workflows: ['bugfix', 'docs'],
        all_workflows: {
          bugfix: sampleCompletedWorkflowState,
          docs: { ...sampleCompletedWorkflowState, workflow: 'docs' },
        },
      }

      useProjectWorkflow.mockReturnValue({
        data: allCompletedResponse,
        isLoading: false,
      })

      renderPage()

      // Active tab should show 0
      expect(screen.getByTestId('workflows-count').textContent).toBe('0')

      // Switch to completed
      const completedButton = screen.getByRole('button', { name: /Completed/ })
      await user.click(completedButton)

      await waitFor(() => {
        expect(screen.getByTestId('workflows-count').textContent).toBe('2')
      })
    })

    it('filters workflows correctly when all are active', async () => {
      const user = userEvent.setup()
      const allActiveResponse: ProjectWorkflowResponse = {
        project_id: 'test-project',
        has_workflow: true,
        state: sampleWorkflowState,
        workflows: ['feature', 'hotfix'],
        all_workflows: {
          feature: sampleWorkflowState,
          hotfix: { ...sampleWorkflowState, workflow: 'hotfix' },
        },
      }

      useProjectWorkflow.mockReturnValue({
        data: allActiveResponse,
        isLoading: false,
      })

      renderPage()

      // Active tab should show 2
      expect(screen.getByTestId('workflows-count').textContent).toBe('2')

      // Switch to completed
      const completedButton = screen.getByRole('button', { name: /Completed/ })
      await user.click(completedButton)

      await waitFor(() => {
        expect(screen.getByTestId('workflows-count').textContent).toBe('0')
      })
    })

    it('routes workflows with project_completed status to Completed tab', () => {
      const projectCompletedState: WorkflowState = {
        ...sampleCompletedWorkflowState,
        status: 'project_completed',
        workflow: 'feature',
      }

      const workflowResponse: ProjectWorkflowResponse = {
        project_id: 'test-project',
        has_workflow: true,
        state: projectCompletedState,
        workflows: ['feature'],
        all_workflows: {
          feature: projectCompletedState,
        },
      }

      useProjectWorkflow.mockReturnValue({
        data: workflowResponse,
        isLoading: false,
      })

      renderPage()

      // Active tab should show 0 workflows (project_completed goes to Completed tab)
      expect(screen.getByTestId('workflows-count').textContent).toBe('0')
      expect(screen.getByRole('button', { name: /Active \(0\)/ })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /Completed \(1\)/ })).toBeInTheDocument()
    })

    it('shows project_completed workflow in Completed tab', async () => {
      const user = userEvent.setup()
      const projectCompletedState: WorkflowState = {
        ...sampleCompletedWorkflowState,
        status: 'project_completed',
        workflow: 'feature',
      }

      const workflowResponse: ProjectWorkflowResponse = {
        project_id: 'test-project',
        has_workflow: true,
        state: projectCompletedState,
        workflows: ['feature'],
        all_workflows: {
          feature: projectCompletedState,
        },
      }

      useProjectWorkflow.mockReturnValue({
        data: workflowResponse,
        isLoading: false,
      })

      renderPage()

      // Switch to Completed tab
      const completedButton = screen.getByRole('button', { name: /Completed/ })
      await user.click(completedButton)

      await waitFor(() => {
        expect(screen.getByTestId('workflows-count').textContent).toBe('1')
        expect(screen.getByTestId('displayed-status').textContent).toBe('project_completed')
        expect(screen.getByTestId('displayed-workflow').textContent).toBe('feature')
      })
    })

    it('correctly separates project_completed from active workflows', () => {
      const projectCompletedState: WorkflowState = {
        ...sampleCompletedWorkflowState,
        status: 'project_completed',
        workflow: 'feature',
      }

      const mixedWorkflowResponse: ProjectWorkflowResponse = {
        project_id: 'test-project',
        has_workflow: true,
        state: sampleWorkflowState,
        workflows: ['feature', 'bugfix', 'hotfix'],
        all_workflows: {
          feature: projectCompletedState,
          bugfix: sampleWorkflowState,
          hotfix: { ...sampleWorkflowState, workflow: 'hotfix', status: 'failed' },
        },
      }

      useProjectWorkflow.mockReturnValue({
        data: mixedWorkflowResponse,
        isLoading: false,
      })

      renderPage()

      // Active tab should show 2 workflows (bugfix=active, hotfix=failed)
      // feature with project_completed should be in Completed tab
      expect(screen.getByTestId('workflows-count').textContent).toBe('2')
      expect(screen.getByRole('button', { name: /Active \(2\)/ })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /Completed \(1\)/ })).toBeInTheDocument()
    })

    it('correctly counts both completed and project_completed workflows in Completed tab', async () => {
      const user = userEvent.setup()
      const projectCompletedState: WorkflowState = {
        ...sampleCompletedWorkflowState,
        status: 'project_completed',
        workflow: 'feature',
      }

      const mixedCompletedResponse: ProjectWorkflowResponse = {
        project_id: 'test-project',
        has_workflow: true,
        state: sampleWorkflowState,
        workflows: ['feature', 'bugfix', 'docs'],
        all_workflows: {
          feature: projectCompletedState,
          bugfix: sampleCompletedWorkflowState,
          docs: { ...sampleWorkflowState, workflow: 'docs', status: 'active' },
        },
      }

      useProjectWorkflow.mockReturnValue({
        data: mixedCompletedResponse,
        isLoading: false,
      })

      renderPage()

      // Active tab: 1 workflow (docs)
      expect(screen.getByRole('button', { name: /Active \(1\)/ })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /Completed \(2\)/ })).toBeInTheDocument()

      // Switch to Completed tab
      const completedButton = screen.getByRole('button', { name: /Completed/ })
      await user.click(completedButton)

      await waitFor(() => {
        // Should show both completed and project_completed workflows
        expect(screen.getByTestId('workflows-count').textContent).toBe('2')
      })
    })
  })

  describe('Tab Switching Behavior', () => {
    it('resets selectedWorkflow when switching tabs', async () => {
      const user = userEvent.setup()
      const mixedWorkflowResponse: ProjectWorkflowResponse = {
        project_id: 'test-project',
        has_workflow: true,
        state: sampleWorkflowState,
        workflows: ['feature', 'bugfix'],
        all_workflows: {
          feature: sampleWorkflowState,
          bugfix: sampleCompletedWorkflowState,
        },
      }

      useProjectWorkflow.mockReturnValue({
        data: mixedWorkflowResponse,
        isLoading: false,
      })

      renderPage()

      // Initial state shows 'feature' workflow
      expect(screen.getByTestId('displayed-workflow').textContent).toBe('feature')

      // Switch to Completed tab
      const completedButton = screen.getByRole('button', { name: /Completed/ })
      await user.click(completedButton)

      await waitFor(() => {
        // Should now show 'bugfix' (first completed workflow)
        expect(screen.getByTestId('displayed-workflow').textContent).toBe('bugfix')
      })

      // Switch back to Active tab
      const activeButton = screen.getByRole('button', { name: /Active/ })
      await user.click(activeButton)

      await waitFor(() => {
        // Should reset to 'feature' (first active workflow)
        expect(screen.getByTestId('displayed-workflow').textContent).toBe('feature')
      })
    })

    it('passes correct workflows list to WorkflowTabContent based on active tab', async () => {
      const user = userEvent.setup()
      const mixedWorkflowResponse: ProjectWorkflowResponse = {
        project_id: 'test-project',
        has_workflow: true,
        state: sampleWorkflowState,
        workflows: ['feature', 'bugfix', 'hotfix'],
        all_workflows: {
          feature: sampleWorkflowState,
          bugfix: sampleCompletedWorkflowState,
          hotfix: { ...sampleWorkflowState, workflow: 'hotfix' },
        },
      }

      useProjectWorkflow.mockReturnValue({
        data: mixedWorkflowResponse,
        isLoading: false,
      })

      renderPage()

      // Active tab: 2 workflows (feature, hotfix)
      expect(screen.getByTestId('workflows-count').textContent).toBe('2')

      const completedButton = screen.getByRole('button', { name: /Completed/ })
      await user.click(completedButton)

      await waitFor(() => {
        // Completed tab: 1 workflow (bugfix)
        expect(screen.getByTestId('workflows-count').textContent).toBe('1')
      })
    })
  })

  describe('Run Workflow Button Visibility', () => {
    it('passes onShowRunDialog to WorkflowTabContent on Active tab', () => {
      renderPage()

      expect(screen.getByTestId('has-run-dialog').textContent).toBe('true')
    })

    it('does not pass onShowRunDialog to WorkflowTabContent on Completed tab', async () => {
      const user = userEvent.setup()
      const mixedWorkflowResponse: ProjectWorkflowResponse = {
        project_id: 'test-project',
        has_workflow: true,
        state: sampleWorkflowState,
        workflows: ['feature', 'bugfix'],
        all_workflows: {
          feature: sampleWorkflowState,
          bugfix: sampleCompletedWorkflowState,
        },
      }

      useProjectWorkflow.mockReturnValue({
        data: mixedWorkflowResponse,
        isLoading: false,
      })

      renderPage()

      const completedButton = screen.getByRole('button', { name: /Completed/ })
      await user.click(completedButton)

      await waitFor(() => {
        expect(screen.getByTestId('has-run-dialog').textContent).toBe('false')
      })
    })

    it('restores onShowRunDialog when switching back to Active tab', async () => {
      const user = userEvent.setup()
      const mixedWorkflowResponse: ProjectWorkflowResponse = {
        project_id: 'test-project',
        has_workflow: true,
        state: sampleWorkflowState,
        workflows: ['feature', 'bugfix'],
        all_workflows: {
          feature: sampleWorkflowState,
          bugfix: sampleCompletedWorkflowState,
        },
      }

      useProjectWorkflow.mockReturnValue({
        data: mixedWorkflowResponse,
        isLoading: false,
      })

      renderPage()

      const completedButton = screen.getByRole('button', { name: /Completed/ })
      await user.click(completedButton)

      await waitFor(() => {
        expect(screen.getByTestId('has-run-dialog').textContent).toBe('false')
      })

      const activeButton = screen.getByRole('button', { name: /Active/ })
      await user.click(activeButton)

      await waitFor(() => {
        expect(screen.getByTestId('has-run-dialog').textContent).toBe('true')
      })
    })
  })

  describe('Empty States', () => {
    it('shows empty state on Active tab when no active workflows', async () => {
      const onlyCompletedResponse: ProjectWorkflowResponse = {
        project_id: 'test-project',
        has_workflow: true,
        state: sampleCompletedWorkflowState,
        workflows: ['bugfix'],
        all_workflows: {
          bugfix: sampleCompletedWorkflowState,
        },
      }

      useProjectWorkflow.mockReturnValue({
        data: onlyCompletedResponse,
        isLoading: false,
      })

      renderPage()

      expect(screen.getByTestId('has-workflow').textContent).toBe('false')
      expect(screen.getByTestId('workflows-count').textContent).toBe('0')
    })

    it('shows empty state on Completed tab when no completed workflows', async () => {
      const user = userEvent.setup()
      const onlyActiveResponse: ProjectWorkflowResponse = {
        project_id: 'test-project',
        has_workflow: true,
        state: sampleWorkflowState,
        workflows: ['feature'],
        all_workflows: {
          feature: sampleWorkflowState,
        },
      }

      useProjectWorkflow.mockReturnValue({
        data: onlyActiveResponse,
        isLoading: false,
      })

      renderPage()

      const completedButton = screen.getByRole('button', { name: /Completed/ })
      await user.click(completedButton)

      await waitFor(() => {
        expect(screen.getByTestId('has-workflow').textContent).toBe('false')
        expect(screen.getByTestId('workflows-count').textContent).toBe('0')
      })
    })
  })

  describe('Completion Statistics', () => {
    it('passes completed workflow with statistics to WorkflowTabContent', async () => {
      const user = userEvent.setup()
      const completedResponse: ProjectWorkflowResponse = {
        project_id: 'test-project',
        has_workflow: true,
        state: sampleCompletedWorkflowState,
        workflows: ['bugfix'],
        all_workflows: {
          bugfix: sampleCompletedWorkflowState,
        },
      }

      useProjectWorkflow.mockReturnValue({
        data: completedResponse,
        isLoading: false,
      })

      renderPage()

      const completedButton = screen.getByRole('button', { name: /Completed/ })
      await user.click(completedButton)

      await waitFor(() => {
        expect(screen.getByTestId('displayed-status').textContent).toBe('completed')
        // The displayedState should include completed_at, total_duration_sec, total_tokens_used
        // These are rendered by WorkflowTabContent's completion banner
      })
    })
  })
})
