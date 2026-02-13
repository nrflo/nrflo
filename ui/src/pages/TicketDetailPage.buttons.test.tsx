import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as ticketsApi from '@/api/tickets'
import * as workflowsApi from '@/api/workflows'
import {
  sampleTicket,
  workflowWithActivePhase,
  workflowNoActivePhase,
  workflowOrchestrated,
  workflowOrchestratedNoAgents,
  workflowMultiple,
  emptySessions,
  renderPage,
} from './TicketDetailPage.test-utils'

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
  AgentLogPanel: ({
    activeAgents,
    selectedAgent,
  }: {
    activeAgents: Record<string, { agent_type: string; phase?: string; result?: string }>
    selectedAgent: { phaseName: string } | null
  }) => {
    const running = Object.values(activeAgents).filter(a => !a.result)
    if (running.length === 0 && !selectedAgent) return null
    return (
      <div data-testid="running-agent-log">
        {running.map((agent, i) => (
          <span key={i}>{agent.agent_type}</span>
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
}))

describe('TicketDetailPage - Stop button placement', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(emptySessions)
  })

  it('shows Stop button when workflow has an active phase', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithActivePhase)

    renderPage()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /stop/i })).toBeInTheDocument()
    })
  })

  it('shows Stop button when workflow is orchestrated', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowOrchestrated)

    renderPage()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /stop/i })).toBeInTheDocument()
    })
  })

  it('shows Stop button when orchestrated but no active agents yet', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowOrchestratedNoAgents)

    renderPage()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /stop/i })).toBeInTheDocument()
    })
  })

  it('does not show Stop button when no active phase and not orchestrated', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowNoActivePhase)

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Test ticket')).toBeInTheDocument()
    })

    expect(screen.queryByRole('button', { name: /stop/i })).not.toBeInTheDocument()
  })

  it('shows Run Workflow button when no active workflow', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowNoActivePhase)

    renderPage()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /run workflow/i })).toBeInTheDocument()
    })
  })

  it('does not show Run Workflow button when workflow has active phase', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithActivePhase)

    renderPage()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /stop/i })).toBeInTheDocument()
    })

    expect(screen.queryByRole('button', { name: /run workflow/i })).not.toBeInTheDocument()
  })

  it('does not show Run Workflow button when workflow is orchestrated', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowOrchestrated)

    renderPage()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /stop/i })).toBeInTheDocument()
    })

    expect(screen.queryByRole('button', { name: /run workflow/i })).not.toBeInTheDocument()
  })

  it('Stop button is placed next to workflow badge (left side)', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithActivePhase)

    renderPage()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /stop/i })).toBeInTheDocument()
    })

    // The Stop button should be in the same container as the workflow badge
    const workflowBadge = screen.getByText('feature')
    const stopButton = screen.getByRole('button', { name: /stop/i })
    // Both should share the same parent (the left-side flex container)
    expect(workflowBadge.closest('.flex.items-center.gap-3'))
      .toBe(stopButton.closest('.flex.items-center.gap-3'))
  })

  it('Stop button is placed next to Auto badge when orchestrated', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowOrchestrated)

    renderPage()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /stop/i })).toBeInTheDocument()
    })

    // Auto badge and Stop button should be in the same container
    const autoBadge = screen.getByText('Auto')
    const stopButton = screen.getByRole('button', { name: /stop/i })
    expect(autoBadge.closest('.flex.items-center.gap-3'))
      .toBe(stopButton.closest('.flex.items-center.gap-3'))
  })

  it('Stop button does not share container with RunningAgentLog toggle', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithActivePhase)

    renderPage()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /stop/i })).toBeInTheDocument()
      expect(screen.getByTestId('running-agent-log')).toBeInTheDocument()
    })

    // Stop button should NOT be inside the RunningAgentLog component
    const logPanel = screen.getByTestId('running-agent-log')
    expect(within(logPanel).queryByRole('button', { name: /stop/i })).not.toBeInTheDocument()
  })

  it('Stop button calls stopWorkflow with correct params', async () => {
    const user = userEvent.setup()
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithActivePhase)
    vi.mocked(workflowsApi.stopWorkflow).mockResolvedValue({ status: 'stopped' })

    renderPage()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /stop/i })).toBeInTheDocument()
    })

    await user.click(screen.getByRole('button', { name: /stop/i }))

    await waitFor(() => {
      expect(workflowsApi.stopWorkflow).toHaveBeenCalledWith(
        'TICKET-1',
        { workflow: 'feature' }
      )
    })
  })

  it('shows Stop button with multiple workflows and active phase', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowMultiple)

    renderPage()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /stop/i })).toBeInTheDocument()
    })

    // Should show workflow selector dropdown, not badge
    expect(screen.getByRole('combobox')).toBeInTheDocument()
    // Stop button and dropdown should be in the same left-side container
    const dropdown = screen.getByRole('combobox')
    const stopButton = screen.getByRole('button', { name: /stop/i })
    expect(dropdown.closest('.flex.items-center.gap-3'))
      .toBe(stopButton.closest('.flex.items-center.gap-3'))
  })
})
