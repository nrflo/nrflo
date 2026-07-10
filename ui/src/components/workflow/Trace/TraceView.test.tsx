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
      { instance_id: 'wfi-child', workflow: 'deep-research', status: 'active', started_at: '2025-01-01T00:01:30Z', parent_session_id: 's2' },
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
      workflow: 'deep-research',
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
    fireEvent.click(screen.getByLabelText('open trace of deep-research'))
    expect(screen.getByTestId('trace-breadcrumb')).toBeInTheDocument()
    expect(mockUseTrace).toHaveBeenLastCalledWith('wfi-child')
  })

  it('shows truncation notice when marker cap was hit', () => {
    mockUseTrace.mockReturnValue({ data: makeTrace({ truncated: true }), isLoading: false, error: null })
    renderWithQuery(<TraceView instanceId="wfi-1" />)
    expect(screen.getByTestId('trace-truncated')).toBeInTheDocument()
  })

  it('lane click builds SelectedAgentData from sessions', () => {
    mockUseTrace.mockReturnValue({ data: makeTrace(), isLoading: false, error: null })
    const onAgentSelect = vi.fn()
    const sessions = [{ id: 's1', agent_type: 'analyzer' }] as never[]
    renderWithQuery(<TraceView instanceId="wfi-1" sessions={sessions} onAgentSelect={onAgentSelect} />)

    fireEvent.click(screen.getAllByTestId('trace-segment')[0])
    expect(onAgentSelect).toHaveBeenCalledWith(
      expect.objectContaining({ phaseName: 'analyzer', session: expect.objectContaining({ id: 's1' }) })
    )
  })
})
