import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AgentLogDetail } from './AgentLogDetail'
import * as ticketsApi from '@/api/tickets'
import type { SelectedAgentData } from './PhaseGraph/types'
import type { ActiveAgentV4, AgentHistoryEntry, AgentSession } from '@/types/workflow'

Element.prototype.scrollIntoView = vi.fn()

vi.mock('@/api/tickets', async () => {
  const actual = await vi.importActual('@/api/tickets')
  return {
    ...actual,
    getSessionMessages: vi.fn(),
  }
})

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

function makeRunningAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
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

function makeHistoryEntry(overrides: Partial<AgentHistoryEntry> = {}): AgentHistoryEntry {
  return {
    agent_id: 'h1',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude-sonnet-4-5',
    result: 'pass',
    duration_sec: 60,
    started_at: '2026-01-01T00:00:00Z',
    ended_at: '2026-01-01T00:01:00Z',
    ...overrides,
  }
}

function renderDetail(selectedAgent: SelectedAgentData) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <AgentLogDetail selectedAgent={selectedAgent} onBack={vi.fn()} />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({ messages: [] })
})

describe('restarted running session — stale fail suppression', () => {
  // The status-circle spinner lives inside .rounded-full; the messages-loading spinner does not.
  // Use `.rounded-full .spin-sync` to target only the header status icon.

  it('shows spinner + yellow ring, no red ring, no XCircle when session.status=running + historyEntry.result=fail', () => {
    renderDetail({
      phaseName: 'implementation',
      agent: makeRunningAgent({ session_id: 'sess-restarted' }),
      historyEntry: makeHistoryEntry({ result: 'fail' }),
      session: makeSession({ id: 'sess-restarted', status: 'running' }),
    })

    expect(document.querySelector('.rounded-full .spin-sync')).not.toBeNull()
    expect(document.querySelector('.bg-yellow-100')).not.toBeNull()
    expect(document.querySelector('.bg-red-100')).toBeNull()
    // XCircle has a specific path; verify via absence of red ring as proxy — no red bg means no fail state
    expect(document.querySelector('.bg-green-100')).toBeNull()
  })

  it('shows spinner + yellow ring, no red ring when session.status=running + agent.result=fail (stale active-agent payload)', () => {
    renderDetail({
      phaseName: 'implementation',
      agent: makeRunningAgent({ session_id: 'sess-stale', result: 'fail' } as Partial<ActiveAgentV4>),
      session: makeSession({ id: 'sess-stale', status: 'running' }),
    })

    expect(document.querySelector('.rounded-full .spin-sync')).not.toBeNull()
    expect(document.querySelector('.bg-yellow-100')).not.toBeNull()
    expect(document.querySelector('.bg-red-100')).toBeNull()
  })

  it('shows blue ring, no red ring, no spinner in status circle when session.status=user_interactive + historyEntry.result=fail', () => {
    renderDetail({
      phaseName: 'implementation',
      agent: makeRunningAgent({ session_id: 'sess-interactive' }),
      historyEntry: makeHistoryEntry({ result: 'fail' }),
      session: makeSession({ id: 'sess-interactive', status: 'user_interactive' }),
    })

    expect(document.querySelector('.bg-blue-100, .bg-blue-900\\/30')).not.toBeNull()
    expect(document.querySelector('.bg-red-100')).toBeNull()
    expect(document.querySelector('.rounded-full .spin-sync')).toBeNull()
  })
})
