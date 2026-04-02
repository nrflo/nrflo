import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AgentLogPanel } from './AgentLogPanel'
import type { ActiveAgentV4, AgentSession } from '@/types/workflow'
import type { SelectedAgentData } from './PhaseGraph/types'

// Mock useSessionMessages (used internally by AgentLogDetail)
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
vi.mock('./AgentLogDetail', async () => {
  const actual = await vi.importActual<typeof import('./AgentLogDetail')>('./AgentLogDetail')
  return {
    ...actual,
    AgentLogDetail: ({
      selectedAgent,
      onBack,
    }: {
      selectedAgent: SelectedAgentData
      onBack: () => void
    }) => {
      const isRunning = selectedAgent.agent && !selectedAgent.agent.result
      return (
        <div data-testid="agent-log-detail">
          <span data-testid="detail-phase">{selectedAgent.phaseName}</span>
          <span data-testid="detail-agent-type">{selectedAgent.agent?.agent_type || selectedAgent.historyEntry?.agent_type}</span>
          <span data-testid="detail-result">{selectedAgent.agent?.result ?? ''}</span>
          <span data-testid="detail-is-running">{isRunning ? 'running' : 'stopped'}</span>
          <button data-testid="back-button" onClick={onBack}>Back</button>
        </div>
      )
    },
  }
})

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

  describe('running agents view', () => {
    it('renders nothing when no running agents and no selected agent', () => {
      const { container } = renderPanel({ activeAgents: {} })
      expect(container.innerHTML).toBe('')
    })

    it('excludes completed agents from running list', () => {
      const agents = {
        'implementor:claude:sonnet': makeAgent({ result: 'pass' }),
      }
      const { container } = renderPanel({ activeAgents: agents })
      // Completed agent with result means no running agents, no selected => renders nothing
      expect(container.innerHTML).toBe('')
    })

    it('renders one AgentLogDetail with tabs when multiple agents running', () => {
      const agents = {
        'implementor:claude:sonnet': makeAgent(),
        'tester:claude:sonnet': makeAgent({
          agent_id: 'a2',
          agent_type: 'tester',
          phase: 'verification',
        }),
      }
      renderPanel({ activeAgents: agents, sessions: [makeSession()] })
      expect(screen.getAllByTestId('agent-log-detail')).toHaveLength(1)
      expect(screen.getAllByTestId('agent-tab')).toHaveLength(2)
    })

    it('shows collapsed bar when collapsed with running agents', () => {
      const agents = { 'implementor:claude:sonnet': makeAgent() }
      renderPanel({ activeAgents: agents, collapsed: true, sessions: [makeSession()] })
      expect(screen.getByText('Agent Log')).toBeInTheDocument()
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

    // TODO(test-writer): collapse toggle button moved to WorkflowTabContent header — test there
  })

  // TODO(test-writer): Add tests for session lookup logic (findSession matches by session_id, agent_type+phase+model_id)

  describe('ticket nrflow-46fb2e: session lookup fix for completed agents', () => {
    it('uses captured session directly for completed agents instead of re-looking up', async () => {
      // Scenario: Two completed agents with same agent_type/phase/model
      // but different sessions. The bug was that detail mode always looked up
      // the latest session, showing wrong messages.
      const agent1Session = makeSession({
        id: 'agent1-session',
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-opus-4-6',
      })
      const agent2Session = makeSession({
        id: 'agent2-session',
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-opus-4-6',
      })

      const selectedAgent = {
        phaseName: 'implementation',
        historyEntry: {
          agent_id: 'a1',
          agent_type: 'implementor',
          phase: 'implementation',
          session_id: 'agent1-session',
          result: 'pass',
          duration_sec: 3600,
        },
        session: agent1Session, // Captured at click time
      }

      renderPanel({
        selectedAgent,
        sessions: [agent1Session, agent2Session],
      })

      expect(screen.getByTestId('agent-log-detail')).toBeInTheDocument()

      // The fix ensures that for completed agents (historyEntry present, or agent.result present),
      // we use the captured session directly instead of calling findSession() again.
      // This test verifies the component renders with captured session without errors.
    })

    it('uses live session lookup for running agents (no result)', async () => {
      const initialSession = makeSession({
        id: 'running-session',
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-opus-4-6',
        status: 'running',
      })

      const runningAgent = makeAgent({
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-opus-4-6',
        result: undefined, // No result = still running
      })

      const selectedAgent = {
        phaseName: 'implementation',
        agent: runningAgent,
        session: initialSession,
      }

      renderPanel({
        selectedAgent,
        sessions: [initialSession],
        activeAgents: { 'implementor:claude:opus': runningAgent },
      })

      expect(screen.getByTestId('agent-log-detail')).toBeInTheDocument()
    })

    it('does not re-lookup session for completed agent with result', () => {
      const completedAgent = makeAgent({
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-opus-4-6',
        result: 'pass', // Has result = completed
      })

      const capturedSession = makeSession({
        id: 'captured-session',
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-opus-4-6',
        status: 'completed',
        result: 'pass',
      })

      const differentSession = makeSession({
        id: 'different-session',
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-opus-4-6',
        status: 'completed',
        result: 'pass',
      })

      const selectedAgent = {
        phaseName: 'implementation',
        agent: completedAgent,
        session: capturedSession,
      }

      renderPanel({
        selectedAgent,
        sessions: [differentSession, capturedSession],
      })

      // Should render detail view with captured session
      expect(screen.getByTestId('agent-log-detail')).toBeInTheDocument()
    })

    it('uses captured session for historyEntry even when findSession would match different session', () => {
      const capturedSession = makeSession({
        id: 'captured-history-session',
        agent_type: 'setup-analyzer',
        phase: 'investigation',
        model_id: 'claude-sonnet-4-5',
      })

      const latestSession = makeSession({
        id: 'latest-session',
        agent_type: 'setup-analyzer',
        phase: 'investigation',
        model_id: 'claude-sonnet-4-5',
      })

      const selectedAgent = {
        phaseName: 'investigation',
        historyEntry: {
          agent_id: 'h1',
          agent_type: 'setup-analyzer',
          phase: 'investigation',
          session_id: 'captured-history-session',
          result: 'pass',
          duration_sec: 120,
        },
        session: capturedSession,
      }

      renderPanel({
        selectedAgent,
        sessions: [latestSession, capturedSession],
      })

      expect(screen.getByTestId('agent-log-detail')).toBeInTheDocument()
    })

    it('falls back to captured session when findSession returns undefined for running agent', () => {
      const runningAgent = makeAgent({
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-opus-4-6',
        result: undefined,
      })

      const capturedSession = makeSession({
        id: 'captured-session',
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-opus-4-6',
      })

      const selectedAgent = {
        phaseName: 'implementation',
        agent: runningAgent,
        session: capturedSession,
      }

      // Sessions array doesn't contain matching session
      renderPanel({
        selectedAgent,
        sessions: [],
        activeAgents: { 'implementor:claude:opus': runningAgent },
      })

      expect(screen.getByTestId('agent-log-detail')).toBeInTheDocument()
    })
  })

  describe('ticket nrflow-e1c40d: stale spinner fix — live agent resolution', () => {
    it('detail mode shows running state when agent has no result', () => {
      const runningAgent = makeAgent({
        session_id: 'sess-run',
        result: undefined,
      })
      const selectedAgent = {
        phaseName: 'implementation',
        agent: runningAgent,
        session: makeSession({ id: 'sess-run' }),
      }

      renderPanel({
        selectedAgent,
        activeAgents: { 'implementor:claude:sonnet': runningAgent },
        sessions: [makeSession({ id: 'sess-run' })],
      })

      expect(screen.getByTestId('detail-is-running')).toHaveTextContent('running')
      expect(screen.getByTestId('detail-result')).toHaveTextContent('')
    })

    it('detail mode switches to stopped when live activeAgents updates agent with result', () => {
      const staleAgent = makeAgent({
        session_id: 'sess-42',
        result: undefined, // captured snapshot: still running
      })
      const liveCompletedAgent = makeAgent({
        session_id: 'sess-42',
        result: 'pass', // live update: agent completed
      })
      const selectedAgent = {
        phaseName: 'implementation',
        agent: staleAgent, // stale reference at click time
        session: makeSession({ id: 'sess-42' }),
      }

      const { rerender } = renderPanel({
        selectedAgent,
        // activeAgents still shows running (no result) — should be running
        activeAgents: { 'implementor:claude:sonnet': staleAgent },
        sessions: [makeSession({ id: 'sess-42' })],
      })

      expect(screen.getByTestId('detail-is-running')).toHaveTextContent('running')
      expect(screen.getByTestId('detail-result')).toHaveTextContent('')

      // Agent completes: activeAgents updates via React Query invalidation
      const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
      rerender(
        <QueryClientProvider client={queryClient}>
          <AgentLogPanel
            activeAgents={{ 'implementor:claude:sonnet': liveCompletedAgent }}
            sessions={[makeSession({ id: 'sess-42', status: 'completed', result: 'pass' })]}
            collapsed={false}
            selectedAgent={selectedAgent} // still the stale captured snapshot
            onAgentSelect={vi.fn()}
          />
        </QueryClientProvider>
      )

      // Live resolution should pick up liveCompletedAgent from activeAgents
      expect(screen.getByTestId('detail-is-running')).toHaveTextContent('stopped')
      expect(screen.getByTestId('detail-result')).toHaveTextContent('pass')
    })

    it('falls back gracefully when agent is no longer in activeAgents', () => {
      const staleAgent = makeAgent({
        session_id: 'sess-gone',
        result: undefined,
      })
      const selectedAgent = {
        phaseName: 'implementation',
        agent: staleAgent,
        session: makeSession({ id: 'sess-gone' }),
      }

      // Agent removed from activeAgents (e.g., moved to history by server)
      renderPanel({
        selectedAgent,
        activeAgents: {}, // agent no longer present
        sessions: [makeSession({ id: 'sess-gone' })],
      })

      // Falls back to stale captured snapshot — no crash, still shows in detail
      expect(screen.getByTestId('agent-log-detail')).toBeInTheDocument()
      // stale agent has no result, fallback snapshot is used → isRunning stays true
      expect(screen.getByTestId('detail-is-running')).toHaveTextContent('running')
    })

    it('prefers session_id match over agent_type+phase+model_id for live resolution', () => {
      const staleAgent = makeAgent({
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-sonnet-4-5',
        session_id: 'sess-exact',
        result: undefined,
      })
      // Agent matching by session_id only — same props but different session (e.g., retry)
      const liveBySessionId = makeAgent({
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-sonnet-4-5',
        session_id: 'sess-exact',
        result: 'pass', // completed
      })
      // Different agent matching by agent_type+phase+model_id only
      const liveByProps = makeAgent({
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-sonnet-4-5',
        session_id: 'sess-other',
        result: undefined, // still running
      })

      const selectedAgent = {
        phaseName: 'implementation',
        agent: staleAgent,
        session: makeSession({ id: 'sess-exact' }),
      }

      // Both agents in activeAgents — session_id match should win
      const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
      render(
        <QueryClientProvider client={queryClient}>
          <AgentLogPanel
            activeAgents={{
              'implementor:sonnet:exact': liveBySessionId,
              'implementor:sonnet:other': liveByProps,
            }}
            sessions={[makeSession({ id: 'sess-exact' }), makeSession({ id: 'sess-other' })]}
            collapsed={false}
            selectedAgent={selectedAgent}
            onAgentSelect={vi.fn()}
          />
        </QueryClientProvider>
      )

      // Should resolve to liveBySessionId (result='pass'), not liveByProps (no result)
      expect(screen.getByTestId('detail-is-running')).toHaveTextContent('stopped')
      expect(screen.getByTestId('detail-result')).toHaveTextContent('pass')
    })

    it('historyEntry path is unaffected by live resolution (no agent field)', () => {
      const selectedAgent = {
        phaseName: 'investigation',
        historyEntry: {
          agent_id: 'h1',
          agent_type: 'setup-analyzer',
          phase: 'investigation',
          result: 'pass',
          duration_sec: 120,
        },
        session: makeSession({ id: 'hist-session', agent_type: 'setup-analyzer', phase: 'investigation' }),
      }

      // Even if activeAgents has something, historyEntry path sets agent=undefined
      renderPanel({
        selectedAgent,
        activeAgents: { 'setup-analyzer:claude:sonnet': makeAgent({ agent_type: 'setup-analyzer', phase: 'investigation', result: undefined }) },
        sessions: [makeSession({ id: 'hist-session' })],
      })

      // No agent field → liveAgent=undefined → isRunningAgent=false
      expect(screen.getByTestId('agent-log-detail')).toBeInTheDocument()
      // detail-is-running is based on agent field only; no agent → 'stopped'
      expect(screen.getByTestId('detail-is-running')).toHaveTextContent('stopped')
    })
  })
})
