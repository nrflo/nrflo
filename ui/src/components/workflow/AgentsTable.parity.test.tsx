import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentsTable } from './AgentsTable'
import type { PhaseState, ActiveAgentV4, AgentHistoryEntry, AgentSession } from '@/types/workflow'

vi.mock('@/hooks/useElapsedTime', () => ({
  useTickingClock: vi.fn(),
}))

function makePhases(names: string[]): Record<string, PhaseState> {
  return Object.fromEntries(names.map(n => [n, { status: 'pending' as const }]))
}

function makeActive(phaseName: string, overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_type: phaseName,
    phase: phaseName,
    model_id: 'claude-sonnet-4-6',
    session_id: 'session-1',
    started_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeHistory(phaseName: string, overrides: Partial<AgentHistoryEntry> = {}): AgentHistoryEntry {
  return {
    agent_id: 'a1',
    agent_type: phaseName,
    phase: phaseName,
    session_id: 'session-1',
    model_id: 'claude-sonnet-4-6',
    result: 'pass',
    started_at: '2026-01-01T00:00:00Z',
    ended_at: '2026-01-01T00:05:00Z',
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
    agent_type: 'implementation',
    model_id: 'claude-sonnet-4-6',
    status: 'completed',
    message_count: 10,
    restart_count: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:05:00Z',
    ...overrides,
  }
}

describe('AgentsTable parity signals', () => {
  describe('run mode column', () => {
    it('renders Run mode column header', () => {
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{}}
          phaseOrder={['impl']}
        />
      )
      expect(screen.getByText('Run mode')).toBeInTheDocument()
    })

    it.each([
      ['api', 'api'],
      ['script', 'script'],
    ] as const)('renders effective_mode="%s" as "%s" for running agent', (mode, label) => {
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{ 'impl:claude': makeActive('impl', { effective_mode: mode }) }}
          phaseOrder={['impl']}
        />
      )
      expect(screen.getByText(label)).toBeInTheDocument()
    })

    it('renders effective_mode cli_interactive as "cli interactive" for running agent', () => {
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{ 'impl:claude': makeActive('impl', { effective_mode: 'cli_interactive' }) }}
          phaseOrder={['impl']}
        />
      )
      expect(screen.getByText('cli interactive')).toBeInTheDocument()
    })

    it('renders effective_mode for history entry', () => {
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{}}
          agentHistory={[makeHistory('impl', { effective_mode: 'api' })]}
          phaseOrder={['impl']}
        />
      )
      expect(screen.getByText('api')).toBeInTheDocument()
    })

    it('renders em-dash when effective_mode is absent', () => {
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{ 'impl:claude': makeActive('impl') }}
          phaseOrder={['impl']}
        />
      )
      expect(screen.getByText('—')).toBeInTheDocument()
    })
  })

  describe('interactive control indicator', () => {
    it('shows (interactive control) when session status is user_interactive', () => {
      const session = makeSession({ status: 'user_interactive' })
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{ 'impl:claude': makeActive('impl', { session_id: 'session-1' }) }}
          agentHistory={[makeHistory('impl', { session_id: 'session-1', result: 'pass' })]}
          sessions={[session]}
          phaseOrder={['impl']}
        />
      )
      expect(screen.getByText('(interactive control)')).toBeInTheDocument()
    })

    it('does not show (interactive control) when session status is completed', () => {
      const session = makeSession({ status: 'completed' })
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{ 'impl:claude': makeActive('impl', { session_id: 'session-1' }) }}
          agentHistory={[makeHistory('impl', { session_id: 'session-1', result: 'pass' })]}
          sessions={[session]}
          phaseOrder={['impl']}
        />
      )
      expect(screen.queryByText('(interactive control)')).not.toBeInTheDocument()
    })
  })

  describe('restart threshold warning', () => {
    it('shows AlertTriangle svg when running and context_left <= restart_threshold', () => {
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{ 'impl:claude': makeActive('impl', { context_left: 20, restart_threshold: 25 }) }}
          phaseOrder={['impl']}
        />
      )
      const svg = document.querySelector('svg.text-amber-500')
      expect(svg).not.toBeNull()
      expect(screen.getByText('20%')).toBeInTheDocument()
    })

    it('shows AlertTriangle svg when context_left equals restart_threshold (boundary)', () => {
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{ 'impl:claude': makeActive('impl', { context_left: 25, restart_threshold: 25 }) }}
          phaseOrder={['impl']}
        />
      )
      expect(document.querySelector('svg.text-amber-500')).not.toBeNull()
    })

    it('does not show AlertTriangle when context_left > restart_threshold', () => {
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{ 'impl:claude': makeActive('impl', { context_left: 80, restart_threshold: 25 }) }}
          phaseOrder={['impl']}
        />
      )
      expect(document.querySelector('svg.text-amber-500')).toBeNull()
    })

    it('does not show AlertTriangle for completed phase even below threshold', () => {
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{}}
          agentHistory={[makeHistory('impl', { context_left: 10, restart_threshold: 25 })]}
          phaseOrder={['impl']}
        />
      )
      expect(document.querySelector('svg.text-amber-500')).toBeNull()
    })
  })

  describe('attempts cell tooltip', () => {
    it('shows count+1 without tooltip when restart_count=0', () => {
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{ 'impl:claude': makeActive('impl', { restart_count: 0 }) }}
          phaseOrder={['impl']}
        />
      )
      const countEl = screen.getByText('1')
      expect(countEl.className).not.toContain('underline')
    })

    it('shows count+1 with dotted underline (tooltip trigger) when restart_count>0', () => {
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{ 'impl:claude': makeActive('impl', { restart_count: 2 }) }}
          phaseOrder={['impl']}
        />
      )
      const countEl = screen.getByText('3')
      expect(countEl.className).toContain('underline')
      expect(countEl.className).toContain('decoration-dotted')
    })

    it('shows restart reasons in tooltip on hover', async () => {
      const user = userEvent.setup()
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{
            'impl:claude': makeActive('impl', {
              restart_count: 2,
              restart_details: [
                { reason: 'low_context', duration_sec: 120, context_left: 12, message_count: 50 },
                { reason: 'explicit', duration_sec: 60, context_left: 30, message_count: 20 },
              ],
            }),
          }}
          phaseOrder={['impl']}
        />
      )
      await user.hover(screen.getByText('3'))
      const tooltip = await screen.findByRole('tooltip')
      expect(tooltip).toHaveTextContent('Low context')
      expect(tooltip).toHaveTextContent('Manual continue')
    })
  })

  describe('nudge badge', () => {
    it('shows ⏰N/5 badge when running and nudge_count>0', () => {
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{ 'impl:claude': makeActive('impl', { nudge_count: 3 }) }}
          phaseOrder={['impl']}
        />
      )
      expect(screen.getByText('⏰3/5')).toBeInTheDocument()
    })

    it('does not show nudge badge when nudge_count=0', () => {
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{ 'impl:claude': makeActive('impl', { nudge_count: 0 }) }}
          phaseOrder={['impl']}
        />
      )
      expect(screen.queryByText(/⏰/)).not.toBeInTheDocument()
    })

    it('does not show nudge badge for completed phase even with nudge_count', () => {
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{}}
          agentHistory={[makeHistory('impl', { nudge_count: 2 })]}
          phaseOrder={['impl']}
        />
      )
      expect(screen.queryByText(/⏰/)).not.toBeInTheDocument()
    })
  })

  describe('tag badge', () => {
    it('shows emerald tag badge next to phase name for running agent', () => {
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{ 'impl:claude': makeActive('impl', { tag: 'frontend' }) }}
          phaseOrder={['impl']}
        />
      )
      expect(screen.getByText('frontend')).toBeInTheDocument()
      expect(screen.getByText('impl')).toBeInTheDocument()
    })

    it('shows tag badge for history entry', () => {
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{}}
          agentHistory={[makeHistory('impl', { tag: 'backend' })]}
          phaseOrder={['impl']}
        />
      )
      expect(screen.getByText('backend')).toBeInTheDocument()
    })

    it('renders no badge when tag is absent', () => {
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{ 'impl:claude': makeActive('impl') }}
          phaseOrder={['impl']}
        />
      )
      const badge = document.querySelector('.border-emerald-300')
      expect(badge).toBeNull()
    })
  })

  describe('retry button', () => {
    it('renders retry button when result=fail and workflowStatus=failed and onRetryFailed provided', () => {
      const onRetryFailed = vi.fn()
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{}}
          agentHistory={[makeHistory('impl', { result: 'fail', session_id: 'session-1' })]}
          phaseOrder={['impl']}
          workflowStatus="failed"
          onRetryFailed={onRetryFailed}
        />
      )
      expect(document.querySelector('button svg.lucide-refresh-cw')).not.toBeNull()
    })

    it('does not render retry button when workflowStatus=running', () => {
      const onRetryFailed = vi.fn()
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{}}
          agentHistory={[makeHistory('impl', { result: 'fail', session_id: 'session-1' })]}
          phaseOrder={['impl']}
          workflowStatus="running"
          onRetryFailed={onRetryFailed}
        />
      )
      expect(document.querySelector('button svg.lucide-refresh-cw')).toBeNull()
    })

    it('does not render retry button when result is not fail', () => {
      const onRetryFailed = vi.fn()
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{}}
          agentHistory={[makeHistory('impl', { result: 'pass', session_id: 'session-1' })]}
          phaseOrder={['impl']}
          workflowStatus="failed"
          onRetryFailed={onRetryFailed}
        />
      )
      expect(document.querySelector('button svg.lucide-refresh-cw')).toBeNull()
    })

    it('does not render retry button when onRetryFailed is undefined', () => {
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{}}
          agentHistory={[makeHistory('impl', { result: 'fail', session_id: 'session-1' })]}
          phaseOrder={['impl']}
          workflowStatus="failed"
        />
      )
      expect(document.querySelector('button svg.lucide-refresh-cw')).toBeNull()
    })

    it('opens ConfirmDialog on retry button click and calls onRetryFailed on confirm', async () => {
      const user = userEvent.setup()
      const onRetryFailed = vi.fn()
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{}}
          agentHistory={[makeHistory('impl', { result: 'fail', session_id: 'session-retry' })]}
          phaseOrder={['impl']}
          workflowStatus="failed"
          onRetryFailed={onRetryFailed}
        />
      )
      const retryBtn = document.querySelector('button svg.lucide-refresh-cw')!.closest('button')!
      await user.click(retryBtn)
      expect(screen.getByText('Retry Failed Agent')).toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: 'Retry' }))
      expect(onRetryFailed).toHaveBeenCalledWith('session-retry')
    })

    it('does not call onAgentSelect when retry button is clicked (stops propagation)', async () => {
      const user = userEvent.setup()
      const onRetryFailed = vi.fn()
      const onAgentSelect = vi.fn()
      render(
        <AgentsTable
          phases={makePhases(['impl'])}
          activeAgents={{}}
          agentHistory={[makeHistory('impl', { result: 'fail', session_id: 'session-1' })]}
          phaseOrder={['impl']}
          workflowStatus="failed"
          onRetryFailed={onRetryFailed}
          onAgentSelect={onAgentSelect}
        />
      )
      const retryBtn = document.querySelector('button svg.lucide-refresh-cw')!.closest('button')!
      await user.click(retryBtn)
      expect(onAgentSelect).not.toHaveBeenCalled()
    })
  })
})
