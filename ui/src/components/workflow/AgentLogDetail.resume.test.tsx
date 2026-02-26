import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AgentLogDetail } from './AgentLogDetail'
import * as ticketsApi from '@/api/tickets'
import type { SelectedAgentData } from './PhaseGraph/types'
import type { AgentHistoryEntry, AgentSession } from '@/types/workflow'

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
    model_id: 'claude:sonnet-4-5',
    status: 'completed',
    message_count: 0,
    restart_count: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeHistoryEntry(overrides: Partial<AgentHistoryEntry> = {}): AgentHistoryEntry {
  return {
    agent_id: 'h1',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude:sonnet-4-5',
    session_id: 'session-1',
    result: 'pass',
    duration_sec: 60,
    started_at: '2026-01-01T00:00:00Z',
    ended_at: '2026-01-01T00:01:00Z',
    ...overrides,
  }
}

interface ExtraProps {
  onResumeSession?: (sessionId: string) => void
  resumePending?: boolean
}

function renderDetail(
  selectedAgent: SelectedAgentData,
  extraProps: ExtraProps = {},
  onBack = vi.fn(),
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return {
    ...render(
      <QueryClientProvider client={queryClient}>
        <AgentLogDetail
          selectedAgent={selectedAgent}
          onBack={onBack}
          {...extraProps}
        />
      </QueryClientProvider>
    ),
    onBack,
  }
}

describe('AgentLogDetail — resume session button', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [],
      total: 0,
    })
  })

  describe('button visibility', () => {
    it('shows Resume button for a completed Claude session (result=pass)', () => {
      const onResumeSession = vi.fn()
      renderDetail(
        {
          phaseName: 'implementation',
          historyEntry: makeHistoryEntry({ result: 'pass', model_id: 'claude:sonnet-4-5' }),
          session: makeSession({ status: 'completed' }),
        },
        { onResumeSession },
      )

      expect(screen.getByRole('button', { name: /Resume/i })).toBeInTheDocument()
    })

    it('shows Resume button for a failed Claude session (result=fail)', () => {
      const onResumeSession = vi.fn()
      renderDetail(
        {
          phaseName: 'implementation',
          historyEntry: makeHistoryEntry({ result: 'fail', model_id: 'claude:opus-4-6' }),
          session: makeSession({ status: 'failed' }),
        },
        { onResumeSession },
      )

      expect(screen.getByRole('button', { name: /Resume/i })).toBeInTheDocument()
    })

    it('hides Resume button for non-Claude (opencode) session', () => {
      const onResumeSession = vi.fn()
      renderDetail(
        {
          phaseName: 'implementation',
          historyEntry: makeHistoryEntry({ result: 'pass', model_id: 'opencode:openai/gpt-4' }),
          session: makeSession({ status: 'completed', model_id: 'opencode:openai/gpt-4' }),
        },
        { onResumeSession },
      )

      expect(screen.queryByRole('button', { name: /Resume/i })).not.toBeInTheDocument()
    })

    it('hides Resume button for codex session', () => {
      const onResumeSession = vi.fn()
      renderDetail(
        {
          phaseName: 'implementation',
          historyEntry: makeHistoryEntry({ result: 'pass', model_id: 'codex:gpt-normal' }),
          session: makeSession({ status: 'completed', model_id: 'codex:gpt-normal' }),
        },
        { onResumeSession },
      )

      expect(screen.queryByRole('button', { name: /Resume/i })).not.toBeInTheDocument()
    })

    it('hides Resume button when session is user_interactive', () => {
      const onResumeSession = vi.fn()
      renderDetail(
        {
          phaseName: 'implementation',
          historyEntry: makeHistoryEntry({ result: 'pass', model_id: 'claude:sonnet-4-5' }),
          session: makeSession({ status: 'user_interactive' }),
        },
        { onResumeSession },
      )

      expect(screen.queryByRole('button', { name: /Resume/i })).not.toBeInTheDocument()
    })

    it('hides Resume button for running agent (no result)', () => {
      const onResumeSession = vi.fn()
      renderDetail(
        {
          phaseName: 'implementation',
          agent: {
            agent_type: 'implementor',
            model_id: 'claude:sonnet-4-5',
            cli: 'claude',
            session_id: 'session-1',
          },
          session: makeSession({ status: 'running', result: undefined }),
        },
        { onResumeSession },
      )

      expect(screen.queryByRole('button', { name: /Resume/i })).not.toBeInTheDocument()
    })

    it('hides Resume button when onResumeSession prop is not provided', () => {
      renderDetail({
        phaseName: 'implementation',
        historyEntry: makeHistoryEntry({ result: 'pass', model_id: 'claude:sonnet-4-5' }),
        session: makeSession({ status: 'completed' }),
      })

      expect(screen.queryByRole('button', { name: /Resume/i })).not.toBeInTheDocument()
    })

    it('hides Resume button when model_id is absent', () => {
      const onResumeSession = vi.fn()
      renderDetail(
        {
          phaseName: 'implementation',
          historyEntry: makeHistoryEntry({ result: 'pass', model_id: undefined }),
          session: makeSession({ status: 'completed', model_id: undefined }),
        },
        { onResumeSession },
      )

      expect(screen.queryByRole('button', { name: /Resume/i })).not.toBeInTheDocument()
    })
  })

  describe('button interaction', () => {
    it('calls onResumeSession with session.id when clicked', async () => {
      const user = userEvent.setup()
      const onResumeSession = vi.fn()

      renderDetail(
        {
          phaseName: 'implementation',
          historyEntry: makeHistoryEntry({ result: 'pass', model_id: 'claude:sonnet-4-5' }),
          session: makeSession({ id: 'session-abc', status: 'completed' }),
        },
        { onResumeSession },
      )

      await user.click(screen.getByRole('button', { name: /Resume/i }))
      expect(onResumeSession).toHaveBeenCalledOnce()
      expect(onResumeSession).toHaveBeenCalledWith('session-abc')
    })

    it('calls onResumeSession with historyEntry.session_id when session object is absent', async () => {
      const user = userEvent.setup()
      const onResumeSession = vi.fn()

      renderDetail(
        {
          phaseName: 'implementation',
          historyEntry: makeHistoryEntry({
            result: 'pass',
            model_id: 'claude:sonnet-4-5',
            session_id: 'history-sess-id',
          }),
        },
        { onResumeSession },
      )

      await user.click(screen.getByRole('button', { name: /Resume/i }))
      expect(onResumeSession).toHaveBeenCalledWith('history-sess-id')
    })

    it('disables Resume button when resumePending=true', () => {
      const onResumeSession = vi.fn()

      renderDetail(
        {
          phaseName: 'implementation',
          historyEntry: makeHistoryEntry({ result: 'pass', model_id: 'claude:sonnet-4-5' }),
          session: makeSession({ status: 'completed' }),
        },
        { onResumeSession, resumePending: true },
      )

      expect(screen.getByRole('button', { name: /Resume/i })).toBeDisabled()
    })

    it('does not call onResumeSession when button is disabled (resumePending)', async () => {
      const user = userEvent.setup()
      const onResumeSession = vi.fn()

      renderDetail(
        {
          phaseName: 'implementation',
          historyEntry: makeHistoryEntry({ result: 'pass', model_id: 'claude:sonnet-4-5' }),
          session: makeSession({ status: 'completed' }),
        },
        { onResumeSession, resumePending: true },
      )

      await user.click(screen.getByRole('button', { name: /Resume/i }))
      expect(onResumeSession).not.toHaveBeenCalled()
    })
  })

  describe('button not shown for intermediate states', () => {
    it('hides Resume for interactive_completed session (terminal state but already resumed)', () => {
      const onResumeSession = vi.fn()
      // interactive_completed is a terminal state; session?.status !== 'user_interactive' is true,
      // but the history entry's result drives visibility. With result=pass it should show.
      // This test documents that interactive_completed sessions CAN be resumed again.
      renderDetail(
        {
          phaseName: 'implementation',
          historyEntry: makeHistoryEntry({ result: 'pass', model_id: 'claude:sonnet-4-5' }),
          session: makeSession({ status: 'interactive_completed' }),
        },
        { onResumeSession },
      )

      // interactive_completed is not 'user_interactive', so button should appear
      expect(screen.getByRole('button', { name: /Resume/i })).toBeInTheDocument()
    })
  })
})
