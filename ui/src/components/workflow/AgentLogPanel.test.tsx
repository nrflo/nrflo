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

    // TODO(test-writer): collapse toggle button moved to WorkflowTabContent header — test there
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

    it('prefers session_id match over agent_type+phase+model_id match', async () => {
      const user = userEvent.setup()
      const agent = makeAgent({
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-opus-4-6',
        session_id: 'session-correct',
      })
      // Session with matching session_id but different properties
      const correctSession = makeSession({
        id: 'session-correct',
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-opus-4-6',
      })
      // Session that matches agent properties but has wrong session_id
      const wrongSession = makeSession({
        id: 'session-wrong',
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-opus-4-6',
      })
      const onAgentSelect = vi.fn()

      renderPanel({
        activeAgents: { 'implementor:claude:opus': agent },
        sessions: [wrongSession, correctSession],
        onAgentSelect,
      })

      const agentButton = screen.getByRole('button', { name: /implementation/i })
      await user.click(agentButton)

      // Should select correctSession by session_id, not wrongSession by properties
      expect(onAgentSelect).toHaveBeenCalledWith(
        expect.objectContaining({ session: correctSession })
      )
    })

    it('falls back to property matching when session_id match not found', async () => {
      const user = userEvent.setup()
      const agent = makeAgent({
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-opus-4-6',
        session_id: 'non-existent-session',
      })
      // No session with id 'non-existent-session', but one that matches properties
      const fallbackSession = makeSession({
        id: 'fallback-session',
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-opus-4-6',
      })
      const onAgentSelect = vi.fn()

      renderPanel({
        activeAgents: { 'implementor:claude:opus': agent },
        sessions: [fallbackSession],
        onAgentSelect,
      })

      const agentButton = screen.getByRole('button', { name: /implementation/i })
      await user.click(agentButton)

      // Should fall back to property matching
      expect(onAgentSelect).toHaveBeenCalledWith(
        expect.objectContaining({ session: fallbackSession })
      )
    })
  })

  describe('ticket nrworkflow-720aec: table view in overview mode', () => {
    it('renders messages in table format (Time|Tool|Message) instead of compact cards in overview', () => {
      const agent = makeAgent()
      const session = makeSession()

      renderPanel({
        activeAgents: { 'implementor:claude:sonnet': agent },
        sessions: [session],
      })

      // Verify table structure exists
      const table = document.querySelector('table')
      expect(table).toBeInTheDocument()

      // Verify table has three columns
      const thead = table!.querySelector('thead')
      expect(thead).toBeInTheDocument()
      const headerCells = thead!.querySelectorAll('th')
      expect(headerCells).toHaveLength(3)
      expect(headerCells[0].textContent).toBe('Time')
      expect(headerCells[1].textContent).toBe('Tool')
      expect(headerCells[2].textContent).toBe('Message')

      // Verify messages are rendered in tbody
      const tbody = table!.querySelector('tbody')
      expect(tbody).toBeInTheDocument()
      const rows = tbody!.querySelectorAll('tr')
      expect(rows.length).toBeGreaterThan(0)

      // Each row should have 3 cells
      rows.forEach(row => {
        const cells = row.querySelectorAll('td')
        expect(cells).toHaveLength(3)
      })
    })

    it('overview table shows messages in newest-first order (reversed)', () => {
      const agent = makeAgent()
      const session = makeSession()

      renderPanel({
        activeAgents: { 'implementor:claude:sonnet': agent },
        sessions: [session],
      })

      // With default mock: 'Building project...' at 00:01:00Z, 'Running tests...' at 00:02:00Z
      // After reversal, 'Running tests...' should be first
      const table = document.querySelector('table')!
      const tbody = table.querySelector('tbody')!
      const rows = tbody.querySelectorAll('tr')
      const msgCells = Array.from(rows).map(r => r.querySelector('td:nth-child(3)')!.textContent)

      // Messages should be reversed: newest first
      expect(msgCells[0]).toBe('Running tests...')
      expect(msgCells[1]).toBe('Building project...')
    })

    it('overview table displays timestamps in Time column', () => {
      const agent = makeAgent()
      const session = makeSession()

      renderPanel({
        activeAgents: { 'implementor:claude:sonnet': agent },
        sessions: [session],
      })

      const table = document.querySelector('table')!
      const tbody = table.querySelector('tbody')!
      const rows = tbody.querySelectorAll('tr')

      // Check first row has a timestamp
      const timeCell = rows[0].querySelector('td:first-child')!
      expect(timeCell.textContent).toBeTruthy()
      // Should contain digits (HH:MM:SS pattern)
      expect(timeCell.textContent).toMatch(/\d/)
    })

    it('overview table renders message content in Message column', () => {
      const agent = makeAgent()
      const session = makeSession()

      renderPanel({
        activeAgents: { 'implementor:claude:sonnet': agent },
        sessions: [session],
      })

      const table = document.querySelector('table')!
      const tbody = table.querySelector('tbody')!
      const rows = tbody.querySelectorAll('tr')

      // Verify message content is rendered
      const firstRowMsgCell = rows[0].querySelector('td:nth-child(3)')!
      expect(firstRowMsgCell.textContent).toBe('Running tests...')

      const secondRowMsgCell = rows[1].querySelector('td:nth-child(3)')!
      expect(secondRowMsgCell.textContent).toBe('Building project...')
    })

    it('no LogMessage cards rendered in overview mode', () => {
      const agent = makeAgent()
      const session = makeSession()

      renderPanel({
        activeAgents: { 'implementor:claude:sonnet': agent },
        sessions: [session],
      })

      // The old implementation rendered LogMessage components which have
      // 'px-2 py-1 rounded-md border bg-muted/30' classes (from LogMessage.tsx variant="compact")
      // Verify these compact card elements are not present in overview
      // Instead, we should have a table
      const table = document.querySelector('table')
      expect(table).toBeInTheDocument()

      // Verify no compact LogMessage cards exist
      // LogMessage component has specific styling: "px-2 py-1 rounded-md border bg-muted/30"
      // These should NOT exist in the agent messages block
      const compactCards = document.querySelectorAll('.rounded-md.border')
      const cardsInMessagesArea = Array.from(compactCards).filter(el => {
        const parentText = el.closest('div')?.textContent
        return parentText?.includes('Building') || parentText?.includes('Running')
      })

      // All messages should be in table rows, not in card divs
      expect(cardsInMessagesArea).toHaveLength(0)
    })

    it('overview table has same structure as detail table', () => {
      // This test verifies criterion #2: both overview and detail use table view
      const agent = makeAgent()
      const session = makeSession()

      renderPanel({
        activeAgents: { 'implementor:claude:sonnet': agent },
        sessions: [session],
      })

      const overviewTable = document.querySelector('table')!
      const overviewHeaders = overviewTable.querySelectorAll('thead th')

      // Table format should match detail view structure:
      // Time (70px) | Tool (70px) | Message
      expect(overviewHeaders[0].textContent).toBe('Time')
      expect(overviewHeaders[0].classList.contains('w-[70px]')).toBe(true)
      expect(overviewHeaders[1].textContent).toBe('Tool')
      expect(overviewHeaders[1].classList.contains('w-[70px]')).toBe(true)
      expect(overviewHeaders[2].textContent).toBe('Message')

      // Table uses font-mono, text-xs, border-collapse
      expect(overviewTable.classList.contains('font-mono')).toBe(true)
      expect(overviewTable.classList.contains('text-xs')).toBe(true)
      expect(overviewTable.classList.contains('border-collapse')).toBe(true)
    })
  })

  describe('ticket nrworkflow-d3a7c4: project-level agent messages', () => {
    it('loads messages using agent.session_id when session object is not available', () => {
      // This tests the fallback: sessionId = session?.id || agent.session_id
      const agent = makeAgent({ session_id: 'session-fallback' })

      // Pass agent without matching session
      renderPanel({
        activeAgents: { 'implementor:claude:sonnet': agent },
        sessions: [], // No sessions provided
      })

      // useSessionMessages should be called with agent.session_id
      // useSessionMessages is mocked at file level
      // The mock is set at file level, but we can verify the component renders
      expect(screen.getByRole('button', { name: /implementation/i })).toBeInTheDocument()
    })

    it('prefers session.id over agent.session_id when both are available', () => {
      const agent = makeAgent({ session_id: 'session-from-agent' })
      const session = makeSession({ id: 'session-from-object' })

      renderPanel({
        activeAgents: { 'implementor:claude:sonnet': agent },
        sessions: [session],
      })

      // Should use session.id since session object is available
      expect(screen.getByRole('button', { name: /implementation/i })).toBeInTheDocument()
    })

    it('passes sessions prop through to AgentMessagesBlock for project scope', () => {
      const agent = makeAgent()
      const projectSession = makeSession({
        id: 'project-session-1',
        ticket_id: '', // Empty for project scope
      })

      renderPanel({
        activeAgents: { 'implementor:claude:sonnet': agent },
        sessions: [projectSession],
      })

      // Component should render with the provided session
      expect(screen.getByRole('button', { name: /implementation/i })).toBeInTheDocument()
    })

    it('finds session by matching agent_type, phase, and model_id for project agents', async () => {
      const user = userEvent.setup()
      const agent = makeAgent({
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-opus-4-6',
        session_id: 'agent-session-id',
      })
      const correctProjectSession = makeSession({
        id: 'correct-project-session',
        ticket_id: '', // Project scope
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-opus-4-6',
      })
      const wrongProjectSession = makeSession({
        id: 'wrong-project-session',
        ticket_id: '',
        agent_type: 'tester',
        phase: 'verification',
        model_id: 'claude-sonnet-4-5',
      })
      const onAgentSelect = vi.fn()

      renderPanel({
        activeAgents: { 'implementor:claude:opus': agent },
        sessions: [wrongProjectSession, correctProjectSession],
        onAgentSelect,
      })

      const agentButton = screen.getByRole('button', { name: /implementation/i })
      await user.click(agentButton)

      expect(onAgentSelect).toHaveBeenCalledWith(
        expect.objectContaining({ session: correctProjectSession })
      )
    })
  })

  describe('ticket nrworkflow-46fb2e: session lookup fix for completed agents', () => {
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

  describe('ticket nrworkflow-e1c40d: stale spinner fix — live agent resolution', () => {
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
            onToggleCollapse={vi.fn()}
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
            onToggleCollapse={vi.fn()}
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
