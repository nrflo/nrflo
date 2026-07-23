import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { StepProgressStrip } from './StepProgressStrip'
import { renderWithQuery } from '@/test/utils'
import { useStepCursors } from '@/hooks/useStepCursors'
import type { StepCursorProgress, StepCursorsResponse, StepProgressStep } from '@/types/stepwise'

vi.mock('@/hooks/useStepCursors')

function makeStep(overrides: Partial<StepProgressStep> = {}): StepProgressStep {
  return {
    step_id: 'step-1',
    title: 'Write tests',
    status: 'pending',
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
      makeStep({ step_id: 's1', title: 'Write tests', status: 'done' }),
      makeStep({ step_id: 's2', title: 'Implement', status: 'active' }),
      makeStep({ step_id: 's3', title: 'Verify', status: 'pending' }),
    ],
    ...overrides,
  }
}

function mockData(cursors: Record<string, StepCursorProgress>) {
  vi.mocked(useStepCursors).mockReturnValue({
    data: { workflow_instance_id: 'wi1', cursors } as StepCursorsResponse,
  } as ReturnType<typeof useStepCursors>)
}

describe('StepProgressStrip', () => {
  it('returns null when there is no cursor for the node (non-stepwise agents unchanged)', () => {
    mockData({})
    const { container } = renderWithQuery(<StepProgressStrip instanceId="wi1" nodeId="implementation" />)
    expect(container).toBeEmptyDOMElement()
  })

  it('returns null when nodeId is not provided', () => {
    mockData({ implementation: makeCursor() })
    const { container } = renderWithQuery(<StepProgressStrip instanceId="wi1" />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders the N/M chip and one pip per step', () => {
    mockData({ implementation: makeCursor() })
    renderWithQuery(<StepProgressStrip instanceId="wi1" nodeId="implementation" />)

    expect(screen.getByText('2/3')).toBeInTheDocument()
    // 3 steps => 3 pips, rendered as small dots (Badge also has rounded-full,
    // so scope to the pip-specific sizing class).
    const pips = document.querySelectorAll('.w-1\\.5.rounded-full')
    expect(pips).toHaveLength(3)
  })

  it('applies rejected/rotated styling to the corresponding pips', () => {
    mockData({
      implementation: makeCursor({
        steps: [
          makeStep({ step_id: 's1', status: 'rejected_retrying', rejections: 1 }),
          makeStep({ step_id: 's2', status: 'done', rotated: true }),
        ],
      }),
    })
    renderWithQuery(<StepProgressStrip instanceId="wi1" nodeId="implementation" />)

    const pips = document.querySelectorAll('.w-1\\.5.rounded-full')
    expect(pips[0].className).toContain('bg-red-500')
    // rotated wins over done for display styling
    expect(pips[1].className).toContain('bg-purple-500')
  })

  it('tooltip text contains every step title and completed_at timestamps', async () => {
    const user = userEvent.setup()
    mockData({
      implementation: makeCursor({
        steps: [
          makeStep({ step_id: 's1', title: 'Write tests', status: 'done', completed_at: '2026-01-01T00:05:00Z' }),
          makeStep({ step_id: 's2', title: 'Implement', status: 'active' }),
        ],
      }),
    })
    renderWithQuery(<StepProgressStrip instanceId="wi1" nodeId="implementation" />)

    await user.hover(screen.getByText('2/3'))
    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent('Write tests')
    expect(tooltip).toHaveTextContent('Implement')
    expect(tooltip).toHaveTextContent(new Date('2026-01-01T00:05:00Z').toLocaleString())
  })

  it('tooltip line for a step without completed_at omits the "@ timestamp" suffix', async () => {
    const user = userEvent.setup()
    mockData({
      implementation: makeCursor({
        steps: [makeStep({ step_id: 's1', title: 'Implement', status: 'active' })],
      }),
    })
    renderWithQuery(<StepProgressStrip instanceId="wi1" nodeId="implementation" />)

    await user.hover(screen.getByText('2/3'))
    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent('1. Implement — active')
    expect(tooltip.textContent).not.toContain(' @ ')
  })

  it('applies distinct pip colors for pending/active/done states', () => {
    mockData({
      implementation: makeCursor({
        steps: [
          makeStep({ step_id: 's1', status: 'pending' }),
          makeStep({ step_id: 's2', status: 'active' }),
          makeStep({ step_id: 's3', status: 'done' }),
        ],
      }),
    })
    renderWithQuery(<StepProgressStrip instanceId="wi1" nodeId="implementation" />)

    const pips = document.querySelectorAll('.w-1\\.5.rounded-full')
    expect(pips[0].className).toContain('bg-gray-300')
    expect(pips[1].className).toContain('bg-amber-400')
    expect(pips[2].className).toContain('bg-green-500')
    const colors = [pips[0].className, pips[1].className, pips[2].className]
    expect(new Set(colors).size).toBe(3)
  })
})
