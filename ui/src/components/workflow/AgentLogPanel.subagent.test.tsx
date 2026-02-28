import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AgentLogPanel } from './AgentLogPanel'
import type { ActiveAgentV4, AgentSession, MessageWithTime } from '@/types/workflow'

// Module-scoped variable so tests can control returned messages
let mockMessages: MessageWithTime[] = []

vi.mock('@/hooks/useTickets', () => ({
  useSessionMessages: () => ({
    data: {
      session_id: 'session-1',
      messages: mockMessages,
      total: mockMessages.length,
    },
    isLoading: false,
  }),
}))

// Mock AgentLogDetail to avoid deep dependencies
vi.mock('./AgentLogDetail', async () => {
  const actual = await vi.importActual<typeof import('./AgentLogDetail')>('./AgentLogDetail')
  return {
    ...actual,
    AgentLogDetail: () => <div data-testid="agent-log-detail">Detail View</div>,
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

function renderPanel(messages: MessageWithTime[], props: Partial<React.ComponentProps<typeof AgentLogPanel>> = {}) {
  mockMessages = messages

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  const defaultProps = {
    activeAgents: { 'implementor:claude:sonnet': makeAgent() },
    sessions: [makeSession()],
    collapsed: false,
    onToggleCollapse: vi.fn(),
    selectedAgent: null,
    onAgentSelect: vi.fn(),
    ...props,
  }

  return render(
    <QueryClientProvider client={queryClient}>
      <AgentLogPanel {...defaultProps} />
    </QueryClientProvider>
  )
}

describe('AgentLogPanel - subagent count indicator', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockMessages = []
  })

  it('shows "1 sub-agent" (singular) when one subagent message exists', () => {
    renderPanel([
      { content: 'text msg', category: 'text', created_at: '2026-01-01T00:00:10Z' },
      { content: '[Agent] sub-agent', category: 'subagent', created_at: '2026-01-01T00:00:20Z' },
    ])

    expect(screen.getByText('1 sub-agent')).toBeInTheDocument()
  })

  it('shows "N sub-agents" (plural) when multiple subagent messages exist', () => {
    renderPanel([
      { content: '[Agent] agent 1', category: 'subagent', created_at: '2026-01-01T00:00:10Z' },
      { content: '[AgentResult] done 1', category: 'subagent', created_at: '2026-01-01T00:00:11Z' },
      { content: '[Task] agent 2 (legacy)', category: 'subagent', created_at: '2026-01-01T00:00:20Z' },
    ])

    expect(screen.getByText('3 sub-agents')).toBeInTheDocument()
  })

  it('does not show sub-agent indicator when no subagent messages', () => {
    renderPanel([
      { content: 'text message', category: 'text', created_at: '2026-01-01T00:00:10Z' },
      { content: '[Bash] tool call', category: 'tool', created_at: '2026-01-01T00:00:20Z' },
    ])

    expect(screen.queryByText(/sub-agent/)).not.toBeInTheDocument()
  })

  it('does not show sub-agent indicator when messages list is empty', () => {
    renderPanel([])

    expect(screen.queryByText(/sub-agent/)).not.toBeInTheDocument()
  })

  it('correctly counts only subagent-category messages (ignores text and tool)', () => {
    renderPanel([
      { content: 'text 1', category: 'text', created_at: '2026-01-01T00:00:01Z' },
      { content: '[Bash] tool', category: 'tool', created_at: '2026-01-01T00:00:02Z' },
      { content: '[Agent] sub 1', category: 'subagent', created_at: '2026-01-01T00:00:03Z' },
      { content: '[Skill] skill', category: 'skill', created_at: '2026-01-01T00:00:04Z' },
      { content: '[AgentResult] done', category: 'subagent', created_at: '2026-01-01T00:00:05Z' },
    ])

    // Only 2 subagent messages out of 5 total
    expect(screen.getByText('2 sub-agents')).toBeInTheDocument()
  })

  it('counts [Agent] and legacy [Task] messages equally when both have subagent category', () => {
    renderPanel([
      { content: '[Task] old-style sub-agent', category: 'subagent', created_at: '2026-01-01T00:00:10Z' },
      { content: '[Agent] new-style sub-agent', category: 'subagent', created_at: '2026-01-01T00:00:20Z' },
    ])

    expect(screen.getByText('2 sub-agents')).toBeInTheDocument()
  })
})
