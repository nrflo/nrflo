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
      ],
      total: 1,
    },
    isLoading: false,
  }),
}))

// Mock AgentLogDetail to isolate restart button tests
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
    }) => (
      <div data-testid="agent-log-detail">
        <span data-testid="detail-agent-type">{selectedAgent.agent?.agent_type}</span>
        <button data-testid="back-button" onClick={onBack}>Back</button>
      </div>
    ),
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
    session_id: 'session-abc',
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

describe('AgentLogPanel - restart button', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('overview mode', () => {
    it('shows restart button for running agent when onRestart is provided', () => {
      const agent = makeAgent()
      renderPanel({
        activeAgents: { 'impl:claude:sonnet': agent },
        sessions: [makeSession()],
        onRestart: vi.fn(),
      })

      expect(screen.getByTitle('Restart agent (save context, relaunch)')).toBeInTheDocument()
    })

    it('does not show restart button when onRestart is not provided', () => {
      const agent = makeAgent()
      renderPanel({
        activeAgents: { 'impl:claude:sonnet': agent },
        sessions: [makeSession()],
      })

      expect(screen.queryByTitle('Restart agent (save context, relaunch)')).not.toBeInTheDocument()
    })

    it('does not show restart button for agent without session_id', () => {
      const agent = makeAgent({ session_id: undefined })
      renderPanel({
        activeAgents: { 'impl:claude:sonnet': agent },
        sessions: [makeSession()],
        onRestart: vi.fn(),
      })

      expect(screen.queryByTitle('Restart agent (save context, relaunch)')).not.toBeInTheDocument()
    })

    it('calls onRestart with session_id when restart button clicked', async () => {
      const user = userEvent.setup()
      const onRestart = vi.fn()
      const agent = makeAgent({ session_id: 'my-session' })

      renderPanel({
        activeAgents: { 'impl:claude:sonnet': agent },
        sessions: [makeSession()],
        onRestart,
      })

      await user.click(screen.getByTitle('Restart agent (save context, relaunch)'))
      expect(onRestart).toHaveBeenCalledWith('my-session')
    })

    it('restart button click does not propagate to agent selection', async () => {
      const user = userEvent.setup()
      const onRestart = vi.fn()
      const onAgentSelect = vi.fn()
      const agent = makeAgent({ session_id: 'my-session' })

      renderPanel({
        activeAgents: { 'impl:claude:sonnet': agent },
        sessions: [makeSession()],
        onRestart,
        onAgentSelect,
      })

      await user.click(screen.getByTitle('Restart agent (save context, relaunch)'))

      // onRestart should fire
      expect(onRestart).toHaveBeenCalledWith('my-session')
      // onAgentSelect should NOT fire (stopPropagation)
      expect(onAgentSelect).not.toHaveBeenCalled()
    })

    it('disables restart button when restartingSessionId matches', () => {
      const agent = makeAgent({ session_id: 'sess-1' })
      renderPanel({
        activeAgents: { 'impl:claude:sonnet': agent },
        sessions: [makeSession()],
        onRestart: vi.fn(),
        restartingSessionId: 'sess-1',
      })

      expect(screen.getByTitle('Restart agent (save context, relaunch)')).toBeDisabled()
    })

    it('does not disable restart button when restartingSessionId differs', () => {
      const agent = makeAgent({ session_id: 'sess-1' })
      renderPanel({
        activeAgents: { 'impl:claude:sonnet': agent },
        sessions: [makeSession()],
        onRestart: vi.fn(),
        restartingSessionId: 'sess-other',
      })

      expect(screen.getByTitle('Restart agent (save context, relaunch)')).not.toBeDisabled()
    })

    it('does not call onRestart when disabled button is clicked', async () => {
      const user = userEvent.setup()
      const onRestart = vi.fn()
      const agent = makeAgent({ session_id: 'sess-1' })

      renderPanel({
        activeAgents: { 'impl:claude:sonnet': agent },
        sessions: [makeSession()],
        onRestart,
        restartingSessionId: 'sess-1',
      })

      await user.click(screen.getByTitle('Restart agent (save context, relaunch)'))
      expect(onRestart).not.toHaveBeenCalled()
    })
  })

  describe('multiple running agents', () => {
    it('shows restart buttons for each running agent', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ session_id: 'sess-1' }),
        'tester:claude:opus': makeAgent({
          agent_id: 'a2',
          agent_type: 'tester',
          phase: 'verification',
          session_id: 'sess-2',
        }),
      }
      renderPanel({
        activeAgents: agents,
        sessions: [
          makeSession(),
          makeSession({ id: 'sess-2', agent_type: 'tester', phase: 'verification' }),
        ],
        onRestart: vi.fn(),
      })

      expect(screen.getAllByTitle('Restart agent (save context, relaunch)')).toHaveLength(2)
    })
  })
})
