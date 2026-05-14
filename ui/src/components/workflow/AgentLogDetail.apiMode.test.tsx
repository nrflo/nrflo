import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AgentLogDetail } from './AgentLogDetail'
import * as ticketsApi from '@/api/tickets'
import type { SelectedAgentData } from './PhaseGraph/types'
import type { AgentHistoryEntry, AgentSession } from '@/types/workflow'

Element.prototype.scrollIntoView = vi.fn()

vi.mock('@/api/tickets', async () => {
  const actual = await vi.importActual('@/api/tickets')
  return { ...actual, getSessionMessages: vi.fn() }
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
    model_id: 'claude:sonnet-4-5',
    status: 'completed',
    message_count: 0,
    restart_count: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeHistoryEntry(overrides: Partial<AgentHistoryEntry> = {}): AgentHistoryEntry {
  return {
    agent_id: 'h1',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude:sonnet-4-5',
    session_id: 'session-1',
    result: 'pass',
    duration_sec: 60,
    started_at: '2026-01-01T00:00:00Z',
    ended_at: '2026-01-01T00:01:00Z',
    ...overrides,
  }
}

function renderDetail(
  selectedAgent: SelectedAgentData,
  extraProps: Partial<React.ComponentProps<typeof AgentLogDetail>> = {},
) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <AgentLogDetail selectedAgent={selectedAgent} {...extraProps} />
    </QueryClientProvider>
  )
}

describe('AgentLogDetail — agentExecutionMode gate on Resume button', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [],
      total: 0,
    })
  })

  it('hides Resume button when agentExecutionMode=api', () => {
    renderDetail(
      {
        phaseName: 'implementation',
        historyEntry: makeHistoryEntry({ result: 'pass', model_id: 'claude:sonnet-4-5' }),
        session: makeSession({ status: 'completed' }),
      },
      { onResumeSession: vi.fn(), agentExecutionMode: 'api' },
    )
    expect(screen.queryByRole('button', { name: /Resume/i })).not.toBeInTheDocument()
  })

  it('shows Resume button when agentExecutionMode=cli_interactive', () => {
    renderDetail(
      {
        phaseName: 'implementation',
        historyEntry: makeHistoryEntry({ result: 'pass', model_id: 'claude:sonnet-4-5' }),
        session: makeSession({ status: 'completed' }),
      },
      { onResumeSession: vi.fn(), agentExecutionMode: 'cli_interactive' },
    )
    expect(screen.getByRole('button', { name: /Resume/i })).toBeInTheDocument()
  })

  it('shows Resume button when agentExecutionMode is undefined (backward compat)', () => {
    renderDetail(
      {
        phaseName: 'implementation',
        historyEntry: makeHistoryEntry({ result: 'pass', model_id: 'claude:sonnet-4-5' }),
        session: makeSession({ status: 'completed' }),
      },
      { onResumeSession: vi.fn() },
    )
    expect(screen.getByRole('button', { name: /Resume/i })).toBeInTheDocument()
  })

  it('hides Resume for api mode even with result=fail', () => {
    renderDetail(
      {
        phaseName: 'implementation',
        historyEntry: makeHistoryEntry({ result: 'fail', model_id: 'claude:sonnet-4-5' }),
        session: makeSession({ status: 'failed' }),
      },
      { onResumeSession: vi.fn(), agentExecutionMode: 'api' },
    )
    expect(screen.queryByRole('button', { name: /Resume/i })).not.toBeInTheDocument()
  })
})
