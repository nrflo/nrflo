import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
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

function getDataRows(): NodeListOf<HTMLTableRowElement> {
  return document.querySelectorAll('tbody tr')
}

describe('current running layer highlight', () => {
  it('highlights layer-0 running row; layer-1 pending row is not highlighted', () => {
    render(
      <AgentsTable
        phases={makePhases(['setup_analyzer', 'implementor'])}
        activeAgents={{ 'setup:claude': makeActive('setup_analyzer') }}
        phaseOrder={['setup_analyzer', 'implementor']}
        phaseLayers={{ setup_analyzer: 0, implementor: 1 }}
      />
    )
    const rows = getDataRows()
    expect(rows[0].className).toContain('bg-yellow-50')
    expect(rows[1].className).not.toContain('bg-yellow-50')
  })

  it('highlights layer-1 running row when layer-0 is already completed', () => {
    render(
      <AgentsTable
        phases={makePhases(['setup_analyzer', 'implementor'])}
        activeAgents={{ 'impl:claude': makeActive('implementor') }}
        agentHistory={[makeHistory('setup_analyzer', { result: 'pass' })]}
        phaseOrder={['setup_analyzer', 'implementor']}
        phaseLayers={{ setup_analyzer: 0, implementor: 1 }}
      />
    )
    const rows = getDataRows()
    expect(rows[0].className).not.toContain('bg-yellow-50')
    expect(rows[1].className).toContain('bg-yellow-50')
  })

  it('highlights no rows when no agents are running', () => {
    render(
      <AgentsTable
        phases={makePhases(['setup_analyzer', 'implementor'])}
        activeAgents={{}}
        phaseOrder={['setup_analyzer', 'implementor']}
        phaseLayers={{ setup_analyzer: 0, implementor: 1 }}
      />
    )
    const rows = getDataRows()
    rows.forEach(row => {
      expect(row.className).not.toContain('bg-yellow-50')
    })
  })

  it('highlights only the running row when two phases share the same layer', () => {
    render(
      <AgentsTable
        phases={makePhases(['agent_a', 'agent_b'])}
        activeAgents={{ 'a:claude': makeActive('agent_a') }}
        phaseOrder={['agent_a', 'agent_b']}
        phaseLayers={{ agent_a: 0, agent_b: 0 }}
      />
    )
    const rows = getDataRows()
    expect(rows[0].className).toContain('bg-yellow-50')
    expect(rows[1].className).not.toContain('bg-yellow-50')
  })
})
