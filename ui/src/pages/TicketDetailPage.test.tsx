import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TicketDetailPage } from './TicketDetailPage'
import * as ticketsApi from '@/api/tickets'
import * as workflowsApi from '@/api/workflows'
import type { TicketWithDeps } from '@/types/ticket'
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

// Mock PhaseTimeline to avoid deep dependency tree
vi.mock('@/components/workflow/PhaseTimeline', () => ({
  PhaseTimeline: () => <div data-testid="phase-timeline">PhaseTimeline</div>,
}))

// Mock RunWorkflowDialog to avoid deep dependency tree
vi.mock('@/components/workflow/RunWorkflowDialog', () => ({
  RunWorkflowDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="run-dialog">RunWorkflowDialog</div> : null,
}))

// Mock AgentMessagesModal
vi.mock('@/components/workflow/PhaseGraph/AgentMessagesModal', () => ({
  AgentMessagesModal: ({ open, phaseName }: { open: boolean; phaseName: string }) =>
    open ? <div data-testid="agent-messages-modal">Modal: {phaseName}</div> : null,
}))

// Mock RunningAgentLog to test integration without inner hook dependencies
vi.mock('@/components/workflow/RunningAgentLog', () => ({
  RunningAgentLog: ({
    activeAgents,
    collapsed,
    onToggleCollapse,
    onAgentClick,
  }: {
    activeAgents: Record<string, { agent_type: string; phase?: string; result?: string }>
    collapsed: boolean
    onToggleCollapse: () => void
    onAgentClick: (agent: { agent_type: string; phase?: string }, session?: unknown) => void
  }) => {
    const running = Object.values(activeAgents).filter(a => !a.result)
    if (running.length === 0) return null
    return (
      <div data-testid="running-agent-log">
        <span>{collapsed ? 'collapsed' : 'expanded'}</span>
        <button data-testid="toggle-collapse" onClick={onToggleCollapse}>Toggle</button>
        {running.map((agent, i) => (
          <button
            key={i}
            data-testid={`agent-row-${agent.agent_type}`}
            onClick={() => onAgentClick(agent)}
          >
            {agent.agent_type}
          </button>
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

const sampleTicket: TicketWithDeps = {
  id: 'TICKET-1',
  title: 'Test ticket',
  description: 'A test ticket',
  status: 'in_progress',
  priority: 2,
  issue_type: 'feature',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  closed_at: null,
  created_by: 'ui',
  close_reason: null,
  blockers: [],
  blocks: [],
}

// Workflow with an active phase (agents running)
const workflowWithActivePhase: WorkflowResponse = {
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
        started_at: '2026-01-01T00:00:00Z',
      },
    },
  },
  workflows: ['feature'],
  all_workflows: {},
}

// Workflow with no active phase
const workflowNoActivePhase: WorkflowResponse = {
  ticket_id: 'TICKET-1',
  has_workflow: true,
  state: {
    workflow: 'feature',
    version: 4,
    current_phase: 'implementation',
    phase_order: ['investigation', 'implementation'],
    phases: {
      investigation: { status: 'completed', result: 'pass' },
      implementation: { status: 'completed', result: 'pass' },
    },
    active_agents: {},
  },
  workflows: ['feature'],
  all_workflows: {},
}

// Orchestrated workflow (running via Auto) with active phase
const workflowOrchestrated: WorkflowResponse = {
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
        started_at: '2026-01-01T00:00:00Z',
      },
    },
    findings: {
      _orchestration: { status: 'running' },
    },
  },
  workflows: ['feature'],
  all_workflows: {},
}

// Orchestrated workflow with no active agents yet (between phases)
const workflowOrchestratedNoAgents: WorkflowResponse = {
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
    },
    active_agents: {},
    findings: {
      _orchestration: { status: 'running' },
    },
  },
  workflows: ['feature'],
  all_workflows: {},
}

// Workflow with multiple workflows (dropdown selector visible)
const workflowMultiple: WorkflowResponse = {
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
        started_at: '2026-01-01T00:00:00Z',
      },
    },
  },
  workflows: ['feature', 'bugfix'],
  all_workflows: {
    feature: {
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
          started_at: '2026-01-01T00:00:00Z',
        },
      },
    },
    bugfix: {
      workflow: 'bugfix',
      version: 4,
      current_phase: 'investigation',
      phase_order: ['investigation', 'implementation'],
      phases: {},
      active_agents: {},
    },
  },
}

const emptySessions: AgentSessionsResponse = {
  ticket_id: 'TICKET-1',
  sessions: [],
}

function renderPage(ticketId = 'TICKET-1') {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[`/tickets/${encodeURIComponent(ticketId)}`]}>
        <Routes>
          <Route path="/tickets/:id" element={<TicketDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
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

    await waitFor(() => {
      expect(screen.getByTestId('running-agent-log')).toBeInTheDocument()
    })
  })

  it('does not show RunningAgentLog when no active phase', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowNoActivePhase)

    renderPage()

    // Wait for ticket to load
    await waitFor(() => {
      expect(screen.getByText('Test ticket')).toBeInTheDocument()
    })

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

    await waitFor(() => {
      expect(screen.getByText('Test ticket')).toBeInTheDocument()
    })

    expect(screen.queryByTestId('running-agent-log')).not.toBeInTheDocument()
  })

  it('opens AgentMessagesModal when agent in log is clicked', async () => {
    const user = userEvent.setup()
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithActivePhase)

    renderPage()

    await waitFor(() => {
      expect(screen.getByTestId('running-agent-log')).toBeInTheDocument()
    })

    await user.click(screen.getByTestId('agent-row-implementor'))

    await waitFor(() => {
      expect(screen.getByTestId('agent-messages-modal')).toBeInTheDocument()
    })
    expect(screen.getByText(/Modal: implementation/)).toBeInTheDocument()
  })

  it('toggles log panel collapse state', async () => {
    const user = userEvent.setup()
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithActivePhase)

    renderPage()

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
    const user = userEvent.setup()
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowWithActivePhase)

    renderPage()

    await waitFor(() => {
      expect(screen.getByTestId('running-agent-log')).toBeInTheDocument()
    })

    // Switch to description tab
    await user.click(screen.getByText('Description'))

    // Log panel should not be visible (it's only in workflow tab)
    expect(screen.queryByTestId('running-agent-log')).not.toBeInTheDocument()
  })
})

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
