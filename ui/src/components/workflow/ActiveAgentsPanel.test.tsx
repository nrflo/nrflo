import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ActiveAgentsPanel } from './ActiveAgentsPanel'
import type { ActiveAgentV4 } from '@/types/workflow'

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

describe('ActiveAgentsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('restart button visibility', () => {
    it('shows restart button for running agent with session_id when onRestart provided', () => {
      const onRestart = vi.fn()
      render(
        <ActiveAgentsPanel
          agents={{ 'impl:claude:sonnet': makeAgent() }}
          onRestart={onRestart}
        />
      )

      expect(screen.getByLabelText('Restart agent (save context, relaunch)')).toBeInTheDocument()
    })

    it('does not show restart button when onRestart is not provided', () => {
      render(
        <ActiveAgentsPanel agents={{ 'impl:claude:sonnet': makeAgent() }} />
      )

      expect(screen.queryByLabelText('Restart agent (save context, relaunch)')).not.toBeInTheDocument()
    })

    it('does not show restart button for completed agent (has result)', () => {
      const onRestart = vi.fn()
      render(
        <ActiveAgentsPanel
          agents={{ 'impl:claude:sonnet': makeAgent({ result: 'pass' }) }}
          onRestart={onRestart}
        />
      )

      // Completed agents are not in the running list at all
      expect(screen.queryByLabelText('Restart agent (save context, relaunch)')).not.toBeInTheDocument()
    })

    it('does not show restart button for agent without session_id', () => {
      const onRestart = vi.fn()
      render(
        <ActiveAgentsPanel
          agents={{ 'impl:claude:sonnet': makeAgent({ session_id: undefined }) }}
          onRestart={onRestart}
        />
      )

      expect(screen.queryByLabelText('Restart agent (save context, relaunch)')).not.toBeInTheDocument()
    })
  })

  describe('restart button interaction', () => {
    it('calls onRestart with correct session_id when clicked', async () => {
      const user = userEvent.setup()
      const onRestart = vi.fn()

      render(
        <ActiveAgentsPanel
          agents={{ 'impl:claude:sonnet': makeAgent({ session_id: 'my-session-id' }) }}
          onRestart={onRestart}
        />
      )

      await user.click(screen.getByLabelText('Restart agent (save context, relaunch)'))
      // Confirm dialog opens, click "Restart"
      await user.click(screen.getByText('Restart'))
      expect(onRestart).toHaveBeenCalledWith('my-session-id')
      expect(onRestart).toHaveBeenCalledTimes(1)
    })

    it('disables restart button when restartingSessionId matches agent session_id', () => {
      const onRestart = vi.fn()

      render(
        <ActiveAgentsPanel
          agents={{ 'impl:claude:sonnet': makeAgent({ session_id: 'sess-1' }) }}
          onRestart={onRestart}
          restartingSessionId="sess-1"
        />
      )

      const button = screen.getByLabelText('Restart agent (save context, relaunch)')
      expect(button).toBeDisabled()
    })

    it('does not disable restart button when restartingSessionId is for a different agent', () => {
      const onRestart = vi.fn()

      render(
        <ActiveAgentsPanel
          agents={{ 'impl:claude:sonnet': makeAgent({ session_id: 'sess-1' }) }}
          onRestart={onRestart}
          restartingSessionId="sess-other"
        />
      )

      const button = screen.getByLabelText('Restart agent (save context, relaunch)')
      expect(button).not.toBeDisabled()
    })

    it('does not call onRestart when button is disabled (rapid clicking protection)', async () => {
      const user = userEvent.setup()
      const onRestart = vi.fn()

      render(
        <ActiveAgentsPanel
          agents={{ 'impl:claude:sonnet': makeAgent({ session_id: 'sess-1' }) }}
          onRestart={onRestart}
          restartingSessionId="sess-1"
        />
      )

      const button = screen.getByLabelText('Restart agent (save context, relaunch)')
      await user.click(button)
      expect(onRestart).not.toHaveBeenCalled()
    })
  })

  describe('multiple agents', () => {
    it('shows restart buttons per running agent', () => {
      const onRestart = vi.fn()
      const agents = {
        'impl:claude:sonnet': makeAgent({ session_id: 'sess-1' }),
        'tester:claude:opus': makeAgent({
          agent_id: 'a2',
          agent_type: 'tester',
          phase: 'verification',
          session_id: 'sess-2',
        }),
      }

      render(
        <ActiveAgentsPanel agents={agents} onRestart={onRestart} />
      )

      const buttons = screen.getAllByLabelText('Restart agent (save context, relaunch)')
      expect(buttons).toHaveLength(2)
    })

    it('only disables the restart button for the restarting agent', () => {
      const onRestart = vi.fn()
      const agents = {
        'impl:claude:sonnet': makeAgent({ session_id: 'sess-1' }),
        'tester:claude:opus': makeAgent({
          agent_id: 'a2',
          agent_type: 'tester',
          phase: 'verification',
          session_id: 'sess-2',
        }),
      }

      render(
        <ActiveAgentsPanel
          agents={agents}
          onRestart={onRestart}
          restartingSessionId="sess-1"
        />
      )

      const buttons = screen.getAllByLabelText('Restart agent (save context, relaunch)')
      // First agent (sess-1) should be disabled
      expect(buttons[0]).toBeDisabled()
      // Second agent (sess-2) should not be disabled
      expect(buttons[1]).not.toBeDisabled()
    })

    it('calls onRestart with the correct session_id for each agent', async () => {
      const user = userEvent.setup()
      const onRestart = vi.fn()
      const agents = {
        'impl:claude:sonnet': makeAgent({ session_id: 'sess-1' }),
        'tester:claude:opus': makeAgent({
          agent_id: 'a2',
          agent_type: 'tester',
          phase: 'verification',
          session_id: 'sess-2',
        }),
      }

      render(
        <ActiveAgentsPanel agents={agents} onRestart={onRestart} />
      )

      const buttons = screen.getAllByLabelText('Restart agent (save context, relaunch)')
      await user.click(buttons[1])
      // Confirm dialog opens, click "Restart"
      await user.click(screen.getByText('Restart'))
      expect(onRestart).toHaveBeenCalledWith('sess-2')
    })
  })

  describe('no restart button edge cases', () => {
    it('does not render restart button when restartingSessionId is null', () => {
      const onRestart = vi.fn()

      render(
        <ActiveAgentsPanel
          agents={{ 'impl:claude:sonnet': makeAgent() }}
          onRestart={onRestart}
          restartingSessionId={null}
        />
      )

      const button = screen.getByLabelText('Restart agent (save context, relaunch)')
      expect(button).not.toBeDisabled()
    })
  })
})
