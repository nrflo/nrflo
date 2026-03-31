import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as ticketsApi from '@/api/tickets'
import {
  sampleTicket,
  workflowWithActivePhase,
  workflowMultiple,
  emptySessions,
  renderPage,
} from './TicketDetailPage.test-utils'
import type { AgentSession, AgentSessionsResponse } from '@/types/workflow'

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

// Mock AgentLogPanel to render session IDs so we can assert which sessions are visible
vi.mock('@/components/workflow/AgentLogPanel', () => ({
  AgentLogPanel: ({
    activeAgents,
    sessions,
    selectedAgent,
  }: {
    activeAgents: Record<string, { agent_type: string; result?: string }>
    sessions: AgentSession[]
    selectedAgent: { phaseName: string } | null
  }) => {
    const running = Object.values(activeAgents).filter(a => !a.result)
    if (running.length === 0 && !selectedAgent) return null
    return (
      <div data-testid="agent-log-panel">
        {sessions.map(s => (
          <span key={s.id} data-testid={`session-${s.id}`}>{s.id}</span>
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

function makeSession(overrides: Partial<AgentSession> & { id: string; workflow_instance_id: string }): AgentSession {
  return {
    project_id: 'test-project',
    ticket_id: 'TICKET-1',
    phase: 'implementation',
    workflow: 'feature',
    agent_type: 'implementor',
    model_id: 'claude-sonnet-4-5',
    status: 'completed',
    message_count: 5,
    restart_count: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

async function goToWorkflowTab() {
  const user = userEvent.setup()
  await waitFor(() => {
    expect(screen.getByText('Test ticket')).toBeInTheDocument()
  })
  await user.click(screen.getByText('Workflow'))
  return user
}

describe('TicketDetailPage - multi-instance: session filtering', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows only sessions for the default selected instance', async () => {
    const sessions: AgentSessionsResponse = {
      ticket_id: 'TICKET-1',
      sessions: [
        makeSession({ id: 'sess-inst1', workflow_instance_id: 'inst-multi-01' }),
        makeSession({ id: 'sess-inst2', workflow_instance_id: 'inst-multi-02' }),
      ],
    }
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowMultiple)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(sessions)

    renderPage()
    await goToWorkflowTab()

    await waitFor(() => {
      expect(screen.getByTestId('agent-log-panel')).toBeInTheDocument()
    })

    // Only the session for the first (selected) instance should be shown
    expect(screen.getByTestId('session-sess-inst1')).toBeInTheDocument()
    expect(screen.queryByTestId('session-sess-inst2')).not.toBeInTheDocument()
  })
})

describe('TicketDetailPage - multi-instance: instance switching', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(emptySessions)
  })

  it('selecting a different instance updates the displayed workflow state', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowMultiple)

    renderPage()
    const user = await goToWorkflowTab()

    // Default: inst-multi-01 (feature) has active phase → Stop button visible
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /stop/i })).toBeInTheDocument()
    })

    // Click InstanceList chip for second instance (bugfix, no active phase)
    await user.click(screen.getByText('bugfix (#inst-mul)'))

    // Stop button should disappear — inst-multi-02 has no active phase and is not orchestrated
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /stop/i })).not.toBeInTheDocument()
    })
  })

  it('default selected instance is the first key in all_workflows', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowMultiple)

    renderPage()
    await goToWorkflowTab()

    // InstanceList chip shows label for first instance (inst-multi-01 = feature)
    await waitFor(() => {
      expect(screen.getByText('feature (#inst-mul)')).toBeInTheDocument()
    })
  })
})

describe('TicketDetailPage - single-instance: badge instead of dropdown', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(emptySessions)
  })

  it('shows workflow name as badge without a dropdown selector', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithActivePhase)

    renderPage()
    await goToWorkflowTab()

    await waitFor(() => {
      expect(screen.getByText('feature')).toBeInTheDocument()
    })

    // No "(#...)" short-id label — that format only appears in multi-instance dropdown
    expect(screen.queryByText(/feature.*#/)).not.toBeInTheDocument()
  })
})
