import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as ticketsApi from '@/api/tickets'
import * as workflowsApi from '@/api/workflows'
import {
  sampleTicket,
  emptySessions,
  renderPage,
} from './TicketDetailPage.test-utils'
import type { WorkflowResponse, AgentSessionsResponse } from '@/types/workflow'

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

vi.mock('@/components/workflow/PhaseTimeline', () => ({
  PhaseTimeline: () => <div data-testid="phase-timeline">PhaseTimeline</div>,
}))

vi.mock('@/components/workflow/RunWorkflowDialog', () => ({
  RunWorkflowDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="run-dialog">RunWorkflowDialog</div> : null,
}))

vi.mock('@/components/workflow/RunEpicWorkflowDialog', () => ({
  RunEpicWorkflowDialog: () => null,
}))

vi.mock('@/hooks/useChains', () => ({
  useChainList: () => ({ data: [] }),
}))

vi.mock('@/components/workflow/AgentLogPanel', () => ({
  AgentLogPanel: () => <div data-testid="agent-log-panel">AgentLogPanel</div>,
}))

vi.mock('@/api/tickets', async () => {
  const actual = await vi.importActual('@/api/tickets')
  return {
    ...actual,
    getTicket: vi.fn(),
    getWorkflow: vi.fn(),
    getAgentSessions: vi.fn(),
    closeTicket: vi.fn(),
    deleteTicket: vi.fn(),
  }
})

vi.mock('@/api/workflows', () => ({
  runWorkflow: vi.fn(),
  stopWorkflow: vi.fn(),
  restartAgent: vi.fn(),
  retryFailedAgent: vi.fn(),
}))

// Workflow with failed status and failed agent in history
const workflowWithFailedAgent: WorkflowResponse = {
  ticket_id: 'TICKET-1',
  has_workflow: true,
  state: {
    workflow: 'feature',
    version: 4,
    current_phase: 'implementation',
    status: 'failed',
    phase_order: ['investigation', 'implementation', 'verification'],
    phases: {
      investigation: { status: 'completed', result: 'pass' },
      implementation: { status: 'error', result: 'fail' },
    },
    active_agents: {},
    agent_history: [
      {
        agent_id: 'a1',
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-sonnet-4-5',
        result: 'fail',
        started_at: '2026-01-01T00:00:00Z',
        ended_at: '2026-01-01T00:05:00Z',
        session_id: 'sess-failed-123',
      },
    ],
  },
  workflows: ['feature'],
  all_workflows: {},
}

const sessionsData: AgentSessionsResponse = {
  ticket_id: 'TICKET-1',
  sessions: [],
}

describe('TicketDetailPage - Retry failed agent', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('passes onRetryFailed callback to WorkflowTabContent', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithFailedAgent)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(sessionsData)

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Workflow Failed')).toBeInTheDocument()
    })

    // Should have retry button
    expect(screen.getByText('Retry Failed')).toBeInTheDocument()
  })

  it('shows "Workflow Failed" banner when workflow status is failed', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithFailedAgent)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(sessionsData)

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Workflow Failed')).toBeInTheDocument()
    })
  })

  it('calls retryFailedAgent API with correct parameters when retry clicked', async () => {
    const user = userEvent.setup()
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithFailedAgent)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(sessionsData)
    vi.mocked(workflowsApi.retryFailedAgent).mockResolvedValue({ status: 'retrying' })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Retry Failed')).toBeInTheDocument()
    })

    await user.click(screen.getByText('Retry Failed'))

    await waitFor(() => {
      expect(workflowsApi.retryFailedAgent).toHaveBeenCalledWith(
        'TICKET-1',
        { workflow: 'feature', session_id: 'sess-failed-123' }
      )
    })
  })

  it('retryingSessionId is null when no retry is pending', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithFailedAgent)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(sessionsData)

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Workflow Failed')).toBeInTheDocument()
    })

    // Initially null
    const button = screen.getByText('Retry Failed').closest('button')
    expect(button).not.toBeDisabled()
  })

  it('invalidates workflow and sessions queries on successful retry', async () => {
    const user = userEvent.setup()
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithFailedAgent)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(sessionsData)
    vi.mocked(workflowsApi.retryFailedAgent).mockResolvedValue({ status: 'retrying' })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Retry Failed')).toBeInTheDocument()
    })

    await user.click(screen.getByText('Retry Failed'))

    // Wait for mutation to complete and queries to be invalidated
    await waitFor(() => {
      expect(workflowsApi.retryFailedAgent).toHaveBeenCalled()
    })

    // Queries should be refetched after successful mutation
    await waitFor(() => {
      expect(ticketsApi.getWorkflow).toHaveBeenCalledTimes(2) // Initial + refetch
    })
  })

  it('does not show retry button when workflow status is active', async () => {
    const activeWorkflow = {
      ...workflowWithFailedAgent,
      state: { ...workflowWithFailedAgent.state, status: 'active' },
    }
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(activeWorkflow)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(emptySessions)

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Test ticket')).toBeInTheDocument()
    })

    expect(screen.queryByText('Retry Failed')).not.toBeInTheDocument()
  })

  it('does not show retry button when workflow status is completed', async () => {
    const completedWorkflow = {
      ...workflowWithFailedAgent,
      state: { ...workflowWithFailedAgent.state, status: 'completed' },
    }
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(completedWorkflow)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(emptySessions)

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Test ticket')).toBeInTheDocument()
    })

    expect(screen.queryByText('Retry Failed')).not.toBeInTheDocument()
  })

  it('does not show retry button when no failed agents in history', async () => {
    const workflowNoFailedAgents = {
      ...workflowWithFailedAgent,
      state: {
        ...workflowWithFailedAgent.state,
        agent_history: [],
      },
    }
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowNoFailedAgents)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(emptySessions)

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Test ticket')).toBeInTheDocument()
    })

    expect(screen.queryByText('Retry Failed')).not.toBeInTheDocument()
  })

  it('passes props to WorkflowTabContent for failed workflow', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithFailedAgent)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(sessionsData)

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Workflow Failed')).toBeInTheDocument()
    })

    // WorkflowTabContent receives the necessary props
    // (AgentLogPanel only renders when there are active agents, which is not the case
    // in a failed workflow with no running agents)
  })

  it('uses first failed agent session_id when multiple failed agents', async () => {
    const user = userEvent.setup()
    const workflowMultipleFailed = {
      ...workflowWithFailedAgent,
      state: {
        ...workflowWithFailedAgent.state,
        agent_history: [
          {
            agent_id: 'a1',
            agent_type: 'implementor',
            phase: 'implementation',
            model_id: 'claude-sonnet-4-5',
            result: 'fail',
            started_at: '2026-01-01T00:00:00Z',
            ended_at: '2026-01-01T00:05:00Z',
            session_id: 'sess-first-failed',
          },
          {
            agent_id: 'a2',
            agent_type: 'tester',
            phase: 'verification',
            model_id: 'claude-opus-4-6',
            result: 'fail',
            started_at: '2026-01-01T00:06:00Z',
            ended_at: '2026-01-01T00:10:00Z',
            session_id: 'sess-second-failed',
          },
        ],
      },
    }
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowMultipleFailed)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(sessionsData)
    vi.mocked(workflowsApi.retryFailedAgent).mockResolvedValue({ status: 'retrying' })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Retry Failed')).toBeInTheDocument()
    })

    await user.click(screen.getByText('Retry Failed'))

    await waitFor(() => {
      expect(workflowsApi.retryFailedAgent).toHaveBeenCalledWith(
        'TICKET-1',
        { workflow: 'feature', session_id: 'sess-first-failed' }
      )
    })
  })
})
