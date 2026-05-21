import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { AgentsTable } from './AgentsTable'
import type { PhaseState } from '@/types/workflow'

vi.mock('@/hooks/useElapsedTime', () => ({
  useTickingClock: vi.fn(),
}))

function makePhases(names: string[]): Record<string, PhaseState> {
  return Object.fromEntries(names.map(n => [n, { status: 'pending' as const }]))
}

function getSeparatorRows(): HTMLTableRowElement[] {
  return Array.from(document.querySelectorAll<HTMLTableRowElement>('tbody tr[aria-hidden="true"]'))
}

describe('AgentsTable separator rows', () => {
  it('renders one separator row between phases with different layers', () => {
    render(
      <AgentsTable
        phases={makePhases(['phase_a', 'phase_b'])}
        activeAgents={{}}
        phaseOrder={['phase_a', 'phase_b']}
        phaseLayers={{ phase_a: 0, phase_b: 1 }}
      />
    )
    const separators = getSeparatorRows()
    expect(separators).toHaveLength(1)
  })

  it('separator cell carries bg-muted-foreground/30 class', () => {
    render(
      <AgentsTable
        phases={makePhases(['phase_a', 'phase_b'])}
        activeAgents={{}}
        phaseOrder={['phase_a', 'phase_b']}
        phaseLayers={{ phase_a: 0, phase_b: 1 }}
      />
    )
    const [separator] = getSeparatorRows()
    const cell = separator.querySelector('td')!
    expect(cell.className).toContain('bg-muted-foreground/30')
  })

  it('renders two separators for three distinct layers', () => {
    render(
      <AgentsTable
        phases={makePhases(['phase_a', 'phase_b', 'phase_c'])}
        activeAgents={{}}
        phaseOrder={['phase_a', 'phase_b', 'phase_c']}
        phaseLayers={{ phase_a: 0, phase_b: 1, phase_c: 2 }}
      />
    )
    expect(getSeparatorRows()).toHaveLength(2)
  })

  it('renders no separator rows when all phases share the same layer', () => {
    render(
      <AgentsTable
        phases={makePhases(['phase_a', 'phase_b'])}
        activeAgents={{}}
        phaseOrder={['phase_a', 'phase_b']}
        phaseLayers={{ phase_a: 0, phase_b: 0 }}
      />
    )
    expect(getSeparatorRows()).toHaveLength(0)
  })

  it('renders no separator rows when phaseLayers is undefined', () => {
    render(
      <AgentsTable
        phases={makePhases(['phase_a', 'phase_b'])}
        activeAgents={{}}
        phaseOrder={['phase_a', 'phase_b']}
      />
    )
    expect(getSeparatorRows()).toHaveLength(0)
  })
})
