import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as ticketsApi from '@/api/tickets'
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

vi.mock('@/hooks/useWebSocketSubscription', () => ({
  useWebSocketSubscription: () => ({ isConnected: true }),
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

// AgentLogPanel no longer accepts onRestart/restartingSessionId (ticket nrworkflow-9ada6f).
// Restart for running agents is handled by ActiveAgentsPanel.
vi.mock('@/components/workflow/AgentLogPanel', () => ({
  AgentLogPanel: ({
    activeAgents,
    selectedAgent,
  }: {
    activeAgents: Record<string, { agent_type: string; phase?: string; result?: string; session_id?: string }>
    selectedAgent: { phaseName: string } | null
  }) => {
    const running = Object.values(activeAgents).filter(a => !a.result)
    if (running.length === 0 && !selectedAgent) return null
    return (
      <div data-testid="running-agent-log">
        {running.map((agent, i) => (
          <div key={i} data-testid={`agent-row-${agent.agent_type}`}>
            <span>{agent.agent_type}</span>
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
const sessionState = {
  workflow: 'feature',
  instance_id: 'inst-sess-01',
  version: 4,
  current_phase: 'implementation',
  phase_order: ['investigation', 'implementation', 'verification'],
  phases: {
    investigation: { status: 'completed' as const, result: 'pass' as const },
    implementation: { status: 'in_progress' as const },
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
}
const workflowWithSessionId: WorkflowResponse = {
  ticket_id: 'TICKET-1',
  has_workflow: true,
  state: sessionState,
  workflows: ['feature'],
  all_workflows: { 'inst-sess-01': sessionState },
}

const sessionsData: AgentSessionsResponse = {
  ticket_id: 'TICKET-1',
  sessions: [{
    id: 'sess-uuid-123',
    project_id: 'test-project',
    ticket_id: 'TICKET-1',
    workflow_instance_id: 'inst-sess-01',
    phase: 'implementation',
    workflow: 'feature',
    agent_type: 'implementor',
    model_id: 'claude-sonnet-4-5',
    status: 'running',
    message_count: 10,
    restart_count: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }],
}

/** Navigate to workflow tab after page loads */
async function goToWorkflowTab() {
  const user = userEvent.setup()
  await waitFor(() => {
    expect(screen.getByText('Test ticket')).toBeInTheDocument()
  })
  await user.click(screen.getByText('Workflow'))
  return user
}

describe('TicketDetailPage - Restart agent', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows running agent in AgentLogPanel', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithSessionId)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(sessionsData)

    renderPage()
    await goToWorkflowTab()

    await waitFor(() => {
      expect(screen.getByTestId('running-agent-log')).toBeInTheDocument()
    })
    expect(screen.getByTestId('agent-row-implementor')).toBeInTheDocument()
  })

  it('does not show AgentLogPanel when no active agents', async () => {
    const stateNoAgents = { ...workflowWithActivePhase.state, active_agents: {} }
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue({
      ...workflowWithActivePhase,
      state: stateNoAgents,
      all_workflows: { 'inst-active-01': stateNoAgents },
    })
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(emptySessions)

    renderPage()
    await goToWorkflowTab()

    expect(screen.queryByTestId('running-agent-log')).not.toBeInTheDocument()
  })

  it('shows running agent without session_id in AgentLogPanel', async () => {
    const noSessionState = {
      ...sessionState,
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
    }
    const workflowNoSession: WorkflowResponse = {
      ...workflowWithSessionId,
      state: noSessionState,
      all_workflows: { 'inst-sess-01': noSessionState },
    }
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowNoSession)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(emptySessions)

    renderPage()
    await goToWorkflowTab()

    await waitFor(() => {
      expect(screen.getByTestId('running-agent-log')).toBeInTheDocument()
    })
    expect(screen.getByTestId('agent-row-implementor')).toBeInTheDocument()
  })
})
