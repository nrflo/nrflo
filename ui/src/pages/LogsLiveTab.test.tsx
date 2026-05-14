import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { LogsLiveTab } from './LogsLiveTab'
import type { LiveAgentSession } from '@/types/agentSessionLogs'
import { formatMB, formatDurationSec } from '@/lib/utils'

const mockUseLive = vi.fn()
const mockUseKill = vi.fn()

vi.mock('@/hooks/useAgentSessionLogs', () => ({
  useLiveAgentSessions: () => mockUseLive(),
  useKillAgentSession: () => mockUseKill(),
}))

function makeLiveSession(overrides: Partial<LiveAgentSession> = {}): LiveAgentSession {
  return {
    session_id: 'abc12345def67890',
    project_id: 'proj-1',
    agent_type: 'implementor',
    model_id: 'claude-sonnet',
    workflow_id: 'feature',
    workflow_instance_id: 'inst-001',
    scheduled: false,
    execution_mode: 'cli_interactive',
    duration_sec: 3661,
    pid: 12345,
    rss_kb: 204800,
    cpu_pct: 12.5,
    os_uptime_sec: 7200,
    ...overrides,
  }
}

function renderTab() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <LogsLiveTab />
    </QueryClientProvider>
  )
}

describe('LogsLiveTab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseKill.mockReturnValue({ mutate: vi.fn(), isPending: false })
  })

  it('shows empty state when no sessions', () => {
    mockUseLive.mockReturnValue({ data: { sessions: [] }, isFetching: false, refetch: vi.fn() })
    renderTab()
    expect(screen.getByText('No live processes')).toBeInTheDocument()
  })

  describe('two-row render', () => {
    const session1 = makeLiveSession({
      session_id: 'row1aaaa12345678',
      agent_type: 'implementor',
      model_id: 'claude-sonnet',
      execution_mode: 'cli_interactive',
      workflow_id: 'feature',
      duration_sec: 3661,
      os_uptime_sec: 7200,
      pid: 12345,
      rss_kb: 204800,
      cpu_pct: 12.5,
    })
    const session2 = makeLiveSession({
      session_id: 'row2bbbb12345678',
      agent_type: 'qa-verifier',
      model_id: undefined,
      execution_mode: undefined,
      workflow_id: 'bugfix',
      duration_sec: 90,
      os_uptime_sec: 3600,
      pid: 99999,
      rss_kb: 51200,
      cpu_pct: 0.3,
    })

    beforeEach(() => {
      mockUseLive.mockReturnValue({
        data: { sessions: [session1, session2] },
        isFetching: false,
        refetch: vi.fn(),
      })
    })

    it('renders all 10 column headers', () => {
      renderTab()
      for (const header of ['SID', 'Agent', 'Model', 'Mode', 'Workflow', 'Uptime', 'PID', 'Memory', 'CPU %', 'Actions']) {
        expect(screen.getByText(header)).toBeInTheDocument()
      }
    })

    it('formats Memory with formatMB(rss_kb / 1024)', () => {
      renderTab()
      expect(screen.getByText(formatMB(session1.rss_kb / 1024))).toBeInTheDocument()
      expect(screen.getByText(formatMB(session2.rss_kb / 1024))).toBeInTheDocument()
    })

    it('formats CPU % with toFixed(1)', () => {
      renderTab()
      expect(screen.getByText(`${session1.cpu_pct.toFixed(1)}%`)).toBeInTheDocument()
      expect(screen.getByText(`${session2.cpu_pct.toFixed(1)}%`)).toBeInTheDocument()
    })

    it('formats Uptime with formatDurationSec(os_uptime_sec)', () => {
      renderTab()
      expect(screen.getByText(formatDurationSec(session1.os_uptime_sec))).toBeInTheDocument()
      expect(screen.getByText(formatDurationSec(session2.os_uptime_sec))).toBeInTheDocument()
    })

    it('renders agent_type for each row', () => {
      renderTab()
      expect(screen.getByText('implementor')).toBeInTheDocument()
      expect(screen.getByText('qa-verifier')).toBeInTheDocument()
    })

    it('renders pid for each row', () => {
      renderTab()
      expect(screen.getByText(String(session1.pid))).toBeInTheDocument()
      expect(screen.getByText(String(session2.pid))).toBeInTheDocument()
    })
  })

  describe('SID cell', () => {
    it('is a span with title=full session_id and not a link', () => {
      const session = makeLiveSession({ session_id: 'sid12345abcdefgh' })
      mockUseLive.mockReturnValue({ data: { sessions: [session] }, isFetching: false, refetch: vi.fn() })
      renderTab()

      const sidEl = screen.getByText('sid12345')
      expect(sidEl.nodeName).toBe('SPAN')
      expect(sidEl.closest('a')).toBeNull()
      expect(sidEl).toHaveAttribute('title', 'sid12345abcdefgh')
    })
  })

  describe('Refresh button', () => {
    it('calls refetch when clicked', async () => {
      const refetch = vi.fn()
      mockUseLive.mockReturnValue({
        data: { sessions: [makeLiveSession()] },
        isFetching: false,
        refetch,
      })
      renderTab()

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: /refresh/i }))
      expect(refetch).toHaveBeenCalledOnce()
    })

    it('shows "Loading…" text while fetching', () => {
      mockUseLive.mockReturnValue({ data: { sessions: [] }, isFetching: true, refetch: vi.fn() })
      renderTab()
      expect(screen.getByText('Loading…')).toBeInTheDocument()
    })
  })

  describe('Kill flow', () => {
    const session = makeLiveSession({ session_id: 'target1234567890' })
    const mutate = vi.fn()

    beforeEach(() => {
      mockUseLive.mockReturnValue({ data: { sessions: [session] }, isFetching: false, refetch: vi.fn() })
      mockUseKill.mockReturnValue({ mutate, isPending: false })
    })

    it('opens ConfirmDialog with correct message when Kill icon clicked', async () => {
      const user = userEvent.setup()
      renderTab()

      // buttons: [Refresh, Kill-row]
      const buttons = screen.getAllByRole('button')
      await user.click(buttons[buttons.length - 1])

      expect(screen.getByText('Force-kill this agent? Status becomes failed.')).toBeInTheDocument()
    })

    it('calls mutate(session_id) after confirming Kill', async () => {
      const user = userEvent.setup()
      renderTab()

      const buttons = screen.getAllByRole('button')
      await user.click(buttons[buttons.length - 1])

      // Dialog confirm button has text "Kill"
      await user.click(screen.getByRole('button', { name: 'Kill' }))
      expect(mutate).toHaveBeenCalledWith('target1234567890')
    })

    it('disables Kill button when mutation isPending', () => {
      mockUseKill.mockReturnValue({ mutate: vi.fn(), isPending: true })
      renderTab()

      const buttons = screen.getAllByRole('button')
      // Last button is the row Kill button
      expect(buttons[buttons.length - 1]).toBeDisabled()
    })
  })
})
