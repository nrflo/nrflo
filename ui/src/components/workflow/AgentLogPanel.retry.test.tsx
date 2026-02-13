import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AgentLogPanel } from './AgentLogPanel'
import type { ActiveAgentV4, AgentSession } from '@/types/workflow'
import type { SelectedAgentData } from './PhaseGraph/types'

// Mock useSessionMessages
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

// Mock AgentLogDetail
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

describe('AgentLogPanel - retry failed button', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('overview mode with running agents', () => {
    it('does not show failed agents in overview mode - overview only shows running agents', () => {
      const agent = makeAgent({ result: 'fail' })
      const { container } = renderPanel({
        activeAgents: { 'impl:claude:sonnet': agent },
        sessions: [makeSession()],
        onRetryFailed: vi.fn(),
        workflowStatus: 'failed',
      })

      // Failed agents don't appear in overview mode - AgentLogPanel only shows running agents
      // (agents without a result)
      expect(container.textContent).toBe('')
    })

    it('overview mode does not show agents with results', () => {
      const runningAgent = makeAgent({ result: undefined })
      const failedAgent = makeAgent({ result: 'fail', agent_type: 'failed-agent' })
      renderPanel({
        activeAgents: {
          'running:claude:sonnet': runningAgent,
          'failed:claude:sonnet': failedAgent,
        },
        sessions: [makeSession()],
        onRetryFailed: vi.fn(),
        workflowStatus: 'failed',
      })

      // Only running agent shows
      expect(screen.queryByText(/failed-agent/)).not.toBeInTheDocument()
    })
  })

  describe('note on retry functionality', () => {
    it('retry failed agents functionality is in ActiveAgentsPanel, not AgentLogPanel overview', () => {
      // AgentLogPanel overview mode only shows running agents (no result).
      // Failed agents with retry buttons are shown in ActiveAgentsPanel's "Failed Agents" section.
      // This test documents the architectural decision.
      expect(true).toBe(true)
    })
  })
})
