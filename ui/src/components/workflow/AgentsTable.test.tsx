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

describe('AgentsTable', () => {
  describe('row rendering', () => {
    it('renders one row per phase from phaseOrder', () => {
      render(
        <AgentsTable
          phases={makePhases(['investigation', 'implementation', 'verification'])}
          activeAgents={{}}
          phaseOrder={['investigation', 'implementation', 'verification']}
        />
      )
      expect(screen.getByText('investigation')).toBeInTheDocument()
      expect(screen.getByText('implementation')).toBeInTheDocument()
      expect(screen.getByText('verification')).toBeInTheDocument()
    })

    it('replaces underscores with spaces in phase name display', () => {
      render(
        <AgentsTable
          phases={makePhases(['setup_analyzer'])}
          activeAgents={{}}
          phaseOrder={['setup_analyzer']}
        />
      )
      expect(screen.getByText('setup analyzer')).toBeInTheDocument()
    })

    it('sorts rows ascending by phaseLayers then phaseOrder index', () => {
      render(
        <AgentsTable
          phases={makePhases(['phase_a', 'phase_b', 'phase_c'])}
          activeAgents={{}}
          phaseOrder={['phase_a', 'phase_b', 'phase_c']}
          phaseLayers={{ phase_a: 2, phase_b: 0, phase_c: 1 }}
        />
      )
      const rows = screen.getAllByRole('row')
      // rows[0] = header, rows[1..3] = data rows ordered by layer asc
      expect(rows[1]).toHaveTextContent('phase b')
      expect(rows[2]).toHaveTextContent('phase c')
      expect(rows[3]).toHaveTextContent('phase a')
    })
  })

  describe('pending phase', () => {
    it('shows pending status', () => {
      render(
        <AgentsTable
          phases={makePhases(['setup'])}
          activeAgents={{}}
          phaseOrder={['setup']}
        />
      )
      expect(screen.getByText('pending')).toBeInTheDocument()
    })

    it('shows dashes for model, attempts, context left, and duration', () => {
      render(
        <AgentsTable
          phases={makePhases(['setup'])}
          activeAgents={{}}
          phaseOrder={['setup']}
        />
      )
      expect(screen.getAllByText('-')).toHaveLength(4)
    })
  })

  describe('running phase', () => {
    it('shows running status', () => {
      render(
        <AgentsTable
          phases={makePhases(['implementation'])}
          activeAgents={{ 'impl:claude': makeActive('implementation') }}
          phaseOrder={['implementation']}
        />
      )
      expect(screen.getByText('running')).toBeInTheDocument()
    })

    it('shows animate-pulse icon for running status', () => {
      render(
        <AgentsTable
          phases={makePhases(['implementation'])}
          activeAgents={{ 'impl:claude': makeActive('implementation') }}
          phaseOrder={['implementation']}
        />
      )
      expect(document.querySelector('.animate-pulse')).toBeInTheDocument()
    })

    it('shows model id for running agent', () => {
      render(
        <AgentsTable
          phases={makePhases(['implementation'])}
          activeAgents={{ 'impl:claude': makeActive('implementation', { model_id: 'claude-opus-4-7' }) }}
          phaseOrder={['implementation']}
        />
      )
      expect(screen.getByText('claude-opus-4-7')).toBeInTheDocument()
    })
  })

  describe('completed phase', () => {
    it('shows completed status for pass result', () => {
      render(
        <AgentsTable
          phases={makePhases(['implementation'])}
          activeAgents={{}}
          agentHistory={[makeHistory('implementation', { result: 'pass' })]}
          phaseOrder={['implementation']}
        />
      )
      expect(screen.getByText('completed')).toBeInTheDocument()
    })

    it('shows failed status for fail result', () => {
      render(
        <AgentsTable
          phases={makePhases(['implementation'])}
          activeAgents={{}}
          agentHistory={[makeHistory('implementation', { result: 'fail' })]}
          phaseOrder={['implementation']}
        />
      )
      expect(screen.getByText('failed')).toBeInTheDocument()
    })

    it('shows skipped status for skipped result', () => {
      render(
        <AgentsTable
          phases={makePhases(['implementation'])}
          activeAgents={{}}
          agentHistory={[makeHistory('implementation', { result: 'skipped' })]}
          phaseOrder={['implementation']}
        />
      )
      expect(screen.getByText('skipped')).toBeInTheDocument()
    })

    it('shows formatted duration for completed phase', () => {
      render(
        <AgentsTable
          phases={makePhases(['implementation'])}
          activeAgents={{}}
          agentHistory={[makeHistory('implementation', {
            started_at: '2026-01-01T00:00:00Z',
            ended_at: '2026-01-01T00:05:00Z',
          })]}
          phaseOrder={['implementation']}
        />
      )
      expect(screen.getByText('5m')).toBeInTheDocument()
    })
  })

  describe('click handler', () => {
    it('calls onAgentSelect with only phaseName for pending rows', async () => {
      const user = userEvent.setup()
      const onAgentSelect = vi.fn()
      render(
        <AgentsTable
          phases={makePhases(['setup'])}
          activeAgents={{}}
          phaseOrder={['setup']}
          onAgentSelect={onAgentSelect}
        />
      )
      await user.click(screen.getByText('setup'))
      expect(onAgentSelect).toHaveBeenCalledWith({ phaseName: 'setup' })
    })

    it('calls onAgentSelect with historyEntry and session for completed rows', async () => {
      const user = userEvent.setup()
      const onAgentSelect = vi.fn()
      const history = makeHistory('implementation')
      const session = makeSession()
      render(
        <AgentsTable
          phases={makePhases(['implementation'])}
          activeAgents={{}}
          agentHistory={[history]}
          sessions={[session]}
          phaseOrder={['implementation']}
          onAgentSelect={onAgentSelect}
        />
      )
      await user.click(screen.getByText('implementation'))
      expect(onAgentSelect).toHaveBeenCalledWith({
        phaseName: 'implementation',
        agent: undefined,
        historyEntry: history,
        session,
      })
    })

    it('calls onAgentSelect with agent for running rows', async () => {
      const user = userEvent.setup()
      const onAgentSelect = vi.fn()
      const active = makeActive('implementation')
      render(
        <AgentsTable
          phases={makePhases(['implementation'])}
          activeAgents={{ 'impl:claude': active }}
          phaseOrder={['implementation']}
          onAgentSelect={onAgentSelect}
        />
      )
      await user.click(screen.getByText('implementation'))
      expect(onAgentSelect).toHaveBeenCalledWith({
        phaseName: 'implementation',
        agent: active,
        historyEntry: undefined,
        session: undefined,
      })
    })

    it('does not throw when onAgentSelect is not provided', async () => {
      const user = userEvent.setup()
      render(
        <AgentsTable
          phases={makePhases(['setup'])}
          activeAgents={{}}
          phaseOrder={['setup']}
        />
      )
      await expect(user.click(screen.getByText('setup'))).resolves.not.toThrow()
    })
  })

  describe('multi-agent fan-in', () => {
    it('shows latest history entry by ended_at when multiple entries share a phase', () => {
      render(
        <AgentsTable
          phases={makePhases(['verification'])}
          activeAgents={{}}
          agentHistory={[
            makeHistory('verification', { model_id: 'claude-sonnet-4-5', ended_at: '2026-01-01T00:01:00Z' }),
            makeHistory('verification', { model_id: 'claude-opus-4-7', ended_at: '2026-01-01T00:02:00Z' }),
          ]}
          phaseOrder={['verification']}
        />
      )
      expect(screen.getByText('claude-opus-4-7')).toBeInTheDocument()
      expect(screen.queryByText('claude-sonnet-4-5')).not.toBeInTheDocument()
    })
  })
})
