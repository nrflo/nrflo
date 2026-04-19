/**
 * Controls-visibility regression test for PhaseGraph.
 *
 * Ticket: on the Project Workflows page, short layouts (e.g. a single-layer
 * `ticket-creator` workflow) produced a ReactFlow container shorter than the
 * vertical 4-button Controls panel (zoom-out, zoom-in, fit-view, auto-center
 * checkbox), clipping the bottom half. The fix clamps the wrapper's height to
 * CONTROLS_MIN_HEIGHT (140px) via `Math.max(containerHeight, CONTROLS_MIN_HEIGHT)`.
 *
 * This test forces a tiny computed containerHeight by mocking `./layout` with
 * a small BASE_HEIGHT, then asserts the rendered wrapper's inline height is
 * clamped to at least 140px.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act } from '@testing-library/react'
import { PhaseGraph } from './PhaseGraph'
import type { PhaseGraphProps } from './types'
import type { PhaseState } from '@/types/workflow'

const mockFitView = vi.fn()

vi.mock('@xyflow/react', async () => {
  const actual = await vi.importActual<typeof import('@xyflow/react')>('@xyflow/react')
  return {
    ...actual,
    ReactFlow: ({ children }: { children: React.ReactNode }) => (
      <div data-testid="react-flow">{children}</div>
    ),
    Background: () => <div data-testid="background" />,
    Controls: ({ children }: { children?: React.ReactNode }) => (
      <div data-testid="controls">{children}</div>
    ),
    ControlButton: ({
      children,
      onClick,
      ...rest
    }: React.ButtonHTMLAttributes<HTMLButtonElement> & { children?: React.ReactNode }) => (
      <button onClick={onClick} {...rest}>{children}</button>
    ),
    useReactFlow: () => ({ fitView: mockFitView, zoomIn: vi.fn(), zoomOut: vi.fn() }),
    useStore: (selector: (s: Record<string, unknown>) => unknown) =>
      selector({ width: 800, height: 600 }),
  }
})

// Force a tiny computed containerHeight: BASE_HEIGHT=10 means the wrapper's
// natural height is 0 (y) + 10 (BASE_HEIGHT) + 50 (safety pad) = 60px,
// which is below the CONTROLS_MIN_HEIGHT=140 clamp.
vi.mock('./layout', () => ({
  getLayoutedElements: (nodes: unknown[], edges: unknown[]) =>
    Promise.resolve({ nodes, edges }),
  BASE_HEIGHT: 10,
}))

vi.mock('./AgentFlowNode', () => ({
  AgentFlowNode: ({ data }: { data: { agentKey: string; phaseName: string } }) => (
    <div data-testid={`agent-node-${data.agentKey}`}>{data.phaseName}</div>
  ),
}))

vi.mock('@/hooks/useElapsedTime', () => ({ useTickingClock: vi.fn() }))
vi.mock('@/hooks/useIsMobile', () => ({ useIsMobile: () => false }))

function makePhaseState(overrides: Partial<PhaseState> = {}): PhaseState {
  return { status: 'pending', ...overrides }
}

function singleNodeProps(): PhaseGraphProps {
  return {
    phases: { investigation: makePhaseState({ status: 'in_progress' }) },
    phaseOrder: ['investigation'],
    activeAgents: {
      'setup-analyzer:claude:sonnet': {
        agent_id: 'a1',
        agent_type: 'setup-analyzer',
        phase: 'investigation',
        model_id: 'claude-sonnet-4-5',
        cli: 'claude',
        model: 'sonnet',
        pid: 1,
        session_id: 's1',
        started_at: '2026-01-01T00:00:00Z',
      },
    },
    agentHistory: [],
    sessions: [],
  }
}

describe('PhaseGraph - controls visibility clamp', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('clamps wrapper height to at least CONTROLS_MIN_HEIGHT (140px) when layout is small', async () => {
    const { container } = render(<PhaseGraph {...singleNodeProps()} />)
    // Flush the async layout promise so layoutedNodes is populated.
    await act(async () => {})

    // The outer wrapper is the sole element with class "w-full" and inline
    // height set by PhaseGraph.tsx. Query by className to avoid relying on a
    // test id, keeping the test implementation-agnostic.
    const wrapper = container.querySelector('div.w-full') as HTMLDivElement | null
    expect(wrapper).not.toBeNull()

    const heightPx = parseInt(wrapper!.style.height, 10)
    // Natural containerHeight from the small-BASE_HEIGHT mock is 60px. The
    // assertion proves the Math.max clamp raised it to >= 140px.
    expect(heightPx).toBeGreaterThanOrEqual(140)
  })
})
