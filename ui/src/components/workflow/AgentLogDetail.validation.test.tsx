import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AgentLogDetail } from './AgentLogDetail'
import * as ticketsApi from '@/api/tickets'
import type { SelectedAgentData } from './PhaseGraph/types'
import type { ActiveAgentV4, AgentSession } from '@/types/workflow'

Element.prototype.scrollIntoView = vi.fn()

vi.mock('@/api/tickets', async () => {
  const actual = await vi.importActual('@/api/tickets')
  return {
    ...actual,
    getSessionMessages: vi.fn(),
  }
})

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

function renderDetail(selectedAgent: SelectedAgentData) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <AgentLogDetail selectedAgent={selectedAgent} onBack={vi.fn()} />
    </QueryClientProvider>
  )
}

const baseSelectedAgent: SelectedAgentData = {
  phaseName: 'implementation',
  agent: makeRunningAgent(),
  session: makeSession(),
}

describe('AgentLogDetail - validation category', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows Validation tab in category tablist', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [{ content: 'hello', category: 'text', created_at: '2026-01-01T00:00:10Z' }],
      total: 1,
    })

    renderDetail(baseSelectedAgent)
    await waitFor(() => expect(screen.getByText('1 messages')).toBeInTheDocument())

    expect(screen.getByRole('tab', { name: /Validation/ })).toBeInTheDocument()
  })

  it('Validation tab count shows correct count', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'text msg', category: 'text', created_at: '2026-01-01T00:00:10Z' },
        { content: 'validation output', category: 'validation', created_at: '2026-01-01T00:00:20Z' },
        { content: 'another validation', category: 'validation', created_at: '2026-01-01T00:00:30Z' },
      ],
      total: 3,
    })

    renderDetail(baseSelectedAgent)
    await waitFor(() => expect(screen.getByText('3 messages')).toBeInTheDocument())

    // tabs order: all=0, text=1, tool=2, subagent=3, skill=4, user_input=5, error=6, result=7, validation=8
    const tabs = screen.getAllByRole('tab')
    expect(tabs[8].textContent).toContain('2')
  })

  it('Validation tab is at index 8 in the tab list (9 total tabs)', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [{ content: 'hello', category: 'text', created_at: '2026-01-01T00:00:10Z' }],
      total: 1,
    })

    renderDetail(baseSelectedAgent)
    await waitFor(() => expect(screen.getByText('1 messages')).toBeInTheDocument())

    const tabs = screen.getAllByRole('tab')
    expect(tabs).toHaveLength(9)
    expect(tabs[8].textContent).toMatch(/Validation/)
  })

  it('filtering by Validation shows only validation messages', async () => {
    const user = userEvent.setup()
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'plain text', category: 'text', created_at: '2026-01-01T00:00:10Z' },
        { content: 'error occurred', category: 'error', created_at: '2026-01-01T00:00:20Z' },
        { content: 'validation failed: test suite', category: 'validation', created_at: '2026-01-01T00:00:30Z' },
      ],
      total: 3,
    })

    renderDetail(baseSelectedAgent)
    await waitFor(() => expect(screen.getByText('3 messages')).toBeInTheDocument())

    await user.click(screen.getByRole('tab', { name: /Validation/ }))

    await waitFor(() => expect(screen.getByText('1 of 3 messages')).toBeInTheDocument())

    expect(screen.getByText('validation failed: test suite')).toBeInTheDocument()
    expect(screen.queryByText('plain text')).not.toBeInTheDocument()
    expect(screen.queryByText('error occurred')).not.toBeInTheDocument()
  })

  it('validation row has border-l-destructive styling', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'validation output', category: 'validation', created_at: '2026-01-01T00:00:10Z' },
      ],
      total: 1,
    })

    renderDetail(baseSelectedAgent)
    await waitFor(() => expect(screen.getByText('1 messages')).toBeInTheDocument())

    const rows = document.querySelectorAll('[data-testid="message-row"]')
    expect(rows).toHaveLength(1)
    expect(rows[0].className).toContain('border-l-destructive')
  })

  it('validation row has Validation badge in Tool column', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'make test output', category: 'validation', created_at: '2026-01-01T00:00:10Z' },
      ],
      total: 1,
    })

    renderDetail(baseSelectedAgent)
    await waitFor(() => expect(screen.getByText('1 messages')).toBeInTheDocument())

    const rows = document.querySelectorAll('[data-testid="message-row"]')
    expect(rows).toHaveLength(1)
    const toolCell = rows[0].querySelectorAll(':scope > td')[1]
    expect(within(toolCell as HTMLElement).getByText('Validation')).toBeInTheDocument()
  })

  it('validation count is zero when no validation messages', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'just text', category: 'text', created_at: '2026-01-01T00:00:10Z' },
      ],
      total: 1,
    })

    renderDetail(baseSelectedAgent)
    await waitFor(() => expect(screen.getByText('1 messages')).toBeInTheDocument())

    const tabs = screen.getAllByRole('tab')
    expect(tabs[8].textContent).toContain('0')
  })
})
