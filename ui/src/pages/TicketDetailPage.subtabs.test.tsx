import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as ticketsApi from '@/api/tickets'
import {
  sampleTicket,
  workflowWithActivePhase,
  workflowCompleted,
  workflowMultiple,
  emptySessions,
  renderPage,
} from './TicketDetailPage.test-utils'
import type { WorkflowResponse } from '@/types/workflow'

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
  AgentLogPanel: () => null,
}))

vi.mock('@/components/workflow/CompletedAgentsTable', () => ({
  CompletedAgentsTable: ({ agentHistory }: { agentHistory: Array<{ agent_type: string }> }) => (
    <div data-testid="completed-agents-table">
      {agentHistory.map((a, i) => (
        <div key={i} data-testid={`completed-agent-${a.agent_type}`}>{a.agent_type}</div>
      ))}
    </div>
  ),
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

// Fixture: one running + one completed instance → shows sub-tabs
const runningInstanceState = {
  workflow: 'feature',
  instance_id: 'inst-running-01',
  version: 4,
  current_phase: 'implementation',
  phase_order: ['investigation', 'implementation'],
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
      started_at: '2026-01-01T00:00:00Z',
    },
  },
}

const completedInstanceState = {
  workflow: 'bugfix',
  instance_id: 'inst-compl-01',
  version: 4,
  status: 'completed' as const,
  completed_at: '2026-01-01T02:00:00Z',
  total_duration_sec: 1800,
  total_tokens_used: 100000,
  current_phase: 'investigation',
  phase_order: ['investigation'],
  phases: {
    investigation: { status: 'completed' as const, result: 'pass' as const },
  },
  active_agents: {},
  agent_history: [
    {
      agent_id: 'b1',
      agent_type: 'setup-analyzer',
      phase: 'investigation',
      model_id: 'claude-sonnet-4-5',
      result: 'pass',
      started_at: '2026-01-01T01:00:00Z',
      ended_at: '2026-01-01T02:00:00Z',
      session_id: 'sess-compl-01',
    },
  ],
}

const workflowMixed: WorkflowResponse = {
  ticket_id: 'TICKET-1',
  has_workflow: true,
  state: runningInstanceState,
  workflows: ['feature', 'bugfix'],
  all_workflows: {
    'inst-running-01': runningInstanceState,
    'inst-compl-01': completedInstanceState,
  },
}

async function goToWorkflowTab() {
  const user = userEvent.setup()
  await waitFor(() => {
    expect(screen.getByText('Test ticket')).toBeInTheDocument()
  })
  await user.click(screen.getByText('Workflow'))
  return user
}

describe('TicketDetailPage - Running/Completed sub-tabs visibility', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(emptySessions)
  })

  it('does not show sub-tabs for a single running instance', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithActivePhase)

    renderPage()
    await goToWorkflowTab()

    await waitFor(() => {
      expect(screen.getByTestId('phase-timeline')).toBeInTheDocument()
    })
    expect(screen.queryByText(/Running \(\d+\)/)).not.toBeInTheDocument()
    expect(screen.queryByText(/Failed \(\d+\)/)).not.toBeInTheDocument()
    expect(screen.queryByText(/Completed \(\d+\)/)).not.toBeInTheDocument()
  })

  it('does not show sub-tabs for a single completed instance (singleCompletedOnly optimization)', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowCompleted)

    renderPage()
    await goToWorkflowTab()

    await waitFor(() => {
      expect(screen.getByTestId('phase-timeline')).toBeInTheDocument()
    })
    expect(screen.queryByText(/Running \(\d+\)/)).not.toBeInTheDocument()
    expect(screen.queryByText(/Failed \(\d+\)/)).not.toBeInTheDocument()
    expect(screen.queryByText(/Completed \(\d+\)/)).not.toBeInTheDocument()
  })

  it('shows Running/Failed/Completed sub-tabs for mixed running+completed instances', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowMixed)

    renderPage()
    await goToWorkflowTab()

    await waitFor(() => {
      expect(screen.getByText('Running (1)')).toBeInTheDocument()
      expect(screen.getByText('Failed (0)')).toBeInTheDocument()
      expect(screen.getByText('Completed (1)')).toBeInTheDocument()
    })
  })

  it('shows Running/Failed/Completed sub-tabs for multiple running instances', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowMultiple)

    renderPage()
    await goToWorkflowTab()

    await waitFor(() => {
      expect(screen.getByText('Running (2)')).toBeInTheDocument()
      expect(screen.getByText('Failed (0)')).toBeInTheDocument()
      expect(screen.getByText('Completed (0)')).toBeInTheDocument()
    })
  })
})

describe('TicketDetailPage - Sub-tab switching', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(emptySessions)
  })

  it('defaults to Running sub-tab showing WorkflowTabContent (not CompletedAgentsTable)', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowMixed)

    renderPage()
    await goToWorkflowTab()

    await waitFor(() => {
      expect(screen.getByTestId('phase-timeline')).toBeInTheDocument()
    })
    expect(screen.queryByTestId('completed-agents-table')).not.toBeInTheDocument()
  })

  it('clicking Completed tab shows CompletedAgentsTable and hides WorkflowTabContent', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowMixed)

    renderPage()
    const user = await goToWorkflowTab()

    await waitFor(() => {
      expect(screen.getByText('Completed (1)')).toBeInTheDocument()
    })

    await user.click(screen.getByText('Completed (1)'))

    await waitFor(() => {
      expect(screen.getByTestId('completed-agents-table')).toBeInTheDocument()
    })
    expect(screen.queryByTestId('phase-timeline')).not.toBeInTheDocument()
  })

  it('CompletedAgentsTable receives agents from completed instances', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowMixed)

    renderPage()
    const user = await goToWorkflowTab()

    await waitFor(() => {
      expect(screen.getByText('Completed (1)')).toBeInTheDocument()
    })

    await user.click(screen.getByText('Completed (1)'))

    await waitFor(() => {
      expect(screen.getByTestId('completed-agent-setup-analyzer')).toBeInTheDocument()
    })
  })

  it('clicking Running tab after Completed restores WorkflowTabContent', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowMixed)

    renderPage()
    const user = await goToWorkflowTab()

    await waitFor(() => {
      expect(screen.getByText('Completed (1)')).toBeInTheDocument()
    })

    await user.click(screen.getByText('Completed (1)'))
    await waitFor(() => {
      expect(screen.getByTestId('completed-agents-table')).toBeInTheDocument()
    })

    await user.click(screen.getByText('Running (1)'))
    await waitFor(() => {
      expect(screen.getByTestId('phase-timeline')).toBeInTheDocument()
    })
    expect(screen.queryByTestId('completed-agents-table')).not.toBeInTheDocument()
  })
})
