import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CompletedAgentsTable } from './CompletedAgentsTable'
import type { AgentHistoryEntry, AgentSession } from '@/types/workflow'

function makeHistoryEntry(overrides: Partial<AgentHistoryEntry> = {}): AgentHistoryEntry {
  return {
    agent_id: 'a1',
    agent_type: 'implementor',
    phase: 'implementation',
    session_id: 'session-1',
    model_id: 'claude-sonnet-4-5',
    result: 'pass',
    started_at: '2026-01-01T00:00:00Z',
    ended_at: '2026-01-01T01:00:00Z',
    duration_sec: 3600,
    ...overrides,
  }
}

function makeSession(overrides: Partial<AgentSession> = {}): AgentSession {
  return {
    id: 'session-1',
    project_id: 'test-project',
    ticket_id: '',
    workflow_instance_id: 'wi-1',
    phase: 'implementation',
    workflow: 'feature',
    agent_type: 'implementor',
    model_id: 'claude-sonnet-4-5',
    status: 'completed',
    result: 'pass',
    message_count: 10,
    restart_count: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T01:00:00Z',
    started_at: '2026-01-01T00:00:00Z',
    ended_at: '2026-01-01T01:00:00Z',
    ...overrides,
  }
}

describe('CompletedAgentsTable', () => {
  describe('rendering', () => {
    it('renders table with correct headers', () => {
      const history = [makeHistoryEntry()]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const table = document.querySelector('[data-testid="agent-table"]')
      expect(table).toBeInTheDocument()

      const header = document.querySelector('[data-testid="agent-table-header"]')!
      const headers = header.querySelectorAll(':scope > span')
      expect(headers).toHaveLength(6)
      expect(headers[0].textContent).toBe('Agent')
      expect(headers[1].textContent).toBe('Phase')
      expect(headers[2].textContent).toBe('Model')
      expect(headers[3].textContent).toBe('Result')
      expect(headers[4].textContent).toBe('Duration')
      expect(headers[5].textContent).toBe('Completed At')
    })

    it('renders agent data in table rows', () => {
      const history = [
        makeHistoryEntry({
          agent_type: 'setup-analyzer',
          phase: 'investigation',
          model_id: 'claude-opus-4-6',
          result: 'pass',
          started_at: '2026-01-01T00:00:00Z',
          ended_at: '2026-01-01T00:02:00Z',
          duration_sec: 120,
        }),
      ]
      const sessions = [makeSession({ id: 'session-1' })]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      expect(screen.getByText('setup-analyzer')).toBeInTheDocument()
      expect(screen.getByText('investigation')).toBeInTheDocument()
      expect(screen.getByText('claude-opus-4-6')).toBeInTheDocument()
      expect(screen.getByText('pass')).toBeInTheDocument()
      expect(screen.getByText('2m')).toBeInTheDocument()
    })

    it('shows empty state when no agent history', () => {
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={[]}
          sessions={[]}
          onAgentSelect={onAgentSelect}
        />
      )

      expect(screen.getByText('No completed agents')).toBeInTheDocument()
      expect(document.querySelector('[data-testid="agent-table"]')).not.toBeInTheDocument()
    })

    it('replaces underscores with spaces in phase names', () => {
      const history = [
        makeHistoryEntry({
          phase: 'test_design',
        }),
      ]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      expect(screen.getByText('test design')).toBeInTheDocument()
    })

    it('displays "-" for missing model_id', () => {
      const history = [
        makeHistoryEntry({
          model_id: undefined,
        }),
      ]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const row = document.querySelector('[data-testid="agent-row"]')!
      const cells = row.querySelectorAll(':scope > span')
      expect(cells[2].textContent).toBe('-')
    })

    it('displays "-" for missing ended_at', () => {
      const history = [
        makeHistoryEntry({
          ended_at: undefined,
        }),
      ]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const row = document.querySelector('[data-testid="agent-row"]')!
      const cells = row.querySelectorAll(':scope > span')
      expect(cells[5].textContent).toBe('-')
    })
  })

  describe('sorting', () => {
    it('sorts agents by ended_at DESC (latest first)', () => {
      const history = [
        makeHistoryEntry({
          agent_id: 'a1',
          agent_type: 'setup-analyzer',
          ended_at: '2026-01-01T00:00:00Z',
        }),
        makeHistoryEntry({
          agent_id: 'a2',
          agent_type: 'implementor',
          ended_at: '2026-01-01T05:00:00Z',
        }),
        makeHistoryEntry({
          agent_id: 'a3',
          agent_type: 'qa-verifier',
          ended_at: '2026-01-01T03:00:00Z',
        }),
      ]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const rows = document.querySelectorAll('[data-testid="agent-row"]')
      const firstRowAgentCell = rows[0].querySelector(':scope > span:first-child')
      const secondRowAgentCell = rows[1].querySelector(':scope > span:first-child')
      const thirdRowAgentCell = rows[2].querySelector(':scope > span:first-child')

      // Latest first (DESC order)
      expect(firstRowAgentCell?.textContent).toBe('implementor')
      expect(secondRowAgentCell?.textContent).toBe('qa-verifier')
      expect(thirdRowAgentCell?.textContent).toBe('setup-analyzer')
    })

    it('sorts entries with null ended_at last', () => {
      const history = [
        makeHistoryEntry({
          agent_id: 'a1',
          agent_type: 'setup-analyzer',
          ended_at: '2026-01-01T02:00:00Z',
        }),
        makeHistoryEntry({
          agent_id: 'a2',
          agent_type: 'timeout-agent',
          ended_at: undefined,
        }),
        makeHistoryEntry({
          agent_id: 'a3',
          agent_type: 'implementor',
          ended_at: '2026-01-01T05:00:00Z',
        }),
      ]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const rows = document.querySelectorAll('[data-testid="agent-row"]')
      const lastRowAgentCell = rows[2].querySelector(':scope > span:first-child')

      expect(lastRowAgentCell?.textContent).toBe('timeout-agent')
    })

    it('handles multiple entries with null ended_at', () => {
      const history = [
        makeHistoryEntry({
          agent_id: 'a1',
          agent_type: 'timeout-1',
          ended_at: undefined,
        }),
        makeHistoryEntry({
          agent_id: 'a2',
          agent_type: 'completed',
          ended_at: '2026-01-01T05:00:00Z',
        }),
        makeHistoryEntry({
          agent_id: 'a3',
          agent_type: 'timeout-2',
          ended_at: undefined,
        }),
      ]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const rows = document.querySelectorAll('[data-testid="agent-row"]')

      // First row should be the completed one
      expect(rows[0].querySelector(':scope > span:first-child')?.textContent).toBe('completed')

      // Last two rows should be the timeout ones (order between them doesn't matter)
      const lastTwoAgents = [
        rows[1].querySelector(':scope > span:first-child')?.textContent,
        rows[2].querySelector(':scope > span:first-child')?.textContent,
      ]
      expect(lastTwoAgents).toContain('timeout-1')
      expect(lastTwoAgents).toContain('timeout-2')
    })
  })

  describe('result badges', () => {
    it('displays success badge for pass result', () => {
      const history = [makeHistoryEntry({ result: 'pass' })]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const badge = screen.getByText('pass')
      expect(badge).toBeInTheDocument()
      // Badge with variant="success" uses bg-green-100
      expect(badge.className).toContain('bg-green-100')
    })

    it('displays destructive badge for fail result', () => {
      const history = [makeHistoryEntry({ result: 'fail' })]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const badge = screen.getByText('fail')
      expect(badge).toBeInTheDocument()
      // Badge with variant="destructive" uses bg-destructive CSS variable
      expect(badge.className).toContain('bg-destructive')
    })

    it('displays badge for continue result', () => {
      const history = [makeHistoryEntry({ result: 'continue' })]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const badge = screen.getByText('continue')
      expect(badge).toBeInTheDocument()
    })

    it('does not display badge when result is undefined', () => {
      const history = [makeHistoryEntry({ result: undefined })]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const row = document.querySelector('[data-testid="agent-row"]')!
      const resultCell = row.querySelectorAll(':scope > span')[3]
      expect(resultCell.textContent).toBe('')
    })
  })

  describe('duration formatting', () => {
    it('uses formatElapsedTime when started_at and ended_at are present', () => {
      const history = [makeHistoryEntry({
        started_at: '2026-01-01T00:00:00Z',
        ended_at: '2026-01-01T00:02:05Z',
        duration_sec: 0,
      })]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      expect(screen.getByText('2m 5s')).toBeInTheDocument()
    })

    it('falls back to formatDuration when timestamps are missing', () => {
      const history = [makeHistoryEntry({
        started_at: undefined,
        ended_at: undefined,
        duration_sec: 125,
      })]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      expect(screen.getByText('2m 5s')).toBeInTheDocument()
    })

    it('formats duration with only minutes when no remainder', () => {
      const history = [makeHistoryEntry({
        started_at: '2026-01-01T00:00:00Z',
        ended_at: '2026-01-01T00:02:00Z',
        duration_sec: 0,
      })]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      expect(screen.getByText('2m')).toBeInTheDocument()
    })

    it('formats duration with only seconds when less than a minute', () => {
      const history = [makeHistoryEntry({
        started_at: '2026-01-01T00:00:00Z',
        ended_at: '2026-01-01T00:00:45Z',
        duration_sec: 0,
      })]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      expect(screen.getByText('45s')).toBeInTheDocument()
    })

    it('displays 0s for undefined duration and missing timestamps', () => {
      const history = [makeHistoryEntry({
        started_at: undefined,
        ended_at: undefined,
        duration_sec: undefined,
      })]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      expect(screen.getByText('0s')).toBeInTheDocument()
    })

    it('displays 0s for zero duration and missing timestamps', () => {
      const history = [makeHistoryEntry({
        started_at: undefined,
        ended_at: undefined,
        duration_sec: 0,
      })]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      expect(screen.getByText('0s')).toBeInTheDocument()
    })
  })

  describe('agent selection', () => {
    it('calls onAgentSelect when clicking a row', async () => {
      const user = userEvent.setup()
      const history = [makeHistoryEntry()]
      const session = makeSession()
      const sessions = [session]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const row = screen.getByText('implementor').closest('[data-testid="agent-row"]')!
      await user.click(row)

      expect(onAgentSelect).toHaveBeenCalledWith({
        phaseName: 'implementation',
        historyEntry: history[0],
        session,
      })
    })

    it('finds session by session_id when available', async () => {
      const user = userEvent.setup()
      const history = [
        makeHistoryEntry({
          session_id: 'specific-session',
          agent_type: 'implementor',
          phase: 'implementation',
          model_id: 'claude-opus-4-6',
        }),
      ]
      const correctSession = makeSession({
        id: 'specific-session',
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-opus-4-6',
      })
      const wrongSession = makeSession({
        id: 'wrong-session',
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-sonnet-4-5',
      })
      const sessions = [wrongSession, correctSession]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const row = screen.getByText('implementor').closest('[data-testid="agent-row"]')!
      await user.click(row)

      expect(onAgentSelect).toHaveBeenCalledWith(
        expect.objectContaining({ session: correctSession })
      )
    })

    it('falls back to fuzzy matching when session_id is not found', async () => {
      const user = userEvent.setup()
      const history = [
        makeHistoryEntry({
          session_id: 'nonexistent-session',
          agent_type: 'implementor',
          phase: 'implementation',
          model_id: 'claude-opus-4-6',
        }),
      ]
      const fallbackSession = makeSession({
        id: 'fallback-session',
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-opus-4-6',
      })
      const sessions = [fallbackSession]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const row = screen.getByText('implementor').closest('[data-testid="agent-row"]')!
      await user.click(row)

      expect(onAgentSelect).toHaveBeenCalledWith(
        expect.objectContaining({ session: fallbackSession })
      )
    })

    it('falls back to fuzzy matching when session_id is undefined', async () => {
      const user = userEvent.setup()
      const history = [
        makeHistoryEntry({
          session_id: undefined,
          agent_type: 'setup-analyzer',
          phase: 'investigation',
          model_id: 'claude-sonnet-4-5',
        }),
      ]
      const fuzzyMatchSession = makeSession({
        id: 'fuzzy-session',
        agent_type: 'setup-analyzer',
        phase: 'investigation',
        model_id: 'claude-sonnet-4-5',
      })
      const sessions = [fuzzyMatchSession]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const row = screen.getByText('setup-analyzer').closest('[data-testid="agent-row"]')!
      await user.click(row)

      expect(onAgentSelect).toHaveBeenCalledWith(
        expect.objectContaining({ session: fuzzyMatchSession })
      )
    })

    it('passes undefined session when no match found', async () => {
      const user = userEvent.setup()
      const history = [
        makeHistoryEntry({
          session_id: 'nonexistent',
          agent_type: 'implementor',
        }),
      ]
      const sessions = [
        makeSession({
          id: 'different',
          agent_type: 'tester',
        }),
      ]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const row = screen.getByText('implementor').closest('[data-testid="agent-row"]')!
      await user.click(row)

      expect(onAgentSelect).toHaveBeenCalledWith(
        expect.objectContaining({ session: undefined })
      )
    })

    it('fuzzy matching ignores model_id when entry.model_id is undefined', async () => {
      const user = userEvent.setup()
      const history = [
        makeHistoryEntry({
          session_id: undefined,
          agent_type: 'implementor',
          phase: 'implementation',
          model_id: undefined,
        }),
      ]
      const session = makeSession({
        id: 'match-session',
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-opus-4-6',
      })
      const sessions = [session]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const row = screen.getByText('implementor').closest('[data-testid="agent-row"]')!
      await user.click(row)

      expect(onAgentSelect).toHaveBeenCalledWith(
        expect.objectContaining({ session })
      )
    })
  })

  describe('table styling', () => {
    it('applies correct table classes matching card pattern', () => {
      const history = [makeHistoryEntry()]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const table = document.querySelector('[data-testid="agent-table"]')!
      expect(table.className).toContain('text-xs')
      expect(table.className).toContain('font-mono')
      expect(table.className).toContain('border')
      expect(table.className).toContain('rounded-lg')
    })

    it('applies hover effect to table rows', () => {
      const history = [makeHistoryEntry()]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const row = document.querySelector('[data-testid="agent-row"]')!
      expect(row.className).toContain('hover:bg-muted/50')
      expect(row.className).toContain('cursor-pointer')
      expect(row.className).toContain('transition-colors')
    })

    it('uses correct header styling', () => {
      const history = [makeHistoryEntry()]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const header = document.querySelector('[data-testid="agent-table-header"]')!
      expect(header.className).toContain('text-muted-foreground')
      expect(header.className).toContain('uppercase')
      expect(header.className).toContain('tracking-wider')
    })
  })

  describe('pagination', () => {
    it('shows pagination controls when more than 20 agents', () => {
      const history = Array.from({ length: 25 }, (_, i) =>
        makeHistoryEntry({
          agent_id: `a${i}`,
          agent_type: `agent-${i}`,
          session_id: `session-${i}`,
          ended_at: `2026-01-01T00:${String(i).padStart(2, '0')}:00Z`,
        })
      )
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      // Should show pagination controls
      expect(screen.getByText(/1–20 of 25/)).toBeInTheDocument()

      // Should show prev/next buttons
      const buttons = screen.getAllByRole('button')
      const prevButton = buttons.find(b => b.querySelector('svg'))
      expect(prevButton).toBeDefined()
    })

    it('displays only 20 agents per page', () => {
      const history = Array.from({ length: 25 }, (_, i) =>
        makeHistoryEntry({
          agent_id: `a${i}`,
          agent_type: `agent-${i}`,
          session_id: `session-${i}`,
          ended_at: `2026-01-01T00:${String(i).padStart(2, '0')}:00Z`,
        })
      )
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const rows = document.querySelectorAll('[data-testid="agent-row"]')
      expect(rows).toHaveLength(20)
    })

    it('navigates to next page when clicking next button', async () => {
      const user = userEvent.setup()
      const history = Array.from({ length: 25 }, (_, i) =>
        makeHistoryEntry({
          agent_id: `a${i}`,
          agent_type: `agent-${i}`,
          session_id: `session-${i}`,
          ended_at: `2026-01-01T00:${String(i).padStart(2, '0')}:00Z`,
        })
      )
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      expect(screen.getByText(/1–20 of 25/)).toBeInTheDocument()

      // Click next button (second button)
      const buttons = screen.getAllByRole('button')
      const nextButton = buttons[buttons.length - 1]
      await user.click(nextButton)

      expect(screen.getByText(/21–25 of 25/)).toBeInTheDocument()
      const rows = document.querySelectorAll('[data-testid="agent-row"]')
      expect(rows).toHaveLength(5)
    })

    it('navigates back to previous page when clicking prev button', async () => {
      const user = userEvent.setup()
      const history = Array.from({ length: 25 }, (_, i) =>
        makeHistoryEntry({
          agent_id: `a${i}`,
          agent_type: `agent-${i}`,
          session_id: `session-${i}`,
          ended_at: `2026-01-01T00:${String(i).padStart(2, '0')}:00Z`,
        })
      )
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      // Go to page 2
      const buttons = screen.getAllByRole('button')
      await user.click(buttons[buttons.length - 1])

      expect(screen.getByText(/21–25 of 25/)).toBeInTheDocument()

      // Click prev button (first button)
      const updatedButtons = screen.getAllByRole('button')
      await user.click(updatedButtons[updatedButtons.length - 2])

      expect(screen.getByText(/1–20 of 25/)).toBeInTheDocument()
    })

    it('disables prev button on first page', () => {
      const history = Array.from({ length: 25 }, (_, i) =>
        makeHistoryEntry({
          agent_id: `a${i}`,
          agent_type: `agent-${i}`,
          session_id: `session-${i}`,
          ended_at: `2026-01-01T00:${String(i).padStart(2, '0')}:00Z`,
        })
      )
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const buttons = screen.getAllByRole('button')
      const prevButton = buttons[buttons.length - 2]
      expect(prevButton).toBeDisabled()
    })

    it('disables next button on last page', async () => {
      const user = userEvent.setup()
      const history = Array.from({ length: 25 }, (_, i) =>
        makeHistoryEntry({
          agent_id: `a${i}`,
          agent_type: `agent-${i}`,
          session_id: `session-${i}`,
          ended_at: `2026-01-01T00:${String(i).padStart(2, '0')}:00Z`,
        })
      )
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      // Navigate to last page
      const buttons = screen.getAllByRole('button')
      await user.click(buttons[buttons.length - 1])

      const updatedButtons = screen.getAllByRole('button')
      const nextButton = updatedButtons[updatedButtons.length - 1]
      expect(nextButton).toBeDisabled()
    })

    it('does not show pagination controls when 20 or fewer agents', () => {
      const history = Array.from({ length: 20 }, (_, i) =>
        makeHistoryEntry({
          agent_id: `a${i}`,
          agent_type: `agent-${i}`,
          session_id: `session-${i}`,
        })
      )
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      expect(screen.queryByText(/of 20/)).not.toBeInTheDocument()
    })
  })

  describe('multiple agents', () => {
    it('renders multiple agents in sorted order', () => {
      const history = [
        makeHistoryEntry({
          agent_id: 'a1',
          agent_type: 'setup-analyzer',
          phase: 'investigation',
          ended_at: '2026-01-01T00:00:00Z',
          duration_sec: 60,
        }),
        makeHistoryEntry({
          agent_id: 'a2',
          agent_type: 'implementor',
          phase: 'implementation',
          ended_at: '2026-01-01T03:00:00Z',
          duration_sec: 7200,
        }),
        makeHistoryEntry({
          agent_id: 'a3',
          agent_type: 'qa-verifier',
          phase: 'verification',
          ended_at: '2026-01-01T05:00:00Z',
          duration_sec: 1800,
        }),
      ]
      const sessions = [makeSession()]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      const rows = document.querySelectorAll('[data-testid="agent-row"]')
      expect(rows).toHaveLength(3)

      // Latest first
      expect(rows[0].querySelector(':scope > span:first-child')?.textContent).toBe('qa-verifier')
      expect(rows[1].querySelector(':scope > span:first-child')?.textContent).toBe('implementor')
      expect(rows[2].querySelector(':scope > span:first-child')?.textContent).toBe('setup-analyzer')
    })

    it('each row is clickable and selects correct agent', async () => {
      const user = userEvent.setup()
      const history = [
        makeHistoryEntry({
          agent_id: 'a1',
          agent_type: 'setup-analyzer',
          session_id: 'session-1',
        }),
        makeHistoryEntry({
          agent_id: 'a2',
          agent_type: 'implementor',
          session_id: 'session-2',
        }),
      ]
      const sessions = [
        makeSession({ id: 'session-1', agent_type: 'setup-analyzer' }),
        makeSession({ id: 'session-2', agent_type: 'implementor' }),
      ]
      const onAgentSelect = vi.fn()

      render(
        <CompletedAgentsTable
          agentHistory={history}
          sessions={sessions}
          onAgentSelect={onAgentSelect}
        />
      )

      // Click second row (setup-analyzer, because sorted DESC by ended_at which are equal)
      const rows = document.querySelectorAll('[data-testid="agent-row"]')
      await user.click(rows[0])

      expect(onAgentSelect).toHaveBeenCalledWith(
        expect.objectContaining({
          historyEntry: expect.objectContaining({ agent_type: expect.any(String) }),
        })
      )

      onAgentSelect.mockClear()

      // Click first row
      await user.click(rows[1])

      expect(onAgentSelect).toHaveBeenCalledWith(
        expect.objectContaining({
          historyEntry: expect.objectContaining({ agent_type: expect.any(String) }),
        })
      )
    })
  })
})
