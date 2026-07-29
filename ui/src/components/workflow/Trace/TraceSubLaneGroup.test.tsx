import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { TraceSubLaneGroup } from './TraceSubLaneGroup'
import type { SubLaneGroup } from './subLanes'
import type { TraceLaneData } from './types'

const domain = { min: Date.parse('2025-01-01T00:00:00Z'), max: Date.parse('2025-01-01T00:10:00Z') }

function makeWorkerLane(overrides: Partial<TraceLaneData> = {}): TraceLaneData {
  return {
    lane_id: 'w1',
    phase: 'delegate:w1',
    layer: -1,
    agent_type: 'extractor',
    status: 'completed',
    segments: [
      { session_id: 'w1', status: 'completed', started_at: '2025-01-01T00:01:00Z', ended_at: '2025-01-01T00:02:00Z' },
    ],
    markers: [],
    ...overrides,
  }
}

function makeGroup(overrides: Partial<SubLaneGroup> = {}): SubLaneGroup {
  return {
    key: 'd1',
    kind: 'delegate',
    label: '⤷ delegate ×2',
    lanes: [makeWorkerLane({ lane_id: 'w1' }), makeWorkerLane({ lane_id: 'w2' })],
    ...overrides,
  }
}

describe('TraceSubLaneGroup', () => {
  it('renders collapsed by default with the worker count in the summary label, and expands on click', () => {
    render(
      <TraceSubLaneGroup group={makeGroup()} domain={domain} widthPx={1000} activeTypes={new Set(['tool'])} />
    )

    expect(screen.getByTestId('trace-sublane-group')).toBeInTheDocument()
    expect(screen.getByText('⤷ delegate ×2')).toBeInTheDocument()
    expect(screen.queryAllByTestId('trace-sublane')).toHaveLength(0)
    expect(screen.queryAllByTestId('trace-lane')).toHaveLength(0)

    const toggle = screen.getByRole('button', { expanded: false })
    fireEvent.click(toggle)

    expect(screen.getAllByTestId('trace-sublane')).toHaveLength(2)
    expect(screen.getAllByTestId('trace-lane')).toHaveLength(2)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')

    fireEvent.click(toggle)
    expect(screen.queryAllByTestId('trace-sublane')).toHaveLength(0)
  })

  it('filters nested lane markers through activeTypes and wires onSelect through to the worker session', () => {
    const onSelect = vi.fn()
    const group = makeGroup({
      lanes: [
        makeWorkerLane({
          lane_id: 'w1',
          phase: 'delegate:w1',
          markers: [
            { type: 'tool', at: '2025-01-01T00:01:30Z', label: '[Bash] ls' },
            { type: 'error', at: '2025-01-01T00:01:40Z', label: 'boom' },
          ],
        }),
      ],
    })
    render(
      <TraceSubLaneGroup
        group={group}
        domain={domain}
        widthPx={1000}
        activeTypes={new Set(['tool'])}
        onSelect={onSelect}
      />
    )
    fireEvent.click(screen.getByRole('button', { expanded: false }))

    expect(screen.getAllByTestId('trace-marker')).toHaveLength(1)

    fireEvent.click(screen.getAllByTestId('trace-segment')[0].querySelector('button')!)
    expect(onSelect).toHaveBeenCalledWith('delegate:w1', 'w1')
  })
})
