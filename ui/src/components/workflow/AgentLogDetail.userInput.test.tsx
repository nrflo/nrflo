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

const MIXED_MESSAGES = [
  { content: 'text line', category: 'text' as const, created_at: '2026-01-01T00:00:10Z' },
  { content: 'user typed this', category: 'user_input' as const, created_at: '2026-01-01T00:00:20Z' },
  { content: 'another user message', category: 'user_input' as const, created_at: '2026-01-01T00:00:30Z' },
  { content: '[Bash] git status', category: 'tool' as const, created_at: '2026-01-01T00:00:40Z' },
]

describe('AgentLogDetail - User input tab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders User input tab with correct count', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: MIXED_MESSAGES,
      total: 4,
    })

    renderDetail({ phaseName: 'implementation', agent: makeRunningAgent(), session: makeSession() })

    await waitFor(() => {
      expect(screen.getByText('4 messages')).toBeInTheDocument()
    })

    const userInputTab = screen.getByRole('tab', { name: /User input/ })
    expect(userInputTab).toBeInTheDocument()
    expect(userInputTab.textContent).toContain('2')
  })

  it('clicking User input tab shows only user_input rows', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: MIXED_MESSAGES,
      total: 4,
    })

    const user = userEvent.setup()
    renderDetail({ phaseName: 'implementation', agent: makeRunningAgent(), session: makeSession() })

    await waitFor(() => {
      expect(screen.getByText('4 messages')).toBeInTheDocument()
    })

    await user.click(screen.getByRole('tab', { name: /User input/ }))

    // Shows filtered count
    expect(screen.getByText('2 of 4 messages')).toBeInTheDocument()

    // Only user_input messages visible
    const rows = screen.getAllByTestId('message-row')
    expect(rows).toHaveLength(2)
    expect(within(rows[0]).getByText('User')).toBeInTheDocument()
    expect(within(rows[1]).getByText('User')).toBeInTheDocument()

    // Non-user_input content not visible as separate rows
    expect(screen.queryByText('text line')).not.toBeInTheDocument()
    expect(screen.queryByText('[Bash] git status')).not.toBeInTheDocument()
  })

  it('All tab includes user_input messages among total', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: MIXED_MESSAGES,
      total: 4,
    })

    renderDetail({ phaseName: 'implementation', agent: makeRunningAgent(), session: makeSession() })

    await waitFor(() => {
      expect(screen.getByText('4 messages')).toBeInTheDocument()
    })

    // Default tab is All — all 4 rows are shown
    const rows = screen.getAllByTestId('message-row')
    expect(rows).toHaveLength(4)

    // All tab shows total count = 4
    const allTab = screen.getByRole('tab', { name: /^All/ })
    expect(allTab.textContent).toContain('4')
  })

  it('User input tab is selected and marked aria-selected after click', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: MIXED_MESSAGES,
      total: 4,
    })

    const user = userEvent.setup()
    renderDetail({ phaseName: 'implementation', agent: makeRunningAgent(), session: makeSession() })

    await waitFor(() => {
      expect(screen.getByText('4 messages')).toBeInTheDocument()
    })

    const userInputTab = screen.getByRole('tab', { name: /User input/ })
    expect(userInputTab).toHaveAttribute('aria-selected', 'false')

    await user.click(userInputTab)

    expect(userInputTab).toHaveAttribute('aria-selected', 'true')
  })

  it('user_input rows have left-rail border accent in table', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'plain text', category: 'text' as const, created_at: '2026-01-01T00:00:10Z' },
        { content: 'user input row', category: 'user_input' as const, created_at: '2026-01-01T00:00:20Z' },
      ],
      total: 2,
    })

    renderDetail({ phaseName: 'implementation', agent: makeRunningAgent(), session: makeSession() })

    await waitFor(() => {
      expect(screen.getByText('2 messages')).toBeInTheDocument()
    })

    const rows = screen.getAllByTestId('message-row')
    expect(rows).toHaveLength(2)

    // Rows are reversed (newest first), so row[0] = user_input, row[1] = plain text
    expect(rows[0].className).toContain('border-l-primary')
    expect(rows[1].className).not.toContain('border-l-primary')
  })
})
