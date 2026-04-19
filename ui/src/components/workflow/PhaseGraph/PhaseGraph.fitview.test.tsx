/**
 * FitViewOnChange tests for nodeKey trigger (single 100ms timer + rAF flush).
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act } from '@testing-library/react'
import { PhaseGraph } from './PhaseGraph'
import type { PhaseGraphProps } from './types'
import type { PhaseState, ActiveAgentV4 } from '@/types/workflow'

const mockFitView = vi.fn()

vi.mock('@xyflow/react', async () => {
  const actual = await vi.importActual('@xyflow/react')
  return {
    ...actual,
    ReactFlow: ({ children }: { children: React.ReactNode }) => (
      <div data-testid="react-flow">{children}</div>
    ),
    Background: () => <div data-testid="background" />,
    Controls: () => <div data-testid="controls" />,
    useReactFlow: () => ({ fitView: mockFitView }),
    useStore: (selector: (s: Record<string, unknown>) => unknown) => selector({ width: 800, height: 600 }),
  }
})

vi.mock('./layout', () => ({
  getLayoutedElements: (nodes: any[], edges: any[]) => Promise.resolve({ nodes, edges }),
  BASE_HEIGHT: 110,
}))

vi.mock('./AgentFlowNode', () => ({
  AgentFlowNode: ({ data }: { data: any }) => (
    <div data-testid={`agent-node-${data.agentKey}`}>{data.phaseName}</div>
  ),
}))

vi.mock('@/hooks/useElapsedTime', () => ({ useTickingClock: vi.fn() }))
vi.mock('@/hooks/useIsMobile', () => ({ useIsMobile: () => false }))

function makePhaseState(overrides: Partial<PhaseState> = {}): PhaseState {
  return { status: 'pending', ...overrides }
}

function makeAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_id: 'a1',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude-sonnet-4-5',
    cli: 'claude',
    model: 'sonnet',
    pid: 12345,
    session_id: 's1',
    started_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function baseProps(): PhaseGraphProps {
  return {
    phases: { investigation: makePhaseState({ status: 'in_progress' }) },
    phaseOrder: ['investigation'],
    activeAgents: {
      'setup-analyzer:claude:sonnet': makeAgent({ agent_type: 'setup-analyzer', phase: 'investigation' }),
    },
    agentHistory: [],
    sessions: [],
  }
}

function propsWithTwoAgents(): PhaseGraphProps {
  return {
    ...baseProps(),
    activeAgents: {
      'setup-analyzer:claude:sonnet': makeAgent({ agent_type: 'setup-analyzer', phase: 'investigation' }),
      'setup-analyzer:claude:haiku': makeAgent({ agent_type: 'setup-analyzer', phase: 'investigation', model_id: 'claude-haiku-4-5' }),
    },
  }
}

async function flushLayout() {
  await act(async () => {})
}

describe('FitViewOnChange - nodeKey', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('fires fitView at 100ms on nodeKey change', async () => {
    const { rerender } = render(<PhaseGraph {...baseProps()} />)
    await flushLayout()
    // Flush all mount timers (nodeKey@100ms, container@150ms) + rAF (~16ms)
    act(() => { vi.advanceTimersByTime(200) })
    vi.clearAllMocks()

    // Add second agent — nodeKey changes
    rerender(<PhaseGraph {...propsWithTwoAgents()} />)
    await flushLayout()

    // 100ms timer fires + performFitView's rAF (~16ms) flushes the call
    act(() => { vi.advanceTimersByTime(150) })
    expect(mockFitView).toHaveBeenCalledTimes(1)
    expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3 })
  })

  it('nodeKey cleanup clears timer — fitView does not fire after unmount', async () => {
    const { rerender, unmount } = render(<PhaseGraph {...baseProps()} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(200) })
    vi.clearAllMocks()

    // Trigger nodeKey change
    rerender(<PhaseGraph {...propsWithTwoAgents()} />)
    await flushLayout()

    // Unmount before timer fires
    unmount()

    // Advance past timer + rAF deadline
    act(() => { vi.advanceTimersByTime(300) })

    // Timer must NOT fire — cleanup cleared it
    expect(mockFitView).not.toHaveBeenCalled()
  })
})
