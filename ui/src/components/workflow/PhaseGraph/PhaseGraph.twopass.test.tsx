/**
 * Two-pass fitView strategy tests for FitViewOnChange (nodeKey trigger).
 *
 * Each trigger fires fitView twice:
 * - nodeKey: 500ms (first pass) + 1000ms (second pass)
 *
 * Panel/selectedAgent second-pass tests live in their respective test files.
 * Pattern: flush all mount timers to 1000ms, clearAllMocks, then trigger a
 * nodeKey-only change (same logPanelCollapsed/selectedAgent) to isolate the effect.
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

describe('FitViewOnChange - nodeKey two-pass', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('fires fitView twice on nodeKey change: at 500ms then again at 1000ms', async () => {
    const { rerender } = render(<PhaseGraph {...baseProps()} />)
    await flushLayout()
    // Flush all mount timers (logPanelCollapsed@350ms, selectedAgent@350ms, nodeKey@500ms,
    // logPanelCollapsed@850ms, selectedAgent@850ms, nodeKey@1000ms)
    act(() => { vi.advanceTimersByTime(1000) })
    vi.clearAllMocks()

    // Add second agent — nodeKey changes; logPanelCollapsed/selectedAgent unchanged so only nodeKey fires
    rerender(<PhaseGraph {...propsWithTwoAgents()} />)
    await flushLayout()

    // First pass fires at 500ms
    act(() => { vi.advanceTimersByTime(500) })
    expect(mockFitView).toHaveBeenCalledTimes(1)
    expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })

    // Second pass fires 500ms later (1000ms total since the change)
    act(() => { vi.advanceTimersByTime(500) })
    expect(mockFitView).toHaveBeenCalledTimes(2)
    expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
  })

  it('nodeKey cleanup clears both timers — second pass does not fire after unmount', async () => {
    const { rerender, unmount } = render(<PhaseGraph {...baseProps()} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(1000) })
    vi.clearAllMocks()

    // Trigger nodeKey change
    rerender(<PhaseGraph {...propsWithTwoAgents()} />)
    await flushLayout()

    // First pass fires at 500ms
    act(() => { vi.advanceTimersByTime(500) })
    expect(mockFitView).toHaveBeenCalledTimes(1)

    // Unmount before second timer (1000ms) fires
    unmount()

    // Advance past second timer deadline
    act(() => { vi.advanceTimersByTime(600) })

    // Second pass must NOT fire — cleanup cleared the timer
    expect(mockFitView).toHaveBeenCalledTimes(1)
  })
})
