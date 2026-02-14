import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as ticketsApi from '@/api/tickets'
import {
  sampleTicket,
  workflowWithActivePhase,
  workflowNoActivePhase,
  emptySessions,
  renderPage,
} from './TicketDetailPage.test-utils'

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

vi.mock('@/components/workflow/AgentLogPanel', () => ({
  AgentLogPanel: ({
    activeAgents,
    collapsed,
    onToggleCollapse,
    selectedAgent,
    onAgentSelect,
  }: {
    activeAgents: Record<string, { agent_type: string; phase?: string; result?: string }>
    collapsed: boolean
    onToggleCollapse: () => void
    selectedAgent: { phaseName: string } | null
    onAgentSelect: (data: { phaseName: string; agent?: { agent_type: string; phase?: string } } | null) => void
  }) => {
    const running = Object.values(activeAgents).filter(a => !a.result)
    if (running.length === 0 && !selectedAgent) return null
    return (
      <div data-testid="running-agent-log">
        <span>{collapsed ? 'collapsed' : 'expanded'}</span>
        <button data-testid="toggle-collapse" onClick={onToggleCollapse}>Toggle</button>
        {running.map((agent, i) => (
          <button
            key={i}
            data-testid={`agent-row-${agent.agent_type}`}
            onClick={() => onAgentSelect({ phaseName: agent.phase || agent.agent_type, agent })}
          >
            {agent.agent_type}
          </button>
        ))}
        {selectedAgent && (
          <div data-testid="agent-detail">Detail: {selectedAgent.phaseName}</div>
        )}
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
}))

/** Navigate to workflow tab after page loads */
async function goToWorkflowTab() {
  const user = userEvent.setup()
  await waitFor(() => {
    expect(screen.getByText('Test ticket')).toBeInTheDocument()
  })
  await user.click(screen.getByText('Workflow'))
  return user
}

describe('TicketDetailPage - RunningAgentLog integration', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(emptySessions)
  })

  it('shows RunningAgentLog when hasActivePhase is true', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithActivePhase)

    renderPage()
    await goToWorkflowTab()

    await waitFor(() => {
      expect(screen.getByTestId('running-agent-log')).toBeInTheDocument()
    })
  })

  it('does not show RunningAgentLog when no active phase', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowNoActivePhase)

    renderPage()
    await goToWorkflowTab()

    expect(screen.queryByTestId('running-agent-log')).not.toBeInTheDocument()
  })

  it('does not show RunningAgentLog when no workflow', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue({
      ticket_id: 'TICKET-1',
      has_workflow: false,
      state: {} as never,
    })

    renderPage()
    await goToWorkflowTab()

    expect(screen.queryByTestId('running-agent-log')).not.toBeInTheDocument()
  })

  it('shows agent detail in panel when agent in log is clicked', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithActivePhase)

    renderPage()
    const user = await goToWorkflowTab()

    await waitFor(() => {
      expect(screen.getByTestId('running-agent-log')).toBeInTheDocument()
    })

    await user.click(screen.getByTestId('agent-row-implementor'))

    await waitFor(() => {
      expect(screen.getByTestId('agent-detail')).toBeInTheDocument()
    })
    expect(screen.getByText(/Detail: implementation/)).toBeInTheDocument()
  })

  it('toggles log panel collapse state', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithActivePhase)

    renderPage()
    const user = await goToWorkflowTab()

    await waitFor(() => {
      expect(screen.getByTestId('running-agent-log')).toBeInTheDocument()
    })

    // Initially expanded
    expect(screen.getByText('expanded')).toBeInTheDocument()

    // Toggle to collapsed
    await user.click(screen.getByTestId('toggle-collapse'))

    expect(screen.getByText('collapsed')).toBeInTheDocument()
  })

  it('does not show RunningAgentLog on description tab', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithActivePhase)

    renderPage()
    const user = await goToWorkflowTab()

    await waitFor(() => {
      expect(screen.getByTestId('running-agent-log')).toBeInTheDocument()
    })

    // Switch to description tab
    await user.click(screen.getByText('Description'))

    // Log panel should not be visible (it's only in workflow tab)
    expect(screen.queryByTestId('running-agent-log')).not.toBeInTheDocument()
  })
})
