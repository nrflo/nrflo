import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AgentLogPanel } from './AgentLogPanel'
import type { ActiveAgentV4, AgentSession } from '@/types/workflow'
import type { SelectedAgentData } from './PhaseGraph/types'

// Mock useSessionMessages (used internally by AgentMessagesBlock)
vi.mock('@/hooks/useTickets', () => ({
  useSessionMessages: () => ({
    data: {
      session_id: 'session-1',
      messages: [
        { content: 'Building project...', created_at: '2026-01-01T00:01:00Z' },
        { content: 'Running tests...', created_at: '2026-01-01T00:02:00Z' },
      ],
      total: 2,
    },
    isLoading: false,
  }),
}))

// Mock AgentLogDetail to test routing without deep dependencies
vi.mock('./AgentLogDetail', () => ({
  AgentLogDetail: ({
    selectedAgent,
    onBack,
  }: {
    selectedAgent: SelectedAgentData
    onBack: () => void
  }) => (
    <div data-testid="agent-log-detail">
      <span data-testid="detail-phase">{selectedAgent.phaseName}</span>
      <span data-testid="detail-agent-type">{selectedAgent.agent?.agent_type || selectedAgent.historyEntry?.agent_type}</span>
      <button data-testid="back-button" onClick={onBack}>Back</button>
    </div>
  ),
}))

function makeAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_id: 'a1',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude-sonnet-4-5',
    cli: 'claude',
    pid: 12345,
    started_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeSession(overrides: Partial<AgentSession> = {}): AgentSession {
  return {
    id: 'session-1',
    project_id: 'test-project',
    ticket_id: 'TICKET-1',
    workflow_instance_id: 'wi-1',
    phase: 'implementation',
    workflow: 'feature',
    agent_type: 'implementor',
    model_id: 'claude-sonnet-4-5',
    status: 'running',
    message_count: 5,
    raw_output_size: 1024,
    restart_count: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderPanel(props: Partial<React.ComponentProps<typeof AgentLogPanel>> = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const defaultProps = {
    activeAgents: {} as Record<string, ActiveAgentV4>,
    sessions: [] as AgentSession[],
    collapsed: false,
    onToggleCollapse: vi.fn(),
    selectedAgent: null as SelectedAgentData | null,
    onAgentSelect: vi.fn(),
    ...props,
  }
  return {
    ...render(
      <QueryClientProvider client={queryClient}>
        <AgentLogPanel {...defaultProps} />
      </QueryClientProvider>
    ),
    props: defaultProps,
  }
}

describe('AgentLogPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('overview mode', () => {
    it('renders nothing when no running agents and no selected agent', () => {
      const { container } = renderPanel({ activeAgents: {} })
      expect(container.innerHTML).toBe('')
    })

    it('shows running agents count in header', () => {
      const agents = {
        'implementor:claude:sonnet': makeAgent(),
        'tester:claude:sonnet': makeAgent({
          agent_id: 'a2',
          agent_type: 'tester',
          phase: 'verification',
        }),
      }
      renderPanel({ activeAgents: agents, sessions: [makeSession()] })
      expect(screen.getByText('Running Agents (2)')).toBeInTheDocument()
    })

    it('shows running agent count badge when collapsed', () => {
      const agents = { 'implementor:claude:sonnet': makeAgent() }
      renderPanel({ activeAgents: agents, collapsed: true, sessions: [makeSession()] })
      expect(screen.getByText('1')).toBeInTheDocument()
      expect(screen.getByText('Agent Log')).toBeInTheDocument()
    })

    it('excludes completed agents from running list', () => {
      const agents = {
        'implementor:claude:sonnet': makeAgent({ result: 'pass' }),
      }
      const { container } = renderPanel({ activeAgents: agents })
      // Completed agent with result means no running agents, no selected => renders nothing
      expect(container.innerHTML).toBe('')
    })

    it('clicking a running agent transitions to detail mode', async () => {
      const user = userEvent.setup()
      const agent = makeAgent()
      const session = makeSession()
      const onAgentSelect = vi.fn()

      renderPanel({
        activeAgents: { 'implementor:claude:sonnet': agent },
        sessions: [session],
        onAgentSelect,
      })

      // Click the agent row (it's a button with agent info)
      const agentButton = screen.getByRole('button', { name: /implementation/i })
      await user.click(agentButton)

      expect(onAgentSelect).toHaveBeenCalledWith({
        phaseName: 'implementation',
        agent,
        session,
      })
    })
  })

  describe('detail mode', () => {
    it('renders AgentLogDetail when selectedAgent is set', () => {
      const agent = makeAgent()
      const session = makeSession()

      renderPanel({
        selectedAgent: { phaseName: 'implementation', agent, session },
        sessions: [session],
      })

      expect(screen.getByTestId('agent-log-detail')).toBeInTheDocument()
      expect(screen.getByTestId('detail-phase')).toHaveTextContent('implementation')
      expect(screen.getByTestId('detail-agent-type')).toHaveTextContent('implementor')
    })

    it('shows panel even when no running agents if selectedAgent is set', () => {
      renderPanel({
        activeAgents: {},
        selectedAgent: {
          phaseName: 'investigation',
          historyEntry: {
            agent_id: 'h1',
            agent_type: 'setup-analyzer',
            phase: 'investigation',
            result: 'pass',
            duration_sec: 120,
          },
          session: makeSession({
            id: 'session-2',
            phase: 'investigation',
            agent_type: 'setup-analyzer',
            status: 'completed',
          }),
        },
      })

      expect(screen.getByTestId('agent-log-detail')).toBeInTheDocument()
      expect(screen.getByTestId('detail-agent-type')).toHaveTextContent('setup-analyzer')
    })

    it('back button returns to overview mode (calls onAgentSelect with null)', async () => {
      const user = userEvent.setup()
      const onAgentSelect = vi.fn()

      renderPanel({
        selectedAgent: { phaseName: 'implementation', agent: makeAgent() },
        onAgentSelect,
        sessions: [makeSession()],
      })

      await user.click(screen.getByTestId('back-button'))
      expect(onAgentSelect).toHaveBeenCalledWith(null)
    })

    it('shows collapsed state in detail mode', () => {
      renderPanel({
        selectedAgent: { phaseName: 'implementation', agent: makeAgent() },
        collapsed: true,
        sessions: [makeSession()],
      })

      expect(screen.getByText('Agent Log')).toBeInTheDocument()
      expect(screen.queryByTestId('agent-log-detail')).not.toBeInTheDocument()
    })

    it('toggle collapse button calls onToggleCollapse', async () => {
      const user = userEvent.setup()
      const onToggleCollapse = vi.fn()

      renderPanel({
        selectedAgent: { phaseName: 'implementation', agent: makeAgent() },
        onToggleCollapse,
        sessions: [makeSession()],
      })

      const toggleButton = screen.getByTitle('Collapse agent log')
      await user.click(toggleButton)
      expect(onToggleCollapse).toHaveBeenCalledTimes(1)
    })
  })

  describe('session lookup', () => {
    it('matches session by agent_type, phase, and model_id', async () => {
      const user = userEvent.setup()
      const agent = makeAgent({
        agent_type: 'tester',
        phase: 'verification',
        model_id: 'claude-opus-4-6',
      })
      const correctSession = makeSession({
        id: 'correct-session',
        agent_type: 'tester',
        phase: 'verification',
        model_id: 'claude-opus-4-6',
      })
      const wrongSession = makeSession({
        id: 'wrong-session',
        agent_type: 'tester',
        phase: 'verification',
        model_id: 'claude-sonnet-4-5',
      })
      const onAgentSelect = vi.fn()

      renderPanel({
        activeAgents: { 'tester:claude:opus': agent },
        sessions: [wrongSession, correctSession],
        onAgentSelect,
      })

      const agentButton = screen.getByRole('button', { name: /verification/i })
      await user.click(agentButton)

      expect(onAgentSelect).toHaveBeenCalledWith(
        expect.objectContaining({ session: correctSession })
      )
    })
  })
})
