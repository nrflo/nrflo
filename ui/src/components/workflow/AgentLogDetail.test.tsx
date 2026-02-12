import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AgentLogDetail } from './AgentLogDetail'
import * as ticketsApi from '@/api/tickets'
import type { SelectedAgentData } from './PhaseGraph/types'
import type { ActiveAgentV4, AgentHistoryEntry, AgentSession } from '@/types/workflow'

// jsdom doesn't implement scrollIntoView
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
    raw_output_size: 2048,
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
    agent_type: 'setup-analyzer',
    phase: 'investigation',
    model_id: 'claude-sonnet-4-5',
    result: 'pass',
    duration_sec: 120,
    started_at: '2026-01-01T00:00:00Z',
    ended_at: '2026-01-01T00:02:00Z',
    ...overrides,
  }
}

function renderDetail(
  selectedAgent: SelectedAgentData,
  onBack = vi.fn(),
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return {
    ...render(
      <QueryClientProvider client={queryClient}>
        <AgentLogDetail selectedAgent={selectedAgent} onBack={onBack} />
      </QueryClientProvider>
    ),
    onBack,
  }
}

describe('AgentLogDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        { content: 'Setting up project...', created_at: '2026-01-01T00:00:10Z' },
        { content: 'Installing deps...', created_at: '2026-01-01T00:00:20Z' },
        { content: 'Running build...', created_at: '2026-01-01T00:00:30Z' },
      ],
      total: 3,
    })
  })

  describe('header and status display', () => {
    it('shows phase name and model in header', async () => {
      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession(),
      })

      expect(screen.getByText('implementation')).toBeInTheDocument()
      expect(screen.getByText('4-5')).toBeInTheDocument()
    })

    it('shows pass badge for completed agent', async () => {
      renderDetail({
        phaseName: 'investigation',
        historyEntry: makeHistoryEntry({ result: 'pass' }),
        session: makeSession({ id: 'session-2', status: 'completed' }),
      })

      expect(screen.getByText('pass')).toBeInTheDocument()
    })

    it('shows fail badge for failed agent', () => {
      renderDetail({
        phaseName: 'verification',
        historyEntry: makeHistoryEntry({ result: 'fail' }),
        session: makeSession({ id: 'session-3', status: 'failed' }),
      })

      expect(screen.getByText('fail')).toBeInTheDocument()
    })

    it('shows duration for completed agent', () => {
      renderDetail({
        phaseName: 'investigation',
        historyEntry: makeHistoryEntry({ duration_sec: 120 }),
        session: makeSession({ status: 'completed' }),
      })

      expect(screen.getByText('2m')).toBeInTheDocument()
    })

    it('calls onBack when back button is clicked', async () => {
      const user = userEvent.setup()
      const onBack = vi.fn()

      renderDetail(
        {
          phaseName: 'implementation',
          agent: makeRunningAgent(),
          session: makeSession(),
        },
        onBack,
      )

      const buttons = screen.getAllByRole('button')
      await user.click(buttons[0])
      expect(onBack).toHaveBeenCalledTimes(1)
    })
  })

  describe('messages display', () => {
    it('shows messages in a table with newest first', async () => {
      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession(),
      })

      await waitFor(() => {
        expect(screen.getByText('3 messages')).toBeInTheDocument()
      })

      // Table should exist
      const table = document.querySelector('table')
      expect(table).toBeInTheDocument()

      // Table headers
      expect(screen.getByText('Time')).toBeInTheDocument()
      expect(screen.getByText('Tool')).toBeInTheDocument()
      expect(screen.getByText('Message')).toBeInTheDocument()

      // Messages content
      expect(screen.getByText('Setting up project...')).toBeInTheDocument()
      expect(screen.getByText('Installing deps...')).toBeInTheDocument()
      expect(screen.getByText('Running build...')).toBeInTheDocument()
    })

    it('shows empty state when no messages', async () => {
      vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
        session_id: 'session-1',
        messages: [],
        total: 0,
      })

      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession(),
      })

      await waitFor(() => {
        expect(screen.getByText('No messages available')).toBeInTheDocument()
      })
    })

    it('shows loading state while messages are being fetched', () => {
      vi.mocked(ticketsApi.getSessionMessages).mockReturnValue(new Promise(() => {}))

      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession(),
      })

      expect(screen.getByText('Loading messages...')).toBeInTheDocument()
    })
  })

  describe('acceptance criteria: no tooltip, table layout, no toggle', () => {
    it('renders message table with tool badges and timestamps, no tooltip, no raw/messages toggle', async () => {
      vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
        session_id: 'session-1',
        messages: [
          { content: '[Bash] git status', created_at: '2026-01-01T00:01:00Z' },
          { content: '[Read] src/main.ts', created_at: '2026-01-01T00:01:10Z' },
          { content: '[Edit] src/utils.ts', created_at: '2026-01-01T00:01:20Z' },
          { content: 'plain text message', created_at: '2026-01-01T00:01:30Z' },
        ],
        total: 4,
      })

      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession(),
      })

      await waitFor(() => {
        expect(screen.getByText('4 messages')).toBeInTheDocument()
      })

      // --- Criterion 1: No tooltip ---
      // Tooltip component was deleted. Verify no tooltip-related attributes exist.
      const container = document.querySelector('.flex.flex-col.h-full')!
      expect(container.querySelector('[role="tooltip"]')).toBeNull()
      expect(container.querySelector('[data-tooltip]')).toBeNull()

      // --- Criterion 2: Table with timestamp|tool|message structure ---
      const table = document.querySelector('table')!
      expect(table).toBeInTheDocument()

      // Verify table header structure
      const thead = table.querySelector('thead')!
      const headerCells = thead.querySelectorAll('th')
      expect(headerCells).toHaveLength(3)
      expect(headerCells[0].textContent).toBe('Time')
      expect(headerCells[1].textContent).toBe('Tool')
      expect(headerCells[2].textContent).toBe('Message')

      // Verify table body has correct number of rows
      const tbody = table.querySelector('tbody')!
      const rows = tbody.querySelectorAll('tr')
      expect(rows).toHaveLength(4)

      // Messages are reversed (newest first), so last message in data is first row
      const firstRow = rows[0]
      const firstRowCells = firstRow.querySelectorAll('td')
      expect(firstRowCells).toHaveLength(3)
      // Timestamp column (td[0]) has time text
      expect(firstRowCells[0].textContent).toBeTruthy()
      // No tool for plain text message
      expect(firstRowCells[1].querySelector('span')).toBeNull()
      // Message column
      expect(firstRowCells[2].textContent).toBe('plain text message')

      // Second row (third message in reversed order) should have [Edit] tool badge
      const secondRow = rows[1]
      const secondRowCells = secondRow.querySelectorAll('td')
      expect(within(secondRowCells[1]).getByText('Edit')).toBeInTheDocument()
      expect(secondRowCells[2].textContent).toBe('src/utils.ts')

      // Third row should have [Read] tool badge
      const thirdRow = rows[2]
      const thirdRowCells = thirdRow.querySelectorAll('td')
      expect(within(thirdRowCells[1]).getByText('Read')).toBeInTheDocument()
      expect(thirdRowCells[2].textContent).toBe('src/main.ts')

      // Fourth row (first message) should have [Bash] tool badge
      const fourthRow = rows[3]
      const fourthRowCells = fourthRow.querySelectorAll('td')
      expect(within(fourthRowCells[1]).getByText('Bash')).toBeInTheDocument()
      expect(fourthRowCells[2].textContent).toBe('git status')

      // --- Criterion 3: No messages/raw toggle ---
      // There should be no toggle, tab, or button for switching between messages and raw
      expect(screen.queryByText('Messages')).not.toBeInTheDocument()
      expect(screen.queryByText('Raw')).not.toBeInTheDocument()
      expect(screen.queryByText('Raw Output')).not.toBeInTheDocument()
      expect(screen.queryByRole('tab')).toBeNull()

      // Messages are always displayed directly (the table IS the default and only view)
      expect(table).toBeVisible()
    })
  })

  describe('table tool badge rendering', () => {
    it('renders colored tool badges in the tool column for known tools', async () => {
      vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
        session_id: 'session-1',
        messages: [
          { content: '[Grep] pattern in files', created_at: '2026-01-01T00:00:01Z' },
          { content: '[WebFetch] https://example.com', created_at: '2026-01-01T00:00:02Z' },
          { content: '[Task] codebase-explorer: search', created_at: '2026-01-01T00:00:03Z' },
        ],
        total: 3,
      })

      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession(),
      })

      await waitFor(() => {
        expect(screen.getByText('Grep')).toBeInTheDocument()
      })

      expect(screen.getByText('WebFetch')).toBeInTheDocument()
      expect(screen.getByText('Task')).toBeInTheDocument()

      // Verify the tool badges are in td elements (table cells)
      const grepBadge = screen.getByText('Grep')
      expect(grepBadge.closest('td')).toBeInTheDocument()
    })

    it('leaves tool column empty for messages without tool prefix', async () => {
      vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
        session_id: 'session-1',
        messages: [
          { content: 'no tool prefix here', created_at: '2026-01-01T00:00:01Z' },
        ],
        total: 1,
      })

      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession(),
      })

      await waitFor(() => {
        expect(screen.getByText('no tool prefix here')).toBeInTheDocument()
      })

      const tbody = document.querySelector('tbody')!
      const toolCell = tbody.querySelector('tr td:nth-child(2)')!
      // Tool cell should have no badge child
      expect(toolCell.children).toHaveLength(0)
    })
  })

  describe('table timestamp rendering', () => {
    it('renders formatted timestamps in the time column', async () => {
      vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
        session_id: 'session-1',
        messages: [
          { content: 'msg1', created_at: '2026-01-15T14:30:45Z' },
        ],
        total: 1,
      })

      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession(),
      })

      await waitFor(() => {
        expect(screen.getByText('1 messages')).toBeInTheDocument()
      })

      // The time cell should contain a formatted HH:MM:SS timestamp
      const tbody = document.querySelector('tbody')!
      const timeCell = tbody.querySelector('tr td:first-child')!
      // formatTime produces locale-dependent output, but it should be non-empty
      expect(timeCell.textContent).toBeTruthy()
      // Should contain at least digits and colons (HH:MM:SS pattern)
      expect(timeCell.textContent).toMatch(/\d/)
    })

    it('handles empty created_at gracefully', async () => {
      vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
        session_id: 'session-1',
        messages: [
          { content: 'msg without time', created_at: '' },
        ],
        total: 1,
      })

      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession(),
      })

      await waitFor(() => {
        expect(screen.getByText('msg without time')).toBeInTheDocument()
      })

      // Should not crash, time cell should be empty
      const tbody = document.querySelector('tbody')!
      const timeCell = tbody.querySelector('tr td:first-child')!
      expect(timeCell.textContent).toBe('')
    })
  })

  describe('message order', () => {
    it('reverses messages so newest appears first in the table', async () => {
      vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
        session_id: 'session-1',
        messages: [
          { content: 'first message', created_at: '2026-01-01T00:00:01Z' },
          { content: 'second message', created_at: '2026-01-01T00:00:02Z' },
          { content: 'third message', created_at: '2026-01-01T00:00:03Z' },
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

      const tbody = document.querySelector('tbody')!
      const rows = tbody.querySelectorAll('tr')
      const msgCells = Array.from(rows).map(r => r.querySelector('td:nth-child(3)')!.textContent)

      // Reversed: newest first
      expect(msgCells[0]).toBe('third message')
      expect(msgCells[1]).toBe('second message')
      expect(msgCells[2]).toBe('first message')
    })
  })

  describe('ticket nrworkflow-720aec: no auto-scroll on message updates', () => {
    it('does NOT call scrollIntoView when component renders with messages', async () => {
      const scrollIntoViewMock = Element.prototype.scrollIntoView as ReturnType<typeof vi.fn>
      scrollIntoViewMock.mockClear()

      vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
        session_id: 'session-1',
        messages: [
          { content: 'msg1', created_at: '2026-01-01T00:00:01Z' },
          { content: 'msg2', created_at: '2026-01-01T00:00:02Z' },
          { content: 'msg3', created_at: '2026-01-01T00:00:03Z' },
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

      // Verify scrollIntoView was NOT called at all during render
      expect(scrollIntoViewMock).not.toHaveBeenCalled()
    })

    it('does NOT auto-scroll even for running agents', async () => {
      const scrollIntoViewMock = Element.prototype.scrollIntoView as ReturnType<typeof vi.fn>
      scrollIntoViewMock.mockClear()

      vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
        session_id: 'session-1',
        messages: [
          { content: 'Building...', created_at: '2026-01-01T00:00:01Z' },
          { content: 'Testing...', created_at: '2026-01-01T00:00:05Z' },
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

      // Verify scrollIntoView was NOT called for running agent
      expect(scrollIntoViewMock).not.toHaveBeenCalled()
    })

    it('no ref attribute exists on messages container after auto-scroll removal', async () => {
      vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
        session_id: 'session-1',
        messages: [
          { content: 'message', created_at: '2026-01-01T00:00:01Z' },
        ],
        total: 1,
      })

      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession(),
      })

      await waitFor(() => {
        expect(screen.getByText('1 messages')).toBeInTheDocument()
      })

      // The old code had <div ref={messagesStartRef} /> before the message count
      // Verify this ref div no longer exists
      const contentArea = document.querySelector('.flex-1.overflow-y-auto')
      expect(contentArea).toBeInTheDocument()

      // There should be no empty div with a ref right before the message count
      const messageCountContainer = screen.getByText('1 messages').closest('div')
      const previousSibling = messageCountContainer?.previousElementSibling
      // The previous sibling should NOT be an empty div (it should be the table or nothing)
      if (previousSibling) {
        // If there is a previous sibling, it should have content (be the table)
        expect(previousSibling.tagName.toLowerCase()).toBe('table')
      }
    })
  })
})
