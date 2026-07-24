import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TimeBreakdownBar } from './TimeBreakdownBar'
import type { TimeBuckets } from './types'

function makeBuckets(overrides: Partial<TimeBuckets> = {}): TimeBuckets {
  return {
    thinking_sec: 10,
    tool_arg_sec: 20,
    text_sec: 30,
    tool_wait_sec: 40,
    ...overrides,
  }
}

describe('TimeBreakdownBar', () => {
  it('renders four segments with widths proportional to totals', () => {
    render(<TimeBreakdownBar buckets={makeBuckets()} />)
    expect(screen.getByTestId('trace-lane-timebar-thinking_sec').style.width).toBe('10%')
    expect(screen.getByTestId('trace-lane-timebar-tool_arg_sec').style.width).toBe('20%')
    expect(screen.getByTestId('trace-lane-timebar-text_sec').style.width).toBe('30%')
    expect(screen.getByTestId('trace-lane-timebar-tool_wait_sec').style.width).toBe('40%')
  })

  it('lists per-bucket seconds and percentages in the tooltip', async () => {
    const user = userEvent.setup()
    render(<TimeBreakdownBar buckets={makeBuckets()} />)
    await user.hover(screen.getByTestId('trace-lane-timebar'))
    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent('Thinking: 10.0s (10%)')
    expect(tooltip).toHaveTextContent('Tool args: 20.0s (20%)')
    expect(tooltip).toHaveTextContent('Text: 30.0s (30%)')
    expect(tooltip).toHaveTextContent('Tool wait: 40.0s (40%)')
  })

  it('returns null when buckets is undefined', () => {
    render(<TimeBreakdownBar buckets={undefined} />)
    expect(screen.queryByTestId('trace-lane-timebar')).not.toBeInTheDocument()
  })

  it('returns null when all buckets are zero', () => {
    render(
      <TimeBreakdownBar
        buckets={{ thinking_sec: 0, tool_arg_sec: 0, text_sec: 0, tool_wait_sec: 0 }}
      />
    )
    expect(screen.queryByTestId('trace-lane-timebar')).not.toBeInTheDocument()
  })

  it('skips a zero-value bucket segment while still rendering the others', () => {
    render(<TimeBreakdownBar buckets={makeBuckets({ thinking_sec: 0 })} />)
    expect(screen.queryByTestId('trace-lane-timebar-thinking_sec')).not.toBeInTheDocument()
    expect(screen.getByTestId('trace-lane-timebar-tool_arg_sec')).toBeInTheDocument()
  })
})
