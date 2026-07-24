import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { AgentsTable } from './AgentsTable'
import { renderWithQuery } from '@/test/utils'
import { useStepCursors } from '@/hooks/useStepCursors'
import type { PhaseState, ActiveAgentV4 } from '@/types/workflow'
import type { StepCursorProgress, StepCursorsResponse } from '@/types/stepwise'

vi.mock('@/hooks/useElapsedTime', () => ({
  useTickingClock: vi.fn(),
}))

vi.mock('@/hooks/useStepCursors')

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

function makeCursor(overrides: Partial<StepCursorProgress> = {}): StepCursorProgress {
  return {
    node_id: 'implementation',
    revision: 1,
    current_index: 1,
    total: 3,
    done: false,
    updated_at: '2026-01-01T00:00:00Z',
    steps: [
      { step_id: 's1', title: 'Write tests', status: 'done' },
      { step_id: 's2', title: 'Implement', status: 'active' },
      { step_id: 's3', title: 'Verify', status: 'pending' },
    ],
    ...overrides,
  }
}

function mockCursors(cursors: Record<string, StepCursorProgress>) {
  vi.mocked(useStepCursors).mockImplementation((instanceId?: string) =>
    ({
      data: instanceId ? ({ workflow_instance_id: instanceId, cursors } as StepCursorsResponse) : undefined,
    }) as ReturnType<typeof useStepCursors>
  )
}

describe('AgentsTable stepwise progress', () => {
  it('renders the step progress strip for a running agent when instanceId and a matching cursor are provided', () => {
    mockCursors({ implementation: makeCursor() })
    renderWithQuery(
      <AgentsTable
        phases={makePhases(['implementation'])}
        activeAgents={{ 'impl:claude': makeActive('implementation') }}
        phaseOrder={['implementation']}
        instanceId="wi1"
      />
    )

    expect(screen.getByText('2/3')).toBeInTheDocument()
  })

  it('renders no strip text when the instance has no cursors', () => {
    mockCursors({})
    renderWithQuery(
      <AgentsTable
        phases={makePhases(['implementation'])}
        activeAgents={{ 'impl:claude': makeActive('implementation') }}
        phaseOrder={['implementation']}
        instanceId="wi1"
      />
    )

    expect(screen.queryByText(/^\d+\/\d+$/)).not.toBeInTheDocument()
  })

  it('never mounts the strip (and never calls useStepCursors) when instanceId is not provided', () => {
    mockCursors({ implementation: makeCursor() })
    renderWithQuery(
      <AgentsTable
        phases={makePhases(['implementation'])}
        activeAgents={{ 'impl:claude': makeActive('implementation') }}
        phaseOrder={['implementation']}
      />
    )

    expect(screen.queryByText(/^\d+\/\d+$/)).not.toBeInTheDocument()
  })
})
