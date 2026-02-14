import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ActiveAgentsPanel } from './ActiveAgentsPanel'
import type { ActiveAgentV4, AgentHistoryEntry } from '@/types/workflow'

function makeAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_id: 'a1',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude-sonnet-4-5',
    cli: 'claude',
    pid: 12345,
    session_id: 'session-abc-123',
    started_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeFailedHistoryEntry(overrides: Partial<AgentHistoryEntry> = {}): AgentHistoryEntry {
  return {
    agent_id: 'h1',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude-sonnet-4-5',
    result: 'fail',
    started_at: '2026-01-01T00:00:00Z',
    ended_at: '2026-01-01T00:05:00Z',
    session_id: 'sess-failed-123',
    ...overrides,
  }
}

describe('ActiveAgentsPanel - retry failed agents', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('failed agents section visibility', () => {
    it('shows "Failed Agents" section when workflow status is failed and has failed agents', () => {
      const agentHistory = [makeFailedHistoryEntry()]

      render(
        <ActiveAgentsPanel
          agents={{}}
          onRetryFailed={vi.fn()}
          workflowStatus="failed"
          agentHistory={agentHistory}
        />
      )

      expect(screen.getByText('Failed Agents')).toBeInTheDocument()
    })

    it('does not show "Failed Agents" section when workflow status is active', () => {
      const agentHistory = [makeFailedHistoryEntry()]

      render(
        <ActiveAgentsPanel
          agents={{}}
          onRetryFailed={vi.fn()}
          workflowStatus="active"
          agentHistory={agentHistory}
        />
      )

      expect(screen.queryByText('Failed Agents')).not.toBeInTheDocument()
    })

    it('does not show "Failed Agents" section when workflow status is completed', () => {
      const agentHistory = [makeFailedHistoryEntry()]

      render(
        <ActiveAgentsPanel
          agents={{}}
          onRetryFailed={vi.fn()}
          workflowStatus="completed"
          agentHistory={agentHistory}
        />
      )

      expect(screen.queryByText('Failed Agents')).not.toBeInTheDocument()
    })

    it('does not show "Failed Agents" section when onRetryFailed is not provided', () => {
      const agentHistory = [makeFailedHistoryEntry()]

      render(
        <ActiveAgentsPanel
          agents={{}}
          workflowStatus="failed"
          agentHistory={agentHistory}
        />
      )

      expect(screen.queryByText('Failed Agents')).not.toBeInTheDocument()
    })

    it('does not show "Failed Agents" section when agentHistory is empty', () => {
      render(
        <ActiveAgentsPanel
          agents={{}}
          onRetryFailed={vi.fn()}
          workflowStatus="failed"
          agentHistory={[]}
        />
      )

      expect(screen.queryByText('Failed Agents')).not.toBeInTheDocument()
    })

    it('does not show "Failed Agents" section when agentHistory is undefined', () => {
      render(
        <ActiveAgentsPanel
          agents={{}}
          onRetryFailed={vi.fn()}
          workflowStatus="failed"
          agentHistory={undefined}
        />
      )

      expect(screen.queryByText('Failed Agents')).not.toBeInTheDocument()
    })

    it('does not show "Failed Agents" section when no agents have result=fail', () => {
      const agentHistory = [
        { ...makeFailedHistoryEntry(), result: 'pass', status: 'completed' },
      ]

      render(
        <ActiveAgentsPanel
          agents={{}}
          onRetryFailed={vi.fn()}
          workflowStatus="failed"
          agentHistory={agentHistory}
        />
      )

      expect(screen.queryByText('Failed Agents')).not.toBeInTheDocument()
    })
  })

  describe('failed agent entries', () => {
    it('shows failed agent name and model', () => {
      const agentHistory = [
        makeFailedHistoryEntry({
          agent_type: 'implementor',
          model_id: 'claude-opus-4-6',
        }),
      ]

      render(
        <ActiveAgentsPanel
          agents={{}}
          onRetryFailed={vi.fn()}
          workflowStatus="failed"
          agentHistory={agentHistory}
        />
      )

      expect(screen.getByText('implementor')).toBeInTheDocument()
      expect(screen.getByText('claude-opus-4-6')).toBeInTheDocument()
    })

    it('shows fail badge for failed agents', () => {
      const agentHistory = [makeFailedHistoryEntry()]

      render(
        <ActiveAgentsPanel
          agents={{}}
          onRetryFailed={vi.fn()}
          workflowStatus="failed"
          agentHistory={agentHistory}
        />
      )

      expect(screen.getByText('fail')).toBeInTheDocument()
    })

    it('shows multiple failed agents', () => {
      const agentHistory = [
        makeFailedHistoryEntry({ agent_type: 'implementor', session_id: 'sess-1' }),
        makeFailedHistoryEntry({ agent_type: 'tester', session_id: 'sess-2' }),
      ]

      render(
        <ActiveAgentsPanel
          agents={{}}
          onRetryFailed={vi.fn()}
          workflowStatus="failed"
          agentHistory={agentHistory}
        />
      )

      expect(screen.getByText('implementor')).toBeInTheDocument()
      expect(screen.getByText('tester')).toBeInTheDocument()
    })

    it('only shows agents with result=fail, not other completed agents', () => {
      const agentHistory = [
        { ...makeFailedHistoryEntry(), agent_type: 'passed-agent', result: 'pass', status: 'completed' },
        makeFailedHistoryEntry({ agent_type: 'failed-agent' }),
      ]

      render(
        <ActiveAgentsPanel
          agents={{}}
          onRetryFailed={vi.fn()}
          workflowStatus="failed"
          agentHistory={agentHistory}
        />
      )

      expect(screen.getByText('failed-agent')).toBeInTheDocument()
      expect(screen.queryByText('passed-agent')).not.toBeInTheDocument()
    })
  })

  describe('retry button in failed agents section', () => {
    it('shows retry button for each failed agent', () => {
      const agentHistory = [
        makeFailedHistoryEntry({ session_id: 'sess-1' }),
        makeFailedHistoryEntry({ session_id: 'sess-2', agent_type: 'tester' }),
      ]

      render(
        <ActiveAgentsPanel
          agents={{}}
          onRetryFailed={vi.fn()}
          workflowStatus="failed"
          agentHistory={agentHistory}
        />
      )

      const buttons = screen.getAllByTitle('Retry failed agent')
      expect(buttons).toHaveLength(2)
    })

    it('calls onRetryFailed with correct session_id when clicked', async () => {
      const user = userEvent.setup()
      const onRetryFailed = vi.fn()
      const agentHistory = [makeFailedHistoryEntry({ session_id: 'sess-failed-1' })]

      render(
        <ActiveAgentsPanel
          agents={{}}
          onRetryFailed={onRetryFailed}
          workflowStatus="failed"
          agentHistory={agentHistory}
        />
      )

      await user.click(screen.getByTitle('Retry failed agent'))
      expect(onRetryFailed).toHaveBeenCalledWith('sess-failed-1')
      expect(onRetryFailed).toHaveBeenCalledTimes(1)
    })

    it('disables retry button when retryingSessionId is set', () => {
      const agentHistory = [makeFailedHistoryEntry({ session_id: 'sess-1' })]

      render(
        <ActiveAgentsPanel
          agents={{}}
          onRetryFailed={vi.fn()}
          retryingSessionId="sess-1"
          workflowStatus="failed"
          agentHistory={agentHistory}
        />
      )

      expect(screen.getByTitle('Retry failed agent')).toBeDisabled()
    })

    it('disables all retry buttons when any retryingSessionId is set', () => {
      const agentHistory = [
        makeFailedHistoryEntry({ session_id: 'sess-1' }),
        makeFailedHistoryEntry({ session_id: 'sess-2', agent_type: 'tester' }),
      ]

      render(
        <ActiveAgentsPanel
          agents={{}}
          onRetryFailed={vi.fn()}
          retryingSessionId="sess-1"
          workflowStatus="failed"
          agentHistory={agentHistory}
        />
      )

      const buttons = screen.getAllByTitle('Retry failed agent')
      buttons.forEach(button => {
        expect(button).toBeDisabled()
      })
    })

    it('does not disable retry button when retryingSessionId is null', () => {
      const agentHistory = [makeFailedHistoryEntry()]

      render(
        <ActiveAgentsPanel
          agents={{}}
          onRetryFailed={vi.fn()}
          retryingSessionId={null}
          workflowStatus="failed"
          agentHistory={agentHistory}
        />
      )

      expect(screen.getByTitle('Retry failed agent')).not.toBeDisabled()
    })

    it('does not call onRetryFailed when button is disabled', async () => {
      const user = userEvent.setup()
      const onRetryFailed = vi.fn()
      const agentHistory = [makeFailedHistoryEntry({ session_id: 'sess-1' })]

      render(
        <ActiveAgentsPanel
          agents={{}}
          onRetryFailed={onRetryFailed}
          retryingSessionId="sess-1"
          workflowStatus="failed"
          agentHistory={agentHistory}
        />
      )

      await user.click(screen.getByTitle('Retry failed agent'))
      expect(onRetryFailed).not.toHaveBeenCalled()
    })

    it('does not show retry button when failed agent has no session_id', () => {
      const agentHistory = [makeFailedHistoryEntry({ session_id: undefined })]

      render(
        <ActiveAgentsPanel
          agents={{}}
          onRetryFailed={vi.fn()}
          workflowStatus="failed"
          agentHistory={agentHistory}
        />
      )

      // Section still shows
      expect(screen.getByText('Failed Agents')).toBeInTheDocument()
      // But no retry button
      expect(screen.queryByTitle('Retry failed agent')).not.toBeInTheDocument()
    })
  })

  describe('retry button styling', () => {
    it('retry button has red styling', () => {
      const agentHistory = [makeFailedHistoryEntry()]

      render(
        <ActiveAgentsPanel
          agents={{}}
          onRetryFailed={vi.fn()}
          workflowStatus="failed"
          agentHistory={agentHistory}
        />
      )

      const button = screen.getByTitle('Retry failed agent')
      expect(button.className).toContain('text-red')
      expect(button.className).toContain('border-red')
    })
  })

  describe('mixed running and failed agents', () => {
    it('shows both running agents section and failed agents section', () => {
      const runningAgents = {
        'impl:claude:sonnet': makeAgent(),
      }
      const agentHistory = [makeFailedHistoryEntry({ agent_type: 'tester' })]

      render(
        <ActiveAgentsPanel
          agents={runningAgents}
          onRetryFailed={vi.fn()}
          workflowStatus="failed"
          agentHistory={agentHistory}
        />
      )

      // Running agent shows in main section
      expect(screen.getByText('implementor')).toBeInTheDocument()
      // Failed agent shows in failed section
      expect(screen.getByText('Failed Agents')).toBeInTheDocument()
      expect(screen.getByText('tester')).toBeInTheDocument()
    })

    it('shows restart button for running agent and retry button for failed agent', () => {
      const runningAgents = {
        'impl:claude:sonnet': makeAgent(),
      }
      const agentHistory = [makeFailedHistoryEntry()]

      render(
        <ActiveAgentsPanel
          agents={runningAgents}
          onRestart={vi.fn()}
          onRetryFailed={vi.fn()}
          workflowStatus="failed"
          agentHistory={agentHistory}
        />
      )

      expect(screen.getByTitle('Restart agent (save context, relaunch)')).toBeInTheDocument()
      expect(screen.getByTitle('Retry failed agent')).toBeInTheDocument()
    })
  })
})
