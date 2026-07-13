import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AgentsTable } from './AgentsTable'
import type { PhaseState } from '@/types/workflow'

vi.mock('@/hooks/useElapsedTime', () => ({
  useTickingClock: vi.fn(),
}))

function makePhases(names: string[]): Record<string, PhaseState> {
  return Object.fromEntries(names.map(n => [n, { status: 'pending' as const }]))
}

describe('AgentsTable materialized plan node rendering', () => {
  it('renders a "planner" phase key as a regular row (legal materialized node id)', () => {
    render(
      <AgentsTable
        phases={makePhases(['planner', 'analyzer'])}
        activeAgents={{}}
        phaseOrder={['planner', 'analyzer']}
      />
    )
    expect(screen.getByText('planner')).toBeInTheDocument()
    expect(screen.getByText('analyzer')).toBeInTheDocument()
  })

  it('renders a "planning" phase key as a regular row (legal materialized node id)', () => {
    render(
      <AgentsTable
        phases={makePhases(['planning', 'analyzer'])}
        activeAgents={{}}
        phaseOrder={['planning', 'analyzer']}
      />
    )
    expect(screen.getByText('planning')).toBeInTheDocument()
    expect(screen.getByText('analyzer')).toBeInTheDocument()
  })

  it('renders all rows when no planner phase is present', () => {
    render(
      <AgentsTable
        phases={makePhases(['analyzer', 'implementor'])}
        activeAgents={{}}
        phaseOrder={['analyzer', 'implementor']}
      />
    )
    expect(screen.getByText('analyzer')).toBeInTheDocument()
    expect(screen.getByText('implementor')).toBeInTheDocument()
  })
})
