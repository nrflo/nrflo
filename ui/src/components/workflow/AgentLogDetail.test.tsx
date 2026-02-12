import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
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
    getSessionRawOutput: vi.fn(),
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
    vi.mocked(ticketsApi.getSessionRawOutput).mockResolvedValue({
      session_id: 'session-1',
      raw_output: '$ npm install\n+ added 120 packages\n$ npm run build\nBuild complete.',
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
      // Model name is derived from model_id: claude-sonnet-4-5 => split('-').slice(-2).join('-') => '4-5'
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

      // The back button is an ArrowLeft icon button
      const buttons = screen.getAllByRole('button')
      // First button is the back button
      await user.click(buttons[0])
      expect(onBack).toHaveBeenCalledTimes(1)
    })
  })

  describe('messages/raw toggle - acceptance criteria', () => {
    it('defaults to Messages view (criterion #4)', async () => {
      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession(),
      })

      // Messages button should be active (has 'bg-accent text-accent-foreground font-medium')
      const messagesBtn = screen.getByRole('button', { name: /messages/i })
      expect(messagesBtn.className).toContain('font-medium')

      // Raw button should not be active (no font-medium class)
      const rawBtn = screen.getByRole('button', { name: /raw/i })
      expect(rawBtn.className).not.toContain('font-medium')

      // Messages content should load
      await waitFor(() => {
        expect(screen.getByText('3 messages')).toBeInTheDocument()
      })
    })

    it('shows messages/raw toggle at top of panel (criterion #1)', () => {
      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession(),
      })

      expect(screen.getByRole('button', { name: /messages/i })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /raw/i })).toBeInTheDocument()
    })

    it('toggles to Raw view and shows raw output (criterion #2)', async () => {
      const user = userEvent.setup()

      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession(),
      })

      // Switch to Raw
      await user.click(screen.getByRole('button', { name: /raw/i }))

      // Raw button should now be active
      const rawBtn = screen.getByRole('button', { name: /raw/i })
      expect(rawBtn.className).toContain('font-medium')

      // Raw output should load
      await waitFor(() => {
        expect(screen.getByText(/npm install/)).toBeInTheDocument()
      })
      expect(screen.getByText(/Build complete/)).toBeInTheDocument()
    })

    it('toggles back to Messages from Raw', async () => {
      const user = userEvent.setup()

      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession(),
      })

      // Switch to Raw
      await user.click(screen.getByRole('button', { name: /raw/i }))

      await waitFor(() => {
        expect(screen.getByText(/npm install/)).toBeInTheDocument()
      })

      // Switch back to Messages
      await user.click(screen.getByRole('button', { name: /messages/i }))

      await waitFor(() => {
        expect(screen.getByText('3 messages')).toBeInTheDocument()
      })
    })

    it('shows raw output size in bytes on Raw button', () => {
      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession({ raw_output_size: 2048 }),
      })

      expect(screen.getByText('(2.0 KB)')).toBeInTheDocument()
    })

    it('does not show toggle when no raw output and not running', () => {
      renderDetail({
        phaseName: 'investigation',
        historyEntry: makeHistoryEntry(),
        session: makeSession({ raw_output_size: 0, status: 'completed' }),
      })

      expect(screen.queryByRole('button', { name: /raw/i })).not.toBeInTheDocument()
    })

    it('shows toggle for running agent even with no raw output yet', () => {
      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession({ raw_output_size: 0 }),
      })

      expect(screen.getByRole('button', { name: /raw/i })).toBeInTheDocument()
    })
  })

  describe('messages display', () => {
    it('shows messages in reversed order (newest first)', async () => {
      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession(),
      })

      await waitFor(() => {
        expect(screen.getByText('3 messages')).toBeInTheDocument()
      })

      // Messages should be rendered (getSessionMessages mock returns 3 messages)
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

  describe('raw output display', () => {
    it('shows raw output in monospace pre block', async () => {
      const user = userEvent.setup()

      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession(),
      })

      await user.click(screen.getByRole('button', { name: /raw/i }))

      await waitFor(() => {
        const pre = screen.getByText(/npm install/).closest('pre')
        expect(pre).toBeInTheDocument()
        expect(pre?.className).toContain('font-mono')
      })
    })

    it('shows empty state when raw output is empty', async () => {
      const user = userEvent.setup()

      vi.mocked(ticketsApi.getSessionRawOutput).mockResolvedValue({
        session_id: 'session-1',
        raw_output: '',
      })

      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession(),
      })

      await user.click(screen.getByRole('button', { name: /raw/i }))

      await waitFor(() => {
        expect(screen.getByText('No raw output available')).toBeInTheDocument()
      })
    })

    it('shows loading state while raw output is being fetched', async () => {
      const user = userEvent.setup()

      vi.mocked(ticketsApi.getSessionRawOutput).mockReturnValue(new Promise(() => {}))

      renderDetail({
        phaseName: 'implementation',
        agent: makeRunningAgent(),
        session: makeSession(),
      })

      await user.click(screen.getByRole('button', { name: /raw/i }))

      expect(screen.getByText('Loading raw output...')).toBeInTheDocument()
    })
  })

  describe('agent change behavior', () => {
    it('resets to Messages view when agent changes', async () => {
      const user = userEvent.setup()
      const session1 = makeSession({ id: 'session-1' })
      const session2 = makeSession({ id: 'session-2', phase: 'verification', agent_type: 'tester' })

      vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
        session_id: 'session-1',
        messages: [{ content: 'msg1', created_at: '' }],
        total: 1,
      })

      const queryClient = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      })

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <AgentLogDetail
            selectedAgent={{ phaseName: 'implementation', agent: makeRunningAgent(), session: session1 }}
            onBack={vi.fn()}
          />
        </QueryClientProvider>
      )

      // Toggle to Raw
      await user.click(screen.getByRole('button', { name: /raw/i }))
      expect(screen.getByRole('button', { name: /raw/i }).className).toContain('font-medium')

      // Change agent
      rerender(
        <QueryClientProvider client={queryClient}>
          <AgentLogDetail
            selectedAgent={{
              phaseName: 'verification',
              agent: makeRunningAgent({ agent_type: 'tester', phase: 'verification' }),
              session: session2,
            }}
            onBack={vi.fn()}
          />
        </QueryClientProvider>
      )

      // Should reset to Messages
      await waitFor(() => {
        const messagesBtn = screen.getByRole('button', { name: /messages/i })
        expect(messagesBtn.className).toContain('font-medium')
      })
    })
  })
})
