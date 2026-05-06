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
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
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

describe('AgentLogDetail - payload rendering', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders expandable payload section below message content when payload is present', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'ran ls', category: 'tool', created_at: '2026-01-01T00:00:10Z', payload: { cmd: 'ls', exit: 0 } },
      ],
      total: 1,
    })
    renderDetail(baseSelectedAgent)
    await waitFor(() => expect(screen.getByText('1 messages')).toBeInTheDocument())
    expect(screen.getByText('payload')).toBeInTheDocument()
    const pre = document.querySelector('pre')
    expect(pre).toBeInTheDocument()
    expect(pre!.textContent).toContain('"cmd"')
    expect(pre!.textContent).toContain('"ls"')
  })

  it('does not render payload section when payload is absent', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [{ content: 'no payload', category: 'text', created_at: '2026-01-01T00:00:10Z' }],
      total: 1,
    })
    renderDetail(baseSelectedAgent)
    await waitFor(() => expect(screen.getByText('1 messages')).toBeInTheDocument())
    expect(screen.queryByText('payload')).not.toBeInTheDocument()
  })
})

describe('AgentLogDetail - error and result category tabs', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows Errors and Results tabs in the category tablist', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [{ content: 'hello', category: 'text', created_at: '2026-01-01T00:00:10Z' }],
      total: 1,
    })

    renderDetail(baseSelectedAgent)

    await waitFor(() => {
      expect(screen.getByText('1 messages')).toBeInTheDocument()
    })

    expect(screen.getByRole('tab', { name: /Errors/ })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /Results/ })).toBeInTheDocument()
  })

  it('counts error and result messages in their respective tab badges', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'text msg', category: 'text', created_at: '2026-01-01T00:00:10Z' },
        { content: 'error occurred', category: 'error', created_at: '2026-01-01T00:00:20Z' },
        { content: 'another error', category: 'error', created_at: '2026-01-01T00:00:30Z' },
        { content: 'final result', category: 'result', created_at: '2026-01-01T00:00:40Z' },
      ],
      total: 4,
    })

    renderDetail(baseSelectedAgent)

    await waitFor(() => {
      expect(screen.getByText('4 messages')).toBeInTheDocument()
    })

    const tabs = screen.getAllByRole('tab')
    // tabs[6] = Errors, tabs[7] = Results
    expect(tabs[6].textContent).toContain('2') // Errors: 2
    expect(tabs[7].textContent).toContain('1') // Results: 1
  })

  it('filtering by Errors shows only error messages', async () => {
    const user = userEvent.setup()
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'plain text', category: 'text', created_at: '2026-01-01T00:00:10Z' },
        { content: 'error occurred', category: 'error', created_at: '2026-01-01T00:00:20Z' },
        { content: 'final result', category: 'result', created_at: '2026-01-01T00:00:30Z' },
      ],
      total: 3,
    })

    renderDetail(baseSelectedAgent)

    await waitFor(() => {
      expect(screen.getByText('3 messages')).toBeInTheDocument()
    })

    await user.click(screen.getByRole('tab', { name: /Errors/ }))

    await waitFor(() => {
      expect(screen.getByText('1 of 3 messages')).toBeInTheDocument()
    })

    expect(screen.getByText('error occurred')).toBeInTheDocument()
    expect(screen.queryByText('plain text')).not.toBeInTheDocument()
    expect(screen.queryByText('final result')).not.toBeInTheDocument()
  })

  it('filtering by Results shows only result messages', async () => {
    const user = userEvent.setup()
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'plain text', category: 'text', created_at: '2026-01-01T00:00:10Z' },
        { content: 'error occurred', category: 'error', created_at: '2026-01-01T00:00:20Z' },
        { content: 'final result', category: 'result', created_at: '2026-01-01T00:00:30Z' },
      ],
      total: 3,
    })

    renderDetail(baseSelectedAgent)

    await waitFor(() => {
      expect(screen.getByText('3 messages')).toBeInTheDocument()
    })

    await user.click(screen.getByRole('tab', { name: /Results/ }))

    await waitFor(() => {
      expect(screen.getByText('1 of 3 messages')).toBeInTheDocument()
    })

    expect(screen.getByText('final result')).toBeInTheDocument()
    expect(screen.queryByText('plain text')).not.toBeInTheDocument()
    expect(screen.queryByText('error occurred')).not.toBeInTheDocument()
  })

  it('error message row has Error badge in Tool column', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'something failed', category: 'error', created_at: '2026-01-01T00:00:10Z' },
      ],
      total: 1,
    })

    renderDetail(baseSelectedAgent)

    await waitFor(() => {
      expect(screen.getByText('1 messages')).toBeInTheDocument()
    })

    const rows = document.querySelectorAll('[data-testid="message-row"]')
    expect(rows).toHaveLength(1)
    const toolCell = rows[0].querySelectorAll(':scope > td')[1]
    expect(within(toolCell as HTMLElement).getByText('Error')).toBeInTheDocument()
  })

  it('result message row has Result badge in Tool column', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'task done', category: 'result', created_at: '2026-01-01T00:00:10Z' },
      ],
      total: 1,
    })

    renderDetail(baseSelectedAgent)

    await waitFor(() => {
      expect(screen.getByText('1 messages')).toBeInTheDocument()
    })

    const rows = document.querySelectorAll('[data-testid="message-row"]')
    expect(rows).toHaveLength(1)
    const toolCell = rows[0].querySelectorAll(':scope > td')[1]
    expect(within(toolCell as HTMLElement).getByText('Result')).toBeInTheDocument()
  })

  it('error row has red left-rail styling', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'error msg', category: 'error', created_at: '2026-01-01T00:00:10Z' },
      ],
      total: 1,
    })

    renderDetail(baseSelectedAgent)

    await waitFor(() => {
      expect(screen.getByText('1 messages')).toBeInTheDocument()
    })

    const rows = document.querySelectorAll('[data-testid="message-row"]')
    expect(rows[0].className).toContain('border-l-destructive')
  })

  it('result row has emerald left-rail styling', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'result msg', category: 'result', created_at: '2026-01-01T00:00:10Z' },
      ],
      total: 1,
    })

    renderDetail(baseSelectedAgent)

    await waitFor(() => {
      expect(screen.getByText('1 messages')).toBeInTheDocument()
    })

    const rows = document.querySelectorAll('[data-testid="message-row"]')
    expect(rows[0].className).toContain('border-l-emerald-500')
  })

  it('error/result counts are zero when no such messages exist', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'just text', category: 'text', created_at: '2026-01-01T00:00:10Z' },
      ],
      total: 1,
    })

    renderDetail(baseSelectedAgent)

    await waitFor(() => {
      expect(screen.getByText('1 messages')).toBeInTheDocument()
    })

    const tabs = screen.getAllByRole('tab')
    expect(tabs[6].textContent).toContain('0') // Errors: 0
    expect(tabs[7].textContent).toContain('0') // Results: 0
  })
})
