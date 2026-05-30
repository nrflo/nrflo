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

describe('AgentLogDetail - thinking category', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('Thinking tab exists at index 9 (last tab)', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [{ content: 'a thought', category: 'thinking', created_at: '2026-01-01T00:00:10Z' }],
      total: 1,
    })

    renderDetail(baseSelectedAgent)
    await waitFor(() => expect(screen.getByText('1 messages')).toBeInTheDocument())

    const tabs = screen.getAllByRole('tab')
    expect(tabs).toHaveLength(10)
    expect(tabs[9].textContent).toMatch(/Thinking/)
  })

  it('Thinking tab count reflects thinking messages', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'plain text', category: 'text', created_at: '2026-01-01T00:00:10Z' },
        { content: 'first thought', category: 'thinking', created_at: '2026-01-01T00:00:20Z' },
        { content: 'second thought', category: 'thinking', created_at: '2026-01-01T00:00:30Z' },
      ],
      total: 3,
    })

    renderDetail(baseSelectedAgent)
    await waitFor(() => expect(screen.getByText('3 messages')).toBeInTheDocument())

    const tabs = screen.getAllByRole('tab')
    expect(tabs[9].textContent).toContain('2')
  })

  it('Thinking tab count is zero when no thinking messages', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [{ content: 'just text', category: 'text', created_at: '2026-01-01T00:00:10Z' }],
      total: 1,
    })

    renderDetail(baseSelectedAgent)
    await waitFor(() => expect(screen.getByText('1 messages')).toBeInTheDocument())

    const tabs = screen.getAllByRole('tab')
    expect(tabs[9].textContent).toContain('0')
  })

  it('thinking row renders Thinking badge in Tool column', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [{ content: 'internal reasoning', category: 'thinking', created_at: '2026-01-01T00:00:10Z' }],
      total: 1,
    })

    renderDetail(baseSelectedAgent)
    await waitFor(() => expect(screen.getByText('1 messages')).toBeInTheDocument())

    const rows = document.querySelectorAll('[data-testid="message-row"]')
    expect(rows).toHaveLength(1)
    const toolCell = rows[0].querySelectorAll(':scope > td')[1]
    expect(within(toolCell as HTMLElement).getByText('Thinking')).toBeInTheDocument()
  })

  it('thinking row message cell has italic and muted styling', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [{ content: 'internal reasoning', category: 'thinking', created_at: '2026-01-01T00:00:10Z' }],
      total: 1,
    })

    renderDetail(baseSelectedAgent)
    await waitFor(() => expect(screen.getByText('1 messages')).toBeInTheDocument())

    const rows = document.querySelectorAll('[data-testid="message-row"]')
    expect(rows).toHaveLength(1)
    const messageCell = rows[0].querySelectorAll(':scope > td')[2]
    expect(messageCell.className).toContain('italic')
    expect(messageCell.className).toContain('text-muted-foreground')
  })

  it('clicking Thinking tab filters to only thinking messages', async () => {
    const user = userEvent.setup()
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'plain text', category: 'text', created_at: '2026-01-01T00:00:10Z' },
        { content: '[Bash] git status', category: 'tool', created_at: '2026-01-01T00:00:20Z' },
        { content: 'my inner thought', category: 'thinking', created_at: '2026-01-01T00:00:30Z' },
      ],
      total: 3,
    })

    renderDetail(baseSelectedAgent)
    await waitFor(() => expect(screen.getByText('3 messages')).toBeInTheDocument())

    await user.click(screen.getByRole('tab', { name: /Thinking/ }))

    await waitFor(() => expect(screen.getByText('1 of 3 messages')).toBeInTheDocument())

    expect(screen.getByText('my inner thought')).toBeInTheDocument()
    expect(screen.queryByText('plain text')).not.toBeInTheDocument()
    expect(screen.queryByText('git status')).not.toBeInTheDocument()
  })

  it('All tab still includes thinking messages', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'plain text', category: 'text', created_at: '2026-01-01T00:00:10Z' },
        { content: 'a thought', category: 'thinking', created_at: '2026-01-01T00:00:20Z' },
      ],
      total: 2,
    })

    renderDetail(baseSelectedAgent)
    await waitFor(() => expect(screen.getByText('2 messages')).toBeInTheDocument())

    const tabs = screen.getAllByRole('tab')
    expect(tabs[0].textContent).toContain('2')
    expect(screen.getByText('plain text')).toBeInTheDocument()
    expect(screen.getByText('a thought')).toBeInTheDocument()
  })

  it('Validation tab remains at index 8 (thinking appended last, not shifting validation)', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [{ content: 'hello', category: 'text', created_at: '2026-01-01T00:00:10Z' }],
      total: 1,
    })

    renderDetail(baseSelectedAgent)
    await waitFor(() => expect(screen.getByText('1 messages')).toBeInTheDocument())

    const tabs = screen.getAllByRole('tab')
    expect(tabs[8].textContent).toMatch(/Validation/)
    expect(tabs[9].textContent).toMatch(/Thinking/)
  })
})
