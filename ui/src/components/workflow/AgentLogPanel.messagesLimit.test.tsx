import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AgentLogPanel } from './AgentLogPanel'
import type { ActiveAgentV4, AgentSession, MessageWithTime } from '@/types/workflow'

// Create a module-scoped variable to hold mock data
let mockMessages: MessageWithTime[] = []

// Mock useSessionMessages to return controlled test data
vi.mock('@/hooks/useTickets', () => ({
  useSessionMessages: () => ({
    data: {
      session_id: 'session-1',
      messages: mockMessages,
      total: mockMessages.length,
    },
    isLoading: false,
  }),
}))

// Mock AgentLogDetail to avoid deep dependencies
vi.mock('./AgentLogDetail', async () => {
  const actual = await vi.importActual<typeof import('./AgentLogDetail')>('./AgentLogDetail')
  return {
    ...actual,
    AgentLogDetail: () => <div data-testid="agent-log-detail">Detail View</div>,
  }
})

function makeAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
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

function makeMessages(count: number): MessageWithTime[] {
  return Array.from({ length: count }, (_, i) => ({
    content: `Message ${i + 1}`,
    category: 'text' as const,
    created_at: `2026-01-01T00:${String(i + 1).padStart(2, '0')}:00Z`,
  }))
}

function renderPanel(
  messages: MessageWithTime[],
  props: Partial<React.ComponentProps<typeof AgentLogPanel>> = {}
) {
  // Set the mock messages before rendering
  mockMessages = messages

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  const defaultProps = {
    activeAgents: { 'implementor:claude:sonnet': makeAgent() },
    sessions: [makeSession()],
    collapsed: false,
    onToggleCollapse: vi.fn(),
    selectedAgent: null,
    onAgentSelect: vi.fn(),
    ...props,
  }

  return {
    ...render(
      <QueryClientProvider client={queryClient}>
        <AgentLogPanel {...defaultProps} />
      </QueryClientProvider>
    ),
    props: defaultProps,
  }
}

describe('AgentLogPanel - Messages Limit (nrworkflow-1b9198)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockMessages = [] // Reset mock data between tests
  })

  describe('displayMessages slice behavior', () => {
    it('shows all messages when count is less than 20', () => {
      const messages = makeMessages(10)
      renderPanel(messages)

      const table = document.querySelector('table')
      expect(table).toBeInTheDocument()
      const tbody = table!.querySelector('tbody')!
      const rows = tbody.querySelectorAll('tr')

      // All 10 messages should be displayed
      expect(rows).toHaveLength(10)

      // First row should be the NEWEST message (Message 10)
      const firstRow = rows[0].querySelector('td:nth-child(3)')!
      expect(firstRow.textContent).toBe('Message 10')

      // Last row should be the OLDEST message (Message 1)
      const lastRow = rows[9].querySelector('td:nth-child(3)')!
      expect(lastRow.textContent).toBe('Message 1')
    })

    it('shows exactly 20 messages when count equals 20', () => {
      const messages = makeMessages(20)
      renderPanel(messages)

      const table = document.querySelector('table')
      const tbody = table!.querySelector('tbody')!
      const rows = tbody.querySelectorAll('tr')

      expect(rows).toHaveLength(20)

      // Newest first (Message 20 to Message 1)
      expect(rows[0].querySelector('td:nth-child(3)')!.textContent).toBe('Message 20')
      expect(rows[19].querySelector('td:nth-child(3)')!.textContent).toBe('Message 1')
    })

    it('shows only the LAST 20 messages when count exceeds 20 (newest first)', () => {
      const messages = makeMessages(50)
      renderPanel(messages)

      const table = document.querySelector('table')
      const tbody = table!.querySelector('tbody')!
      const rows = tbody.querySelectorAll('tr')

      // Only 20 rows should be displayed
      expect(rows).toHaveLength(20)

      // First row should be Message 50 (newest)
      const firstRow = rows[0].querySelector('td:nth-child(3)')!
      expect(firstRow.textContent).toBe('Message 50')

      // Last row should be Message 31 (20th from the end)
      const lastRow = rows[19].querySelector('td:nth-child(3)')!
      expect(lastRow.textContent).toBe('Message 31')

      // Messages 1-30 should NOT be displayed
      const allText = tbody.textContent
      expect(allText).not.toContain('Message 1')
      expect(allText).not.toContain('Message 30')
    })

    it('shows the LAST 20 messages with 100 messages (realistic backend limit)', () => {
      const messages = makeMessages(100)
      renderPanel(messages)

      const table = document.querySelector('table')
      const tbody = table!.querySelector('tbody')!
      const rows = tbody.querySelectorAll('tr')

      expect(rows).toHaveLength(20)

      // Should show Message 100 down to Message 81
      expect(rows[0].querySelector('td:nth-child(3)')!.textContent).toBe('Message 100')
      expect(rows[19].querySelector('td:nth-child(3)')!.textContent).toBe('Message 81')

      // Verify old messages are NOT shown (check cell content exactly to avoid substring matches)
      const displayedMessages = Array.from(rows).map(
        r => r.querySelector('td:nth-child(3)')!.textContent
      )
      expect(displayedMessages).not.toContain('Message 1')
      expect(displayedMessages).not.toContain('Message 50')
      expect(displayedMessages).not.toContain('Message 80')
    })

    it('messages are in reverse chronological order (newest first)', () => {
      const messages = makeMessages(30)
      renderPanel(messages)

      const table = document.querySelector('table')
      const tbody = table!.querySelector('tbody')!
      const rows = tbody.querySelectorAll('tr')

      const displayedMessages = Array.from(rows).map(
        r => r.querySelector('td:nth-child(3)')!.textContent
      )

      // Verify order is 30, 29, 28, ..., 11
      expect(displayedMessages).toEqual([
        'Message 30', 'Message 29', 'Message 28', 'Message 27', 'Message 26',
        'Message 25', 'Message 24', 'Message 23', 'Message 22', 'Message 21',
        'Message 20', 'Message 19', 'Message 18', 'Message 17', 'Message 16',
        'Message 15', 'Message 14', 'Message 13', 'Message 12', 'Message 11',
      ])
    })

    it('handles empty messages array gracefully', () => {
      renderPanel([])

      // Should render the agent block but no table (no messages)
      const table = document.querySelector('table')
      expect(table).not.toBeInTheDocument()

      // Agent block should still be present (check via text content)
      const body = document.body
      expect(body.textContent).toMatch(/implementation/i)
    })

    it('handles single message correctly', () => {
      const messages = makeMessages(1)
      renderPanel(messages)

      const table = document.querySelector('table')
      const tbody = table!.querySelector('tbody')!
      const rows = tbody.querySelectorAll('tr')

      expect(rows).toHaveLength(1)
      expect(rows[0].querySelector('td:nth-child(3)')!.textContent).toBe('Message 1')
    })
  })

  describe('regression test: old bug behavior', () => {
    it('DOES NOT show the oldest 20 messages when there are more than 20', () => {
      const messages = makeMessages(50)
      renderPanel(messages)

      const tbody = document.querySelector('tbody')!
      const allText = tbody.textContent

      // OLD BUG: .slice(0, 20) would show Message 1-20
      // NEW BEHAVIOR: .slice(-20) shows Message 31-50
      // Verify the old behavior is NOT present
      expect(allText).not.toContain('Message 1')
      expect(allText).not.toContain('Message 20')

      // Verify new behavior is correct
      expect(allText).toContain('Message 50')
      expect(allText).toContain('Message 31')
    })
  })

  describe('timestamps rendering', () => {
    it('renders timestamps in HH:MM:SS format in time column', () => {
      const messages: MessageWithTime[] = [
        { content: 'First', category: 'text', created_at: '2026-01-01T10:15:30Z' },
        { content: 'Second', category: 'text', created_at: '2026-01-01T10:15:45Z' },
      ]
      renderPanel(messages)

      const table = document.querySelector('table')
      const tbody = table!.querySelector('tbody')!
      const rows = tbody.querySelectorAll('tr')

      // Check time cells contain time format (HH:MM:SS)
      const firstTimeCell = rows[0].querySelector('td:first-child')!
      const secondTimeCell = rows[1].querySelector('td:first-child')!

      expect(firstTimeCell.textContent).toMatch(/\d{2}:\d{2}:\d{2}/)
      expect(secondTimeCell.textContent).toMatch(/\d{2}:\d{2}:\d{2}/)
    })
  })

  describe('boundary conditions', () => {
    it('handles exactly 19 messages (boundary below 20)', () => {
      const messages = makeMessages(19)
      renderPanel(messages)

      const tbody = document.querySelector('tbody')!
      const rows = tbody.querySelectorAll('tr')

      expect(rows).toHaveLength(19)
      expect(rows[0].querySelector('td:nth-child(3)')!.textContent).toBe('Message 19')
      expect(rows[18].querySelector('td:nth-child(3)')!.textContent).toBe('Message 1')
    })

    it('handles exactly 21 messages (boundary above 20)', () => {
      const messages = makeMessages(21)
      renderPanel(messages)

      const tbody = document.querySelector('tbody')!
      const rows = tbody.querySelectorAll('tr')

      expect(rows).toHaveLength(20)
      // Should show Message 21 down to Message 2 (last 20)
      expect(rows[0].querySelector('td:nth-child(3)')!.textContent).toBe('Message 21')
      expect(rows[19].querySelector('td:nth-child(3)')!.textContent).toBe('Message 2')

      // Message 1 should NOT be shown (check cell content exactly)
      const displayedMessages = Array.from(rows).map(
        r => r.querySelector('td:nth-child(3)')!.textContent
      )
      expect(displayedMessages).not.toContain('Message 1')
    })
  })
})
