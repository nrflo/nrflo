import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { LogsPage } from './LogsPage'
import type { AgentSessionLogEntry, AgentSessionLogsResponse } from '@/types/agentSessionLogs'

const mockUseAgentSessionLogs = vi.fn()
vi.mock('@/hooks/useAgentSessionLogs', () => ({
  useAgentSessionLogs: (params?: unknown) => mockUseAgentSessionLogs(params),
}))

function makeLogEntry(overrides: Partial<AgentSessionLogEntry> = {}): AgentSessionLogEntry {
  return {
    session_id: 'abcd1234efgh5678',
    project_id: 'proj-1',
    agent_type: 'implementor',
    status: 'finished',
    started_at: '2026-01-15T10:00:00Z',
    ended_at: '2026-01-15T10:30:00Z',
    workflow_id: 'feature',
    workflow_instance_id: 'inst-001',
    scheduled: false,
    ...overrides,
  }
}

function makeResponse(overrides: Partial<AgentSessionLogsResponse> = {}): AgentSessionLogsResponse {
  return {
    sessions: [],
    total: 0,
    page: 1,
    per_page: 20,
    total_pages: 1,
    ...overrides,
  }
}

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <LogsPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('LogsPage', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows loading state', () => {
    mockUseAgentSessionLogs.mockReturnValue({ data: undefined, isLoading: true })
    renderPage()
    expect(screen.getByText('Loading...')).toBeInTheDocument()
  })

  it('shows empty state when no sessions', () => {
    mockUseAgentSessionLogs.mockReturnValue({ data: makeResponse(), isLoading: false })
    renderPage()
    expect(screen.getByText('No agent sessions yet')).toBeInTheDocument()
  })

  describe('two-row render', () => {
    const row1 = makeLogEntry({
      session_id: 'row1aaaa12345678',
      agent_type: 'implementor',
      execution_mode: 'cli',
      scheduled: true,
      workflow_final_result: 'All tests passed',
    })
    const row2 = makeLogEntry({
      session_id: 'row2bbbb12345678',
      agent_type: 'qa-verifier',
      execution_mode: 'api',
      scheduled: false,
      workflow_final_result: undefined,
    })

    beforeEach(() => {
      mockUseAgentSessionLogs.mockReturnValue({
        data: makeResponse({ sessions: [row1, row2], total: 2, total_pages: 1 }),
        isLoading: false,
      })
    })

    it('renders execution_mode badge for both rows', () => {
      renderPage()
      expect(screen.getByText('cli')).toBeInTheDocument()
      expect(screen.getByText('api')).toBeInTheDocument()
    })

    it('renders CalendarClock icon only for the scheduled row', () => {
      renderPage()
      const icons = document.querySelectorAll('[class*="lucide-calendar-clock"]')
      expect(icons).toHaveLength(1)
    })

    it('renders CheckCircle2 icon only for the row with workflow_final_result', () => {
      renderPage()
      // CheckCircle2 is aliased to CircleCheck in lucide-react v0.563 → class lucide-circle-check
      const icons = document.querySelectorAll('svg.lucide-circle-check')
      expect(icons).toHaveLength(1)
    })

    it('shows em-dash for row 2 result (no workflow_final_result)', () => {
      renderPage()
      const emDashes = screen.getAllByText('—')
      expect(emDashes.length).toBeGreaterThanOrEqual(1)
    })

    it('renders agent_type for both rows', () => {
      renderPage()
      expect(screen.getByText('implementor')).toBeInTheDocument()
      expect(screen.getByText('qa-verifier')).toBeInTheDocument()
    })

    it('truncates session_id to first 8 chars in SID column', () => {
      renderPage()
      expect(screen.getByText('row1aaaa')).toBeInTheDocument()
      expect(screen.getByText('row2bbbb')).toBeInTheDocument()
    })

    describe('tooltip content via hover', () => {
      it('shows "Triggered by scheduler" tooltip on hover of CalendarClock in row 1', async () => {
        const user = userEvent.setup()
        renderPage()

        const icon = document.querySelector('svg.lucide-calendar-clock')
        expect(icon).not.toBeNull()
        await user.hover(icon!)

        // Radix renders text twice (visible content + sr-only role=tooltip span)
        expect((await screen.findAllByText('Triggered by scheduler')).length).toBeGreaterThan(0)
      })

      it('shows workflow_final_result text in tooltip on hover of CheckCircle2 in row 1', async () => {
        const user = userEvent.setup()
        renderPage()

        const icon = document.querySelector('svg.lucide-circle-check')
        expect(icon).not.toBeNull()
        await user.hover(icon!)

        expect((await screen.findAllByText('All tests passed')).length).toBeGreaterThan(0)
      })
    })
  })

  describe('pagination', () => {
    function makeSessions(count: number) {
      return Array.from({ length: count }, (_, i) =>
        makeLogEntry({ session_id: `${String(i).padStart(16, '0')}`, agent_type: `agent-${i}` })
      )
    }

    it('shows 1–20 of 45 for multi-page results', () => {
      mockUseAgentSessionLogs.mockReturnValue({
        data: makeResponse({ sessions: makeSessions(20), total: 45, page: 1, per_page: 20, total_pages: 3 }),
        isLoading: false,
      })
      renderPage()
      expect(screen.getByText('1–20 of 45')).toBeInTheDocument()
    })

    it('disables prev button on page 1', () => {
      mockUseAgentSessionLogs.mockReturnValue({
        data: makeResponse({ sessions: makeSessions(20), total: 45, page: 1, per_page: 20, total_pages: 3 }),
        isLoading: false,
      })
      renderPage()
      const buttons = screen.getAllByRole('button')
      const prevButton = buttons[buttons.length - 2]
      expect(prevButton).toBeDisabled()
    })

    it('clicking next button calls hook with page=2', async () => {
      const user = userEvent.setup()
      mockUseAgentSessionLogs.mockReturnValue({
        data: makeResponse({ sessions: makeSessions(20), total: 45, page: 1, per_page: 20, total_pages: 3 }),
        isLoading: false,
      })
      renderPage()

      const buttons = screen.getAllByRole('button')
      const nextButton = buttons[buttons.length - 1]
      await user.click(nextButton)

      expect(mockUseAgentSessionLogs.mock.calls.some((call: unknown[]) => (call[0] as { page?: number })?.page === 2)).toBe(true)
    })

    it('hides pagination footer when total_pages <= 1', () => {
      mockUseAgentSessionLogs.mockReturnValue({
        data: makeResponse({ sessions: [makeLogEntry()], total: 1, total_pages: 1 }),
        isLoading: false,
      })
      renderPage()
      expect(screen.queryByText(/of \d+/)).not.toBeInTheDocument()
    })
  })
})
