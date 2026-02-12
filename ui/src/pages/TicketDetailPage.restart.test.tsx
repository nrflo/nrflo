import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as ticketsApi from '@/api/tickets'
import * as workflowsApi from '@/api/workflows'
import {
  sampleTicket,
  workflowWithActivePhase,
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

// Mock AgentLogPanel to expose restart controls for testing
let capturedOnRestart: ((sessionId: string) => void) | undefined
let capturedRestartingSessionId: string | null | undefined

vi.mock('@/components/workflow/AgentLogPanel', () => ({
  AgentLogPanel: ({
    activeAgents,
    selectedAgent,
    onRestart,
    restartingSessionId,
  }: {
    activeAgents: Record<string, { agent_type: string; phase?: string; result?: string; session_id?: string }>
    selectedAgent: { phaseName: string } | null
    onRestart?: (sessionId: string) => void
    restartingSessionId?: string | null
  }) => {
    capturedOnRestart = onRestart
    capturedRestartingSessionId = restartingSessionId
    const running = Object.values(activeAgents).filter(a => !a.result)
    if (running.length === 0 && !selectedAgent) return null
    return (
      <div data-testid="running-agent-log">
        {running.map((agent, i) => (
          <div key={i} data-testid={`agent-row-${agent.agent_type}`}>
            <span>{agent.agent_type}</span>
            {onRestart && agent.session_id && !agent.result && (
              <button
                data-testid={`restart-btn-${agent.agent_type}`}
                onClick={() => onRestart(agent.session_id!)}
                disabled={restartingSessionId === agent.session_id}
              >
                Restart
              </button>
            )}
          </div>
        ))}
      </div>
    )
  },
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
}))

// Workflow with a running agent that has a session_id
const workflowWithSessionId: WorkflowResponse = {
  ticket_id: 'TICKET-1',
  has_workflow: true,
  state: {
    workflow: 'feature',
    version: 4,
    current_phase: 'implementation',
    category: 'full',
    phase_order: ['investigation', 'implementation', 'verification'],
    phases: {
      investigation: { status: 'completed', result: 'pass' },
      implementation: { status: 'in_progress' },
    },
    active_agents: {
      'implementor:claude:sonnet': {
        agent_id: 'a1',
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-sonnet-4-5',
        cli: 'claude',
        pid: 12345,
        session_id: 'sess-uuid-123',
        started_at: '2026-01-01T00:00:00Z',
      },
    },
  },
  workflows: ['feature'],
  all_workflows: {},
}

const sessionsData: AgentSessionsResponse = {
  ticket_id: 'TICKET-1',
  sessions: [{
    id: 'sess-uuid-123',
    project_id: 'test-project',
    ticket_id: 'TICKET-1',
    workflow_instance_id: 'wi-1',
    phase: 'implementation',
    workflow: 'feature',
    agent_type: 'implementor',
    model_id: 'claude-sonnet-4-5',
    status: 'running',
    message_count: 10,
    raw_output_size: 2048,
    restart_count: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }],
}

describe('TicketDetailPage - Restart agent', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    capturedOnRestart = undefined
    capturedRestartingSessionId = undefined
  })

  it('passes onRestart callback to AgentLogPanel', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithSessionId)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(sessionsData)

    renderPage()

    await waitFor(() => {
      expect(screen.getByTestId('running-agent-log')).toBeInTheDocument()
    })

    expect(capturedOnRestart).toBeDefined()
    expect(typeof capturedOnRestart).toBe('function')
  })

  it('shows restart button for running agent with session_id', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithSessionId)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(sessionsData)

    renderPage()

    await waitFor(() => {
      expect(screen.getByTestId('restart-btn-implementor')).toBeInTheDocument()
    })
  })

  it('calls restartAgent API with correct parameters when restart clicked', async () => {
    const user = userEvent.setup()
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithSessionId)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(sessionsData)
    vi.mocked(workflowsApi.restartAgent).mockResolvedValue({ status: 'restarting' })

    renderPage()

    await waitFor(() => {
      expect(screen.getByTestId('restart-btn-implementor')).toBeInTheDocument()
    })

    await user.click(screen.getByTestId('restart-btn-implementor'))

    await waitFor(() => {
      expect(workflowsApi.restartAgent).toHaveBeenCalledWith(
        'TICKET-1',
        { workflow: 'feature', session_id: 'sess-uuid-123' }
      )
    })
  })

  it('restartingSessionId is null when no restart is pending', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithSessionId)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(sessionsData)

    renderPage()

    await waitFor(() => {
      expect(screen.getByTestId('running-agent-log')).toBeInTheDocument()
    })

    expect(capturedRestartingSessionId).toBeNull()
  })

  it('does not show restart button when no active agents', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue({
      ...workflowWithActivePhase,
      state: { ...workflowWithActivePhase.state, active_agents: {} },
    })
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(emptySessions)

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Test ticket')).toBeInTheDocument()
    })

    expect(screen.queryByTestId('restart-btn-implementor')).not.toBeInTheDocument()
  })

  it('does not show restart button for agent without session_id', async () => {
    const workflowNoSession: WorkflowResponse = {
      ...workflowWithSessionId,
      state: {
        ...workflowWithSessionId.state,
        active_agents: {
          'implementor:claude:sonnet': {
            agent_id: 'a1',
            agent_type: 'implementor',
            phase: 'implementation',
            model_id: 'claude-sonnet-4-5',
            cli: 'claude',
            pid: 12345,
            started_at: '2026-01-01T00:00:00Z',
            // no session_id
          },
        },
      },
    }
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowNoSession)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(emptySessions)

    renderPage()

    await waitFor(() => {
      expect(screen.getByTestId('running-agent-log')).toBeInTheDocument()
    })

    expect(screen.queryByTestId('restart-btn-implementor')).not.toBeInTheDocument()
  })
})
