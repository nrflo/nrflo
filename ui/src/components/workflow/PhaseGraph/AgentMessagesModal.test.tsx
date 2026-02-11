import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AgentMessagesModal } from './AgentMessagesModal'
import type { ActiveAgentV4, AgentSession, AgentHistoryEntry } from '@/types/workflow'

vi.mock('@/api/tickets', () => ({
  getSessionMessages: vi.fn().mockResolvedValue({ session_id: 's1', messages: [], total: 0 }),
}))

// jsdom doesn't implement scrollIntoView
Element.prototype.scrollIntoView = vi.fn()

function makeAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_id: 'a1',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude-sonnet-4-5',
    cli: 'claude',
    model: 'sonnet',
    pid: 12345,
    session_id: 's1',
    started_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeSession(overrides: Partial<AgentSession> = {}): AgentSession {
  return {
    id: 's1',
    project_id: 'proj1',
    ticket_id: 'T-1',
    workflow_instance_id: 'wi1',
    phase: 'implementation',
    workflow: 'feature',
    agent_type: 'implementor',
    model_id: 'claude-sonnet-4-5',
    status: 'running',
    message_count: 5,
    last_messages: ['[Read] src/main.ts', '[Edit] src/utils.ts'],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeHistory(overrides: Partial<AgentHistoryEntry> = {}): AgentHistoryEntry {
  return {
    agent_id: 'a1',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude-sonnet-4-5',
    result: 'pass',
    started_at: '2026-01-01T00:00:00Z',
    ended_at: '2026-01-01T00:01:30Z',
    duration_sec: 90,
    ...overrides,
  }
}

function renderModal(props: {
  open?: boolean
  onClose?: () => void
  phaseName?: string
  agent?: ActiveAgentV4
  historyEntry?: AgentHistoryEntry
  session?: AgentSession
}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <AgentMessagesModal
        open={props.open ?? true}
        onClose={props.onClose ?? vi.fn()}
        phaseName={props.phaseName ?? 'implementation'}
        agent={props.agent}
        historyEntry={props.historyEntry}
        session={props.session}
      />
    </QueryClientProvider>
  )
}

describe('AgentMessagesModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders nothing when open is false', () => {
    renderModal({ open: false, agent: makeAgent() })
    expect(screen.queryByText('implementation')).not.toBeInTheDocument()
  })

  it('renders phase name in header', () => {
    renderModal({ agent: makeAgent(), phaseName: 'investigation' })
    expect(screen.getByText('investigation')).toBeInTheDocument()
  })

  it('shows running spinner for active agent without result', () => {
    renderModal({ agent: makeAgent() })
    const spinnerContainer = document.querySelector('.animate-spin')
    expect(spinnerContainer).toBeInTheDocument()
  })

  it('shows pass badge for completed agent', () => {
    renderModal({
      historyEntry: makeHistory({ result: 'pass' }),
    })
    expect(screen.getByText('pass')).toBeInTheDocument()
  })

  it('shows fail badge for failed agent', () => {
    renderModal({
      historyEntry: makeHistory({ result: 'fail' }),
    })
    expect(screen.getByText('fail')).toBeInTheDocument()
  })

  it('displays model name derived from model_id', () => {
    renderModal({
      agent: makeAgent({ model_id: 'claude-sonnet-4-5' }),
    })
    // model_id.split('-').slice(-2).join('-') => "4-5"
    expect(screen.getByText('4-5')).toBeInTheDocument()
  })

  it('displays duration for history entries', () => {
    renderModal({
      historyEntry: makeHistory({ duration_sec: 90 }),
    })
    expect(screen.getByText('1m 30s')).toBeInTheDocument()
  })

  it('shows "No messages available" when session has no messages', async () => {
    renderModal({
      agent: makeAgent(),
      session: makeSession(),
    })
    // The query resolves with empty messages, then shows the empty state
    expect(await screen.findByText('No messages available')).toBeInTheDocument()
  })

  it('renders messages using LogMessage with full variant', async () => {
    const { getSessionMessages } = await import('@/api/tickets')
    vi.mocked(getSessionMessages).mockResolvedValue({
      session_id: 's1',
      messages: ['[Read] file.ts', '[Edit] other.ts'],
      total: 2,
    })

    renderModal({
      agent: makeAgent(),
      session: makeSession(),
    })

    // Wait for messages to load
    const msg1 = await screen.findByText('[Read] file.ts')
    expect(msg1).toBeInTheDocument()
    // Full variant uses whitespace-pre-wrap, not truncate
    expect(msg1.className).toContain('whitespace-pre-wrap')
    expect(msg1.className).not.toContain('truncate')

    const msg2 = screen.getByText('[Edit] other.ts')
    expect(msg2).toBeInTheDocument()
    expect(msg2.className).toContain('whitespace-pre-wrap')
  })

  it('shows total message count from API response', async () => {
    const { getSessionMessages } = await import('@/api/tickets')
    vi.mocked(getSessionMessages).mockResolvedValue({
      session_id: 's1',
      messages: ['msg1'],
      total: 42,
    })

    renderModal({
      agent: makeAgent(),
      session: makeSession(),
    })

    expect(await screen.findByText('42 total messages')).toBeInTheDocument()
  })

  it('falls back to cli for model name when model_id is absent', () => {
    renderModal({
      agent: makeAgent({ model_id: undefined, cli: 'opencode' }),
    })
    expect(screen.getByText('opencode')).toBeInTheDocument()
  })

  it('replaces underscores in phase name', () => {
    renderModal({
      agent: makeAgent(),
      phaseName: 'test_design',
    })
    expect(screen.getByText('test design')).toBeInTheDocument()
  })
})
