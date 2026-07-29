import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, fireEvent } from '@testing-library/react'
import { renderWithQuery } from '@/test/utils'
import { TraceView } from './TraceView'
import type { WorkflowTraceResponse } from './types'

const mockUseTrace = vi.fn()
vi.mock('@/hooks/useTrace', async (importOriginal) => {
  const orig = await importOriginal<typeof import('@/hooks/useTrace')>()
  return { ...orig, useTrace: (iid: string | undefined) => mockUseTrace(iid) }
})
vi.mock('@/hooks/useWebSocketSubscription', () => ({
  useWebSocketEvent: vi.fn(),
  useWebSocketSubscription: vi.fn(() => ({ isConnected: true })),
}))

function makeTrace(overrides: Partial<WorkflowTraceResponse> = {}): WorkflowTraceResponse {
  return {
    instance_id: 'wfi-1',
    project_id: 'p',
    workflow: 'feature',
    status: 'active',
    started_at: '2025-01-01T00:00:00Z',
    layers: [
      { layer: 0, phases: ['analyzer'], started_at: '2025-01-01T00:00:01Z', ended_at: '2025-01-01T00:01:00Z' },
      { layer: 1, phases: ['builder'], started_at: '2025-01-01T00:01:00Z' },
    ],
    lanes: [
      {
        lane_id: 's1',
        phase: 'analyzer',
        layer: 0,
        agent_type: 'analyzer',
        status: 'completed',
        result: 'pass',
        segments: [
          { session_id: 's1', status: 'completed', result: 'pass', started_at: '2025-01-01T00:00:01Z', ended_at: '2025-01-01T00:01:00Z' },
        ],
        markers: [
          { type: 'tool', at: '2025-01-01T00:00:10Z', session_id: 's1', label: '[Bash] ls' },
          { type: 'error', at: '2025-01-01T00:00:50Z', session_id: 's1', label: 'boom' },
        ],
      },
      {
        lane_id: 's2',
        phase: 'builder',
        layer: 1,
        agent_type: 'builder',
        status: 'running',
        segments: [{ session_id: 's2', status: 'running', started_at: '2025-01-01T00:01:00Z' }],
        markers: [],
      },
    ],
    children: [
      { instance_id: 'wfi-child', workflow: 'global-research', status: 'active', started_at: '2025-01-01T00:01:30Z', parent_session_id: 's2' },
    ],
    truncated: false,
    ...overrides,
  }
}

beforeEach(() => {
  mockUseTrace.mockReset()
})

describe('TraceView', () => {
  it('renders lanes grouped by layer with segments and child rows', () => {
    mockUseTrace.mockReturnValue({ data: makeTrace(), isLoading: false, error: null })
    renderWithQuery(<TraceView instanceId="wfi-1" />)

    expect(screen.getAllByTestId('trace-lane')).toHaveLength(2)
    expect(screen.getByText('Layer 0')).toBeInTheDocument()
    expect(screen.getByText('Layer 1')).toBeInTheDocument()
    expect(screen.getAllByTestId('trace-segment')).toHaveLength(2)
    expect(screen.getAllByTestId('trace-child')).toHaveLength(1)
    expect(screen.getByTestId('trace-root-span')).toBeInTheDocument()
  })

  it('shows loading / error / empty states', () => {
    mockUseTrace.mockReturnValue({ data: undefined, isLoading: true, error: null })
    const { unmount } = renderWithQuery(<TraceView instanceId="wfi-1" />)
    expect(screen.getByText('Loading trace…')).toBeInTheDocument()
    unmount()

    mockUseTrace.mockReturnValue({ data: undefined, isLoading: false, error: new Error('x') })
    const { unmount: u2 } = renderWithQuery(<TraceView instanceId="wfi-1" />)
    expect(screen.getByText('Failed to load trace')).toBeInTheDocument()
    u2()

    mockUseTrace.mockReturnValue({ data: makeTrace({ lanes: [], children: [] }), isLoading: false, error: null })
    renderWithQuery(<TraceView instanceId="wfi-1" />)
    expect(screen.getByText('No agent sessions yet')).toBeInTheDocument()
  })

  it('category chips filter markers', () => {
    mockUseTrace.mockReturnValue({ data: makeTrace(), isLoading: false, error: null })
    renderWithQuery(<TraceView instanceId="wfi-1" />)

    expect(screen.getAllByTestId('trace-marker').length).toBeGreaterThan(0)
    fireEvent.click(screen.getByTestId('trace-chip-tool'))
    fireEvent.click(screen.getByTestId('trace-chip-error'))
    expect(screen.queryAllByTestId('trace-marker')).toHaveLength(0)
  })

  it('clicking a child row pushes onto the breadcrumb and loads the child trace', () => {
    const childTrace = makeTrace({
      instance_id: 'wfi-child',
      workflow: 'global-research',
      lanes: [],
      children: [],
    })
    mockUseTrace.mockImplementation((iid: string | undefined) => ({
      data: iid === 'wfi-child' ? childTrace : makeTrace(),
      isLoading: false,
      error: null,
    }))
    renderWithQuery(<TraceView instanceId="wfi-1" />)

    expect(screen.queryByTestId('trace-breadcrumb')).not.toBeInTheDocument()
    fireEvent.click(screen.getByLabelText('open trace of global-research'))
    expect(screen.getByTestId('trace-breadcrumb')).toBeInTheDocument()
    expect(mockUseTrace).toHaveBeenLastCalledWith('wfi-child')
  })

  it('zoom buttons widen the plot and reset restores 100%', () => {
    mockUseTrace.mockReturnValue({ data: makeTrace(), isLoading: false, error: null })
    renderWithQuery(<TraceView instanceId="wfi-1" />)

    const plot = screen.getByTestId('trace-plot')
    expect(plot.style.width).toBe('100%')
    expect(screen.getByTestId('trace-zoom-out')).toBeDisabled()
    expect(screen.queryByTestId('trace-zoom-reset')).not.toBeInTheDocument()

    fireEvent.click(screen.getByTestId('trace-zoom-in'))
    expect(plot.style.width).toBe('150%')
    fireEvent.click(screen.getByTestId('trace-zoom-in'))
    expect(plot.style.width).toBe('225%')
    expect(screen.getByTestId('trace-zoom-reset')).toHaveTextContent('225%')

    fireEvent.click(screen.getByTestId('trace-zoom-reset'))
    expect(plot.style.width).toBe('100%')
  })

  it('shows truncation notice when marker cap was hit', () => {
    mockUseTrace.mockReturnValue({ data: makeTrace({ truncated: true }), isLoading: false, error: null })
    renderWithQuery(<TraceView instanceId="wfi-1" />)
    expect(screen.getByTestId('trace-truncated')).toBeInTheDocument()
  })

  it('renders a sub-lane group after its parent lane inside the same layer band, alongside the unaffected trace-child row', () => {
    const trace = makeTrace({
      sub_lanes: [
        {
          lane_id: 'w1',
          phase: 'delegate:w1',
          layer: -1,
          agent_type: 'extractor',
          status: 'completed',
          parent_lane_id: 's2',
          kind: 'delegate',
          delegation_id: 'd1',
          segments: [{ session_id: 'w1', status: 'completed', started_at: '2025-01-01T00:01:10Z', ended_at: '2025-01-01T00:01:20Z' }],
        },
      ],
    })
    mockUseTrace.mockReturnValue({ data: trace, isLoading: false, error: null })
    renderWithQuery(<TraceView instanceId="wfi-1" />)

    // Sub-lane group is collapsed by default: no nested trace-lane rendered yet.
    expect(screen.getAllByTestId('trace-lane')).toHaveLength(2)
    const group = screen.getByTestId('trace-sublane-group')
    expect(group).toBeInTheDocument()

    // It sits within the "Layer 1" band, immediately after the builder lane (parent s2).
    const layer1Label = screen.getByText('Layer 1')
    const band = layer1Label.parentElement!.parentElement!
    expect(band.contains(group)).toBe(true)

    // The pre-existing sub-workflow child row is unaffected by sub_lanes.
    expect(screen.getAllByTestId('trace-child')).toHaveLength(1)
    fireEvent.click(screen.getByLabelText('open trace of global-research'))
    expect(screen.getByTestId('trace-breadcrumb')).toBeInTheDocument()
  })

  it('renders exactly as today when the payload has no sub_lanes key (backward compatibility)', () => {
    const trace = makeTrace()
    delete (trace as { sub_lanes?: unknown }).sub_lanes
    mockUseTrace.mockReturnValue({ data: trace, isLoading: false, error: null })
    renderWithQuery(<TraceView instanceId="wfi-1" />)

    expect(screen.getAllByTestId('trace-lane')).toHaveLength(2)
    expect(screen.queryByTestId('trace-sublane-group')).not.toBeInTheDocument()
    expect(screen.getAllByTestId('trace-child')).toHaveLength(1)
  })

  it('lane click builds SelectedAgentData from sessions', () => {
    mockUseTrace.mockReturnValue({ data: makeTrace(), isLoading: false, error: null })
    const onAgentSelect = vi.fn()
    const sessions = [{ id: 's1', agent_type: 'analyzer' }] as never[]
    renderWithQuery(<TraceView instanceId="wfi-1" sessions={sessions} onAgentSelect={onAgentSelect} />)

    fireEvent.click(screen.getAllByTestId('trace-segment')[0].querySelector('button')!)
    expect(onAgentSelect).toHaveBeenCalledWith(
      expect.objectContaining({ phaseName: 'analyzer', session: expect.objectContaining({ id: 's1' }) })
    )
  })
})
