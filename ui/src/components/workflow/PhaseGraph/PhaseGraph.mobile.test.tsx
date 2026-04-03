import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import { PhaseGraph } from './PhaseGraph'
import type { PhaseGraphProps } from './types'
import type { PhaseState } from '@/types/workflow'
import type { Node } from '@xyflow/react'

// Capture layout call arguments
const capturedLayoutCalls: Array<{ isMobile: boolean | undefined }> = []
const mockGetLayoutedElements = vi.fn((nodes: any[], edges: any[], _expanded: any, isMobile?: boolean) => {
  capturedLayoutCalls.push({ isMobile })
  return Promise.resolve({ nodes, edges })
})

// Capture ReactFlow props for asserting panOnDrag, zoomOnPinch, minZoom
const capturedReactFlowProps: { current: Record<string, unknown> } = { current: {} }
const mockFitView = vi.fn()

vi.mock('@xyflow/react', async () => {
  const actual = await vi.importActual('@xyflow/react')
  return {
    ...actual,
    ReactFlow: (props: Record<string, unknown>) => {
      capturedReactFlowProps.current = props
      return <div data-testid="react-flow">{props.children as React.ReactNode}</div>
    },
    Background: () => null,
    Controls: () => null,
    useReactFlow: () => ({ fitView: mockFitView }),
  }
})

vi.mock('./layout', () => ({
  getLayoutedElements: (nodes: Node[], edges: any[], expanded: any, isMobile?: boolean) =>
    mockGetLayoutedElements(nodes, edges, expanded, isMobile),
  BASE_HEIGHT: 110,
}))

vi.mock('./AgentFlowNode', () => ({
  AgentFlowNode: ({ data }: { data: any }) => (
    <div data-testid={`agent-node-${data.agentKey}`}>{data.phaseName}</div>
  ),
}))

vi.mock('@/hooks/useElapsedTime', () => ({
  useTickingClock: vi.fn(),
}))

// This file always returns isMobile=true to test mobile behavior
vi.mock('@/hooks/useIsMobile', () => ({
  useIsMobile: () => true,
}))

function makePhaseState(overrides: Partial<PhaseState> = {}): PhaseState {
  return { status: 'pending', ...overrides }
}

function makeProps(overrides: Partial<PhaseGraphProps> = {}): PhaseGraphProps {
  return {
    phases: { investigation: makePhaseState() },
    phaseOrder: ['investigation'],
    activeAgents: {},
    agentHistory: [],
    sessions: [],
    ...overrides,
  }
}

async function flushLayout() {
  await act(async () => {})
}

describe('PhaseGraph mobile behavior (isMobile=true)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    capturedLayoutCalls.length = 0
    capturedReactFlowProps.current = {}
    mockFitView.mockClear()
    mockGetLayoutedElements.mockClear()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('passes isMobile=true to getLayoutedElements', async () => {
    render(<PhaseGraph {...makeProps()} />)
    await flushLayout()

    expect(mockGetLayoutedElements).toHaveBeenCalled()
    expect(capturedLayoutCalls[0].isMobile).toBe(true)
  })

  it('enables panOnDrag on mobile', async () => {
    render(<PhaseGraph {...makeProps()} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(200) })

    expect(capturedReactFlowProps.current.panOnDrag).toBe(true)
  })

  it('enables zoomOnPinch on mobile', async () => {
    render(<PhaseGraph {...makeProps()} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(200) })

    expect(capturedReactFlowProps.current.zoomOnPinch).toBe(true)
  })

  it('sets minZoom=0.3 for more zoom-out on mobile', async () => {
    render(<PhaseGraph {...makeProps()} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(200) })

    expect(capturedReactFlowProps.current.minZoom).toBe(0.3)
  })

  it('still disables zoomOnScroll on mobile (would conflict with page scroll)', async () => {
    render(<PhaseGraph {...makeProps()} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(200) })

    expect(capturedReactFlowProps.current.zoomOnScroll).toBe(false)
  })

  it('renders ReactFlow with nodes on mobile', async () => {
    render(<PhaseGraph {...makeProps({
      phases: {
        investigation: makePhaseState({ status: 'pending' }),
        implementation: makePhaseState({ status: 'pending' }),
      },
      phaseOrder: ['investigation', 'implementation'],
    })} />)
    await flushLayout()

    expect(screen.getByTestId('react-flow')).toBeInTheDocument()
  })

  it('calls fitView with correct options on mobile', async () => {
    render(<PhaseGraph {...makeProps({
      phases: { investigation: makePhaseState({ status: 'pending' }) },
      phaseOrder: ['investigation'],
    })} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(200) })

    expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
  })
})
