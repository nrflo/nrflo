import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
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

describe('AgentLogDetail - tab count badges', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows correct count in each tab badge for mixed-category messages', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: '[Bash] git status', category: 'tool', created_at: '2026-01-01T00:00:10Z' },
        { content: '[Read] file.ts', category: 'tool', created_at: '2026-01-01T00:00:11Z' },
        { content: 'plain text 1', category: 'text', created_at: '2026-01-01T00:00:20Z' },
        { content: '[Task] sub-agent work', category: 'subagent', created_at: '2026-01-01T00:00:30Z' },
        { content: '[TaskResult] done', category: 'subagent', created_at: '2026-01-01T00:00:31Z' },
        { content: '[Skill] my-skill', category: 'skill', created_at: '2026-01-01T00:00:40Z' },
      ],
      total: 6,
    })

    renderDetail({
      phaseName: 'implementation',
      agent: makeRunningAgent(),
      session: makeSession(),
    })

    await waitFor(() => {
      expect(screen.getByText('6 messages')).toBeInTheDocument()
    })

    // Each tab button contains a count badge span — verify via textContent
    const tabs = screen.getAllByRole('tab')
    expect(tabs).toHaveLength(5)

    // All tab: total count = 6
    expect(tabs[0].textContent).toContain('6')
    // Text tab: 1 text message
    expect(tabs[1].textContent).toContain('1')
    // Tools tab: 2 tool messages
    expect(tabs[2].textContent).toContain('2')
    // Sub-agents tab: 2 subagent messages
    expect(tabs[3].textContent).toContain('2')
    // Skills tab: 1 skill message
    expect(tabs[4].textContent).toContain('1')
  })

  it('messages without category field are counted in Text tab (fallback)', async () => {
    // Simulate old messages that may arrive from the API without a category field
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        { content: 'no category field', created_at: '2026-01-01T00:00:10Z' } as any,
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        { content: 'also no category', created_at: '2026-01-01T00:00:20Z' } as any,
        { content: '[Bash] tool msg', category: 'tool', created_at: '2026-01-01T00:00:30Z' },
      ],
      total: 3,
    })

    renderDetail({
      phaseName: 'implementation',
      agent: makeRunningAgent(),
      session: makeSession(),
    })

    await waitFor(() => {
      expect(screen.getByText('3 messages')).toBeInTheDocument()
    })

    const tabs = screen.getAllByRole('tab')
    // All tab shows total 3
    expect(tabs[0].textContent).toContain('3')
    // Text tab shows 2 (the two messages without category default to 'text')
    expect(tabs[1].textContent).toContain('2')
    // Tools tab shows 1
    expect(tabs[2].textContent).toContain('1')
    // Sub-agents tab shows 0
    expect(tabs[3].textContent).toContain('0')
    // Skills tab shows 0
    expect(tabs[4].textContent).toContain('0')
  })

  it('filters messages by category=text also respects fallback for uncategorized messages', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        { content: 'legacy message', created_at: '2026-01-01T00:00:10Z' } as any,
        { content: 'explicit text', category: 'text', created_at: '2026-01-01T00:00:20Z' },
        { content: '[Bash] tool', category: 'tool', created_at: '2026-01-01T00:00:30Z' },
      ],
      total: 3,
    })

    renderDetail({
      phaseName: 'implementation',
      agent: makeRunningAgent(),
      session: makeSession(),
    })

    await waitFor(() => {
      expect(screen.getByText('3 messages')).toBeInTheDocument()
    })

    // Text tab should count both explicit 'text' and missing-category messages
    const tabs = screen.getAllByRole('tab')
    expect(tabs[1].textContent).toContain('2') // Text: 2 (1 legacy + 1 explicit text)
    expect(tabs[2].textContent).toContain('1') // Tools: 1
  })

  it('shows zero counts on category tabs when all messages are text', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'text only msg', category: 'text', created_at: '2026-01-01T00:00:10Z' },
        { content: 'another text msg', category: 'text', created_at: '2026-01-01T00:00:20Z' },
      ],
      total: 2,
    })

    renderDetail({
      phaseName: 'implementation',
      agent: makeRunningAgent(),
      session: makeSession(),
    })

    await waitFor(() => {
      expect(screen.getByText('2 messages')).toBeInTheDocument()
    })

    const tabs = screen.getAllByRole('tab')
    // All: 2, Text: 2, Tools: 0, Sub-agents: 0, Skills: 0
    expect(tabs[0].textContent).toContain('2')
    expect(tabs[1].textContent).toContain('2')
    expect(tabs[2].textContent).toContain('0')
    expect(tabs[3].textContent).toContain('0')
    expect(tabs[4].textContent).toContain('0')
  })
})
