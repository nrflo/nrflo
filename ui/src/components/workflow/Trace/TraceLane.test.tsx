import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { TraceLane } from './TraceLane'
import type { TraceLaneData } from './types'

const domain = { min: Date.parse('2025-01-01T00:00:00Z'), max: Date.parse('2025-01-01T00:10:00Z') }

function makeLane(overrides: Partial<TraceLaneData> = {}): TraceLaneData {
  return {
    lane_id: 's1',
    phase: 'builder',
    layer: 1,
    agent_type: 'builder',
    model_id: 'claude:opus',
    status: 'running',
    segments: [
      {
        session_id: 's1',
        status: 'continued',
        result: 'continue',
        started_at: '2025-01-01T00:00:00Z',
        ended_at: '2025-01-01T00:05:00Z',
      },
      { session_id: 's2', status: 'running', started_at: '2025-01-01T00:05:00Z' },
    ],
    restarts: [{ reason: 'low_context', duration_sec: 300, message_count: 12 }],
    markers: [],
    ...overrides,
  }
}

describe('TraceLane', () => {
  it('positions segments by percentage and extends running segment to 100%', () => {
    render(<TraceLane lane={makeLane()} markers={[]} domain={domain} widthPx={1000} />)
    const segments = screen.getAllByTestId('trace-segment')
    expect(segments).toHaveLength(2)
    expect(segments[0].style.left).toBe('0%')
    expect(segments[0].style.width).toBe('50%')
    expect(segments[1].style.left).toBe('50%')
    expect(segments[1].style.width).toBe('50%') // ended_at null → to domain max
    expect(segments[1].querySelector('button')!.className).toContain('animate-pulse')
  })

  it('shows restart count and calls onSelect with the clicked session', () => {
    const onSelect = vi.fn()
    render(<TraceLane lane={makeLane()} markers={[]} domain={domain} widthPx={1000} onSelect={onSelect} />)
    expect(screen.getByTestId('trace-lane-restarts')).toHaveTextContent('↻1')
    fireEvent.click(screen.getAllByTestId('trace-segment')[0].querySelector('button')!)
    expect(onSelect).toHaveBeenCalledWith('s1')
    fireEvent.click(screen.getByText('builder'))
    expect(onSelect).toHaveBeenCalledWith('s2') // lane label → last segment
  })

  it('shows nudge and stop-block badges when counters are set', () => {
    render(
      <TraceLane
        lane={makeLane({ nudge_count: 2, stop_block_count: 1 })}
        markers={[]}
        domain={domain}
        widthPx={1000}
      />
    )
    expect(screen.getByTestId('trace-lane-nudges')).toHaveTextContent('nudged×2')
    expect(screen.getByTestId('trace-lane-stopblocks')).toHaveTextContent('blocked×1')
  })

  it('renders the time breakdown bar when lane has time_buckets', () => {
    render(
      <TraceLane
        lane={makeLane({
          time_buckets: { thinking_sec: 5, tool_arg_sec: 5, text_sec: 5, tool_wait_sec: 5 },
        })}
        markers={[]}
        domain={domain}
        widthPx={1000}
      />
    )
    expect(screen.getByTestId('trace-lane-timebar')).toBeInTheDocument()
  })

  it('omits the time breakdown bar when time_buckets is absent', () => {
    render(<TraceLane lane={makeLane()} markers={[]} domain={domain} widthPx={1000} />)
    expect(screen.queryByTestId('trace-lane-timebar')).not.toBeInTheDocument()
  })

  it('positions segments identically in the nested (indent) variant', () => {
    render(<TraceLane lane={makeLane()} markers={[]} domain={domain} widthPx={1000} indent />)
    const segments = screen.getAllByTestId('trace-segment')
    expect(segments[0].style.left).toBe('0%')
    expect(segments[0].style.width).toBe('50%')
    expect(segments[1].style.left).toBe('50%')
    expect(segments[1].style.width).toBe('50%')
  })

  it('skips segments without a parsable start', () => {
    render(
      <TraceLane
        lane={makeLane({ segments: [{ session_id: 's1', status: 'running', started_at: null }] })}
        markers={[]}
        domain={domain}
        widthPx={1000}
      />
    )
    expect(screen.queryAllByTestId('trace-segment')).toHaveLength(0)
  })
})
