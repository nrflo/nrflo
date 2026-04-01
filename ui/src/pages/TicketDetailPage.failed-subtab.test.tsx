import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as ticketsApi from '@/api/tickets'
import {
  sampleTicket,
  workflowFailed,
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
  CompletedAgentsTable: () => <div data-testid="completed-agents-table" />,
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

// One running instance + one failed instance
const workflowMixedWithFailed: WorkflowResponse = {
  ticket_id: 'TICKET-1',
  has_workflow: true,
  state: {
    workflow: 'feature',
    instance_id: 'inst-running-01',
    version: 4,
    current_phase: 'implementation',
    phase_order: ['investigation', 'implementation'],
    phases: { investigation: { status: 'completed', result: 'pass' } },
    active_agents: {},
  },
  workflows: ['feature', 'bugfix'],
  all_workflows: {
    'inst-running-01': {
      workflow: 'feature',
      instance_id: 'inst-running-01',
      version: 4,
      current_phase: 'implementation',
      phase_order: ['investigation', 'implementation'],
      phases: { investigation: { status: 'completed', result: 'pass' } },
      active_agents: {},
    },
    'inst-failed-01': {
      workflow: 'bugfix',
      instance_id: 'inst-failed-01',
      version: 4,
      status: 'failed',
      current_phase: 'implementation',
      phase_order: ['investigation', 'implementation'],
      phases: {
        investigation: { status: 'completed', result: 'pass' },
        implementation: { status: 'completed', result: 'fail' },
      },
      active_agents: {},
    },
  },
}

async function goToWorkflowTab() {
  const user = userEvent.setup()
  await waitFor(() => expect(screen.getByText('Test ticket')).toBeInTheDocument())
  await user.click(screen.getByText('Workflow'))
  return user
}

describe('TicketDetailPage - Failed sub-tab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(emptySessions)
  })

  it('does not show sub-tabs for a single failed instance (singleNonRunningOnly)', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowFailed)

    renderPage()
    await goToWorkflowTab()

    await waitFor(() => expect(screen.getByTestId('phase-timeline')).toBeInTheDocument())
    expect(screen.queryByText(/Running \(\d+\)/)).not.toBeInTheDocument()
    expect(screen.queryByText(/Failed \(\d+\)/)).not.toBeInTheDocument()
    expect(screen.queryByText(/Completed \(\d+\)/)).not.toBeInTheDocument()
  })

  it('shows Failed (1) and Running (1) counts when one instance is failed', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowMixedWithFailed)

    renderPage()
    await goToWorkflowTab()

    await waitFor(() => {
      expect(screen.getByText('Running (1)')).toBeInTheDocument()
      expect(screen.getByText('Failed (1)')).toBeInTheDocument()
      expect(screen.getByText('Completed (0)')).toBeInTheDocument()
    })
  })

  it('clicking Failed tab shows phase-timeline for failed instance', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowMixedWithFailed)

    renderPage()
    const user = await goToWorkflowTab()

    await waitFor(() => expect(screen.getByText('Failed (1)')).toBeInTheDocument())

    await user.click(screen.getByText('Failed (1)'))

    await waitFor(() => expect(screen.getByTestId('phase-timeline')).toBeInTheDocument())
    expect(screen.queryByTestId('completed-agents-table')).not.toBeInTheDocument()
  })

  it('Running tab only shows running instance (failed instance excluded from Running count)', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowMixedWithFailed)

    renderPage()
    await goToWorkflowTab()

    // Running tab shows count 1, not 2 (failed instance is not counted as running)
    await waitFor(() => expect(screen.getByText('Running (1)')).toBeInTheDocument())
    expect(screen.queryByText('Running (2)')).not.toBeInTheDocument()
  })
})
