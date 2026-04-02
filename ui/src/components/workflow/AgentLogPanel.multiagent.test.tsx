/**
 * Multi-agent view tests for AgentLogPanel.
 *
 * Covers behavior specific to the new always-detail mode:
 * - Multi-agent rendering (each running agent gets its own AgentLogDetail)
 * - onBack is NOT passed in multi-agent view (no back button)
 * - phaseName derived from agent.phase || agent.agent_type || ''
 * - findSession in multi-agent path (by session_id, then agent_type+phase fallback)
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentLogPanel } from './AgentLogPanel'
import type { ActiveAgentV4, AgentSession } from '@/types/workflow'
import type { SelectedAgentData } from './PhaseGraph/types'

vi.mock('@/hooks/useTickets', () => ({
  useSessionMessages: vi.fn(() => ({ data: { messages: [] } })),
}))

// Mock with optional onBack and hideHeader — renders indicators so tests can assert on them.
vi.mock('./AgentLogDetail', () => ({
  AgentLogDetail: ({
    selectedAgent,
    onBack,
    hideHeader,
  }: {
    selectedAgent: SelectedAgentData
    onBack?: () => void
    hideHeader?: boolean
  }) => (
    <div data-testid="agent-log-detail">
      <span data-testid="detail-phase">{selectedAgent.phaseName}</span>
      <span data-testid="detail-agent-type">
        {selectedAgent.agent?.agent_type || selectedAgent.historyEntry?.agent_type}
      </span>
      <span data-testid="detail-session-id">{selectedAgent.session?.id ?? 'none'}</span>
      {hideHeader && <span data-testid="detail-hide-header">true</span>}
      {onBack && <button data-testid="back-button" onClick={onBack}>Back</button>}
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
    message_count: 0,
    restart_count: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderPanel(props: Partial<React.ComponentProps<typeof AgentLogPanel>> = {}) {
  const defaultProps = {
    activeAgents: {} as Record<string, ActiveAgentV4>,
    sessions: [] as AgentSession[],
    collapsed: false,
    selectedAgent: null as SelectedAgentData | null,
    onAgentSelect: vi.fn(),
  }
  return render(<AgentLogPanel {...defaultProps} {...props} />)
}

describe('AgentLogPanel - multi-agent detail view', () => {
  beforeEach(() => vi.clearAllMocks())

  describe('rendering', () => {
    it('renders exactly one AgentLogDetail when one agent is running', () => {
      renderPanel({
        activeAgents: { 'a1': makeAgent() },
        sessions: [makeSession()],
      })
      expect(screen.getAllByTestId('agent-log-detail')).toHaveLength(1)
    })

    it('renders one AgentLogDetail with tabs for three running agents', () => {
      renderPanel({
        activeAgents: {
          'a1': makeAgent({ agent_id: 'a1', agent_type: 'implementor', phase: 'implementation' }),
          'a2': makeAgent({ agent_id: 'a2', agent_type: 'tester', phase: 'verification' }),
          'a3': makeAgent({ agent_id: 'a3', agent_type: 'doc-updater', phase: 'docs' }),
        },
        sessions: [],
      })
      expect(screen.getAllByTestId('agent-log-detail')).toHaveLength(1)
      expect(screen.getAllByTestId('agent-tab')).toHaveLength(3)
    })

    it('excludes completed agents (with result) from multi-agent view', () => {
      renderPanel({
        activeAgents: {
          'a1': makeAgent({ result: 'pass' }),  // completed — excluded
          'a2': makeAgent({ agent_id: 'a2', agent_type: 'tester', phase: 'verification' }),
        },
        sessions: [],
      })
      // Only a2 (no result) shown
      expect(screen.getAllByTestId('agent-log-detail')).toHaveLength(1)
      expect(screen.getByTestId('detail-agent-type')).toHaveTextContent('tester')
    })
  })

  describe('onBack not passed in multi-agent view', () => {
    it('no back button when multiple agents are running (onBack absent)', () => {
      renderPanel({
        activeAgents: {
          'a1': makeAgent(),
          'a2': makeAgent({ agent_id: 'a2', agent_type: 'tester', phase: 'verification' }),
        },
        sessions: [],
      })
      expect(screen.queryByTestId('back-button')).not.toBeInTheDocument()
    })

    it('no back button for single running agent without selection', () => {
      renderPanel({
        activeAgents: { 'a1': makeAgent() },
        sessions: [],
      })
      expect(screen.queryByTestId('back-button')).not.toBeInTheDocument()
    })

    it('back button IS present when specific agent is selected (selectedAgent prop)', () => {
      const agent = makeAgent()
      const session = makeSession()
      renderPanel({
        selectedAgent: { phaseName: 'implementation', agent, session },
        sessions: [session],
      })
      expect(screen.getByTestId('back-button')).toBeInTheDocument()
    })
  })

  describe('phaseName derivation', () => {
    it('uses agent.phase as phaseName', () => {
      renderPanel({
        activeAgents: { 'a1': makeAgent({ phase: 'qa-phase' }) },
        sessions: [],
      })
      expect(screen.getByTestId('detail-phase')).toHaveTextContent('qa-phase')
    })

    it('falls back to agent_type when phase is empty string', () => {
      renderPanel({
        activeAgents: { 'a1': makeAgent({ agent_type: 'doc-writer', phase: '' }) },
        sessions: [],
      })
      expect(screen.getByTestId('detail-phase')).toHaveTextContent('doc-writer')
    })

    it('falls back to agent_type when phase is undefined', () => {
      renderPanel({
        activeAgents: { 'a1': makeAgent({ agent_type: 'setup-analyzer', phase: undefined }) },
        sessions: [],
      })
      expect(screen.getByTestId('detail-phase')).toHaveTextContent('setup-analyzer')
    })
  })

  describe('tab interaction', () => {
    it('clicking a second tab switches the displayed agent', async () => {
      const user = userEvent.setup()
      renderPanel({
        activeAgents: {
          'a1': makeAgent({ agent_id: 'a1', agent_type: 'implementor', phase: 'implementation' }),
          'a2': makeAgent({ agent_id: 'a2', agent_type: 'tester', phase: 'verification' }),
        },
        sessions: [],
      })
      expect(screen.getByTestId('detail-agent-type')).toHaveTextContent('implementor')
      const tabs = screen.getAllByTestId('agent-tab')
      await user.click(tabs[1])
      expect(screen.getByTestId('detail-agent-type')).toHaveTextContent('tester')
    })

    it('auto-selects next tab when current active tab agent completes', () => {
      const a1 = makeAgent({ agent_id: 'a1', agent_type: 'implementor', phase: 'implementation' })
      const a2 = makeAgent({ agent_id: 'a2', agent_type: 'tester', phase: 'verification' })
      const onAgentSelect = vi.fn()

      const { rerender } = renderPanel({ activeAgents: { 'a1': a1, 'a2': a2 }, sessions: [] })
      expect(screen.getByTestId('detail-agent-type')).toHaveTextContent('implementor')

      rerender(
        <AgentLogPanel
          activeAgents={{ 'a1': { ...a1, result: 'pass' }, 'a2': a2 }}
          sessions={[]}
          collapsed={false}
          selectedAgent={null}
          onAgentSelect={onAgentSelect}
        />
      )
      expect(screen.getByTestId('detail-agent-type')).toHaveTextContent('tester')
    })
  })

  describe('findSession in multi-agent view', () => {
    it('passes session matched by session_id to AgentLogDetail', () => {
      const session = makeSession({ id: 'sess-exact' })
      const agent = makeAgent({ session_id: 'sess-exact' })
      renderPanel({
        activeAgents: { 'a1': agent },
        sessions: [session],
      })
      expect(screen.getByTestId('detail-session-id')).toHaveTextContent('sess-exact')
    })

    it('falls back to agent_type+phase+model_id when agent has no session_id', () => {
      const session = makeSession({
        id: 'sess-fallback',
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-sonnet-4-5',
      })
      const agent = makeAgent({ session_id: undefined })
      renderPanel({
        activeAgents: { 'a1': agent },
        sessions: [session],
      })
      expect(screen.getByTestId('detail-session-id')).toHaveTextContent('sess-fallback')
    })

    it('session is undefined when no session matches', () => {
      // Agent type mismatch — findSession returns undefined
      const agent = makeAgent({ agent_type: 'special-agent', session_id: undefined })
      const session = makeSession({ agent_type: 'implementor' })
      renderPanel({
        activeAgents: { 'a1': agent },
        sessions: [session],
      })
      expect(screen.getByTestId('detail-session-id')).toHaveTextContent('none')
    })

    it('prefers session_id match over agent_type+phase fallback', () => {
      const correctSession = makeSession({ id: 'sess-by-id', agent_type: 'tester', phase: 'verification' })
      const fallbackSession = makeSession({ id: 'sess-by-type', agent_type: 'implementor', phase: 'implementation' })
      // agent_type matches fallbackSession, but session_id matches correctSession
      const agent = makeAgent({
        agent_type: 'implementor',
        phase: 'implementation',
        session_id: 'sess-by-id',
      })
      renderPanel({
        activeAgents: { 'a1': agent },
        sessions: [correctSession, fallbackSession],
      })
      expect(screen.getByTestId('detail-session-id')).toHaveTextContent('sess-by-id')
    })
  })

  describe('tab label includes model name', () => {
    it('shows "phase : model" format for hyphen-separated model_id', () => {
      // model_id 'claude-sonnet-4-5' → slice(-2).join('-') = '4-5'
      renderPanel({
        activeAgents: { 'a1': makeAgent({ phase: 'implementation', model_id: 'claude-sonnet-4-5' }) },
        sessions: [],
      })
      const tab = screen.getByTestId('agent-tab')
      expect(tab).toHaveTextContent('implementation : 4-5')
    })

    it('passes through colon-separated model_id unchanged', () => {
      // 'claude:opus'.split('-') = ['claude:opus'] → slice(-2) = ['claude:opus'] → join = 'claude:opus'
      renderPanel({
        activeAgents: { 'a1': makeAgent({ phase: 'ticket-creator', model_id: 'claude:opus' }) },
        sessions: [],
      })
      const tab = screen.getByTestId('agent-tab')
      expect(tab).toHaveTextContent('ticket-creator : claude:opus')
    })

    it('falls back to agent.cli when model_id is undefined', () => {
      renderPanel({
        activeAgents: { 'a1': makeAgent({ phase: 'implementation', model_id: undefined, cli: 'claude' }) },
        sessions: [],
      })
      const tab = screen.getByTestId('agent-tab')
      expect(tab).toHaveTextContent('implementation : claude')
    })

    it('shows only phase name when both model_id and cli are absent', () => {
      renderPanel({
        activeAgents: { 'a1': makeAgent({ phase: 'implementation', model_id: undefined, cli: undefined }) },
        sessions: [],
      })
      const tab = screen.getByTestId('agent-tab')
      expect(tab.textContent?.trim()).toBe('implementation')
    })

    it('replaces underscores with spaces in phase portion of label', () => {
      renderPanel({
        activeAgents: { 'a1': makeAgent({ phase: 'setup_analyzer', model_id: 'claude:sonnet' }) },
        sessions: [],
      })
      const tab = screen.getByTestId('agent-tab')
      expect(tab).toHaveTextContent('setup analyzer : claude:sonnet')
    })
  })

  describe('hideHeader prop on AgentLogDetail', () => {
    it('passes hideHeader=true to AgentLogDetail in multi-agent tabbed view', () => {
      renderPanel({
        activeAgents: { 'a1': makeAgent() },
        sessions: [],
      })
      expect(screen.getByTestId('detail-hide-header')).toBeInTheDocument()
    })

    it('does NOT pass hideHeader to AgentLogDetail in selected-agent view', () => {
      const agent = makeAgent()
      const session = makeSession()
      renderPanel({
        activeAgents: {},
        selectedAgent: { phaseName: 'implementation', agent, session },
        sessions: [session],
      })
      expect(screen.queryByTestId('detail-hide-header')).not.toBeInTheDocument()
    })
  })
})
