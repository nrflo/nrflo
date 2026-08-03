import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AgentsTable } from './AgentsTable'
import type { PhaseState, ActiveAgentV4, AgentHistoryEntry } from '@/types/workflow'

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

describe('AgentsTable effort column', () => {
  it('renders Effort column header', () => {
    render(
      <AgentsTable phases={makePhases(['impl'])} activeAgents={{}} phaseOrder={['impl']} />
    )
    expect(screen.getByText('Effort')).toBeInTheDocument()
  })

  it('shows resolved_effort for a running agent', () => {
    render(
      <AgentsTable
        phases={makePhases(['impl'])}
        activeAgents={{ 'impl:claude': makeActive('impl', { resolved_effort: 'high' }) }}
        phaseOrder={['impl']}
      />
    )
    expect(screen.getByText('high')).toBeInTheDocument()
  })

  it('falls back to history resolved_effort when no active agent', () => {
    render(
      <AgentsTable
        phases={makePhases(['impl'])}
        activeAgents={{}}
        agentHistory={[makeHistory('impl', { resolved_effort: 'medium' })]}
        phaseOrder={['impl']}
      />
    )
    expect(screen.getByText('medium')).toBeInTheDocument()
  })

  it('shows em-dash when resolved_effort is absent on both active and history', () => {
    render(
      <AgentsTable
        phases={makePhases(['impl'])}
        activeAgents={{ 'impl:claude': makeActive('impl') }}
        phaseOrder={['impl']}
      />
    )
    expect(screen.getAllByText('—').length).toBeGreaterThan(0)
  })
})
