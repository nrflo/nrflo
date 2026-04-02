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
    model_id: 'claude-sonnet-4-5',
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
    model_id: 'claude-sonnet-4-5',
    cli: 'claude',
    pid: 12345,
    started_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderDetail(selectedAgent: SelectedAgentData, hideHeader?: boolean) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <AgentLogDetail selectedAgent={selectedAgent} hideHeader={hideHeader} onBack={vi.fn()} />
    </QueryClientProvider>
  )
}

const defaultSelected: SelectedAgentData = {
  phaseName: 'implementation',
  agent: makeRunningAgent(),
  session: makeSession(),
}

describe('AgentLogDetail - hideHeader prop', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [],
      total: 0,
    })
  })

  it('renders phase name and model in header when hideHeader is omitted', () => {
    renderDetail(defaultSelected)
    // Phase name rendered in header
    expect(screen.getByText('implementation')).toBeInTheDocument()
    // model_id 'claude-sonnet-4-5' → last 2 segments = '4-5'
    expect(screen.getByText('4-5')).toBeInTheDocument()
  })

  it('hides phase name and model when hideHeader=true', () => {
    renderDetail(defaultSelected, true)
    expect(screen.queryByText('implementation')).not.toBeInTheDocument()
    expect(screen.queryByText('4-5')).not.toBeInTheDocument()
  })

  it('renders header when hideHeader=false (explicit)', () => {
    renderDetail(defaultSelected, false)
    expect(screen.getByText('implementation')).toBeInTheDocument()
    expect(screen.getByText('4-5')).toBeInTheDocument()
  })

  it('detail tab bar (Messages/Context/Findings) always renders regardless of hideHeader', () => {
    renderDetail(defaultSelected, true)
    expect(screen.getByText('Messages')).toBeInTheDocument()
    expect(screen.getByText('Context')).toBeInTheDocument()
    expect(screen.getByText('Findings')).toBeInTheDocument()
    expect(screen.getByText('All Findings')).toBeInTheDocument()
  })

  it('back button is suppressed when hideHeader=true even when onBack is provided', () => {
    // The back button lives inside the header block — it should not render
    // We can confirm no ArrowLeft button is rendered (no button triggers the callback)
    renderDetail(defaultSelected, true)
    // Messages/Context/Findings/All Findings tab buttons exist, but the back button
    // (first button in header) should not be present. Check via absence of "implementation" header
    // as a proxy — the whole header block is gone.
    const detailContainer = document.querySelector('.flex.flex-col.h-full')!
    // The header div has class 'flex items-center gap-2 px-3 py-2 border-b'
    const headerDiv = detailContainer.querySelector('.px-3.py-2.border-b')
    expect(headerDiv).toBeNull()
  })
})
