/**
 * Model-label tests for AgentLogDetail's header.
 *
 * Covers the de-truncation change: the header shows the full model slug
 * (only the cli: prefix is stripped) instead of the last two hyphen
 * segments, and exposes the raw model_id via a title attribute.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AgentLogDetail } from './AgentLogDetail'
import * as ticketsApi from '@/api/tickets'
import type { SelectedAgentData } from './PhaseGraph/types'
import type { ActiveAgentV4, AgentSession } from '@/types/workflow'

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
    model_id: 'claude:opus-5-1m',
    status: 'running',
    message_count: 0,
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
    model_id: 'claude:opus-5-1m',
    cli: 'claude',
    pid: 12345,
    started_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderDetail(selectedAgent: SelectedAgentData) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <AgentLogDetail selectedAgent={selectedAgent} onBack={vi.fn()} />
    </QueryClientProvider>
  )
}

describe('AgentLogDetail - model label', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [],
      total: 0,
    })
  })

  it('shows the full model slug without the last-two-segments truncation', () => {
    renderDetail({
      phaseName: 'implementation',
      agent: makeRunningAgent({ model_id: 'claude:opus-5-1m' }),
      session: makeSession({ model_id: 'claude:opus-5-1m' }),
    })
    expect(screen.getByText('opus-5-1m')).toBeInTheDocument()
    expect(screen.queryByText('5-1m')).not.toBeInTheDocument()
  })

  it('exposes the raw model_id via a title attribute', () => {
    renderDetail({
      phaseName: 'implementation',
      agent: makeRunningAgent({ model_id: 'claude:opus-5-1m' }),
      session: makeSession({ model_id: 'claude:opus-5-1m' }),
    })
    expect(screen.getByText('opus-5-1m')).toHaveAttribute('title', 'claude:opus-5-1m')
  })
})
