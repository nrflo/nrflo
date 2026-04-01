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

vi.mock('@/hooks/useElapsedTime', () => ({
  useTickingClock: vi.fn(),
}))

vi.mock('@/hooks/useIsMobile', () => ({
  useIsMobile: () => false,
}))

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

function makeProps(overrides: Partial<PhaseGraphProps> = {}): PhaseGraphProps {
  return {
    phases: {
      investigation: makePhaseState({ status: 'completed' }),
      implementation: makePhaseState({ status: 'completed' }),
    },
    phaseOrder: ['investigation', 'implementation'],
    activeAgents: {},
    agentHistory: [
      makeAgent({ agent_type: 'setup-analyzer', phase: 'investigation', session_id: 'sess-1' }) as any,
      makeAgent({ agent_type: 'implementor', phase: 'implementation', session_id: 'sess-2' }) as any,
    ],
    sessions: [],
    ...overrides,
  }
}

/** Flush pending microtasks (async layout Promise) */
async function flushLayout() {
  await act(async () => {})
}

describe('PhaseGraph - selectedAgent fitView', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('fires fitView at 350ms when selectedAgent changes from null to a value', async () => {
    const { rerender } = render(<PhaseGraph {...makeProps({ selectedAgent: null })} />)
    await flushLayout()

    // Flush all mount timers (nodeKey 50ms + logPanelCollapsed 350ms + selectedAgent 350ms)
    act(() => { vi.advanceTimersByTime(350) })
    vi.clearAllMocks()

    // Agent selected: null → 'sess-1'
    rerender(<PhaseGraph {...makeProps({ selectedAgent: 'sess-1' })} />)
    await flushLayout()

    // Not yet at 350ms
    act(() => { vi.advanceTimersByTime(349) })
    expect(mockFitView).not.toHaveBeenCalled()

    // Fires exactly at 350ms with correct options
    act(() => { vi.advanceTimersByTime(1) })
    expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
    expect(mockFitView).toHaveBeenCalledTimes(1)
  })

  it('fires fitView at 350ms when selectedAgent changes back to null (panel unmounts)', async () => {
    const { rerender } = render(<PhaseGraph {...makeProps({ selectedAgent: 'sess-1' })} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(350) })
    vi.clearAllMocks()

    // Deselect: 'sess-1' → null
    rerender(<PhaseGraph {...makeProps({ selectedAgent: null })} />)
    await flushLayout()

    act(() => { vi.advanceTimersByTime(350) })
    expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
    expect(mockFitView).toHaveBeenCalledTimes(1)
  })

  it('fires fitView when switching between two selected agents', async () => {
    const { rerender } = render(<PhaseGraph {...makeProps({ selectedAgent: 'sess-1' })} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(350) })
    vi.clearAllMocks()

    rerender(<PhaseGraph {...makeProps({ selectedAgent: 'sess-2' })} />)
    await flushLayout()

    act(() => { vi.advanceTimersByTime(350) })
    expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
    expect(mockFitView).toHaveBeenCalledTimes(1)
  })

  it('does not fire extra fitView when selectedAgent is unchanged across rerenders', async () => {
    const props = makeProps({ selectedAgent: 'sess-1' })
    const { rerender } = render(<PhaseGraph {...props} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(350) })
    vi.clearAllMocks()

    // Rerender with same selectedAgent value (e.g., parent re-renders with identical derived key)
    rerender(<PhaseGraph {...props} />)
    await flushLayout()

    act(() => { vi.advanceTimersByTime(400) })
    expect(mockFitView).not.toHaveBeenCalled()
  })

  it('selectedAgent and logPanelCollapsed effects fire independently', async () => {
    const { rerender } = render(
      <PhaseGraph {...makeProps({ selectedAgent: null, logPanelCollapsed: false })} />
    )
    await flushLayout()
    act(() => { vi.advanceTimersByTime(350) })
    vi.clearAllMocks()

    // Change both simultaneously
    rerender(<PhaseGraph {...makeProps({ selectedAgent: 'sess-1', logPanelCollapsed: true })} />)
    await flushLayout()

    act(() => { vi.advanceTimersByTime(350) })
    // Both effects fire — expect at least 2 calls (logPanelCollapsed + selectedAgent)
    expect(mockFitView.mock.calls.length).toBeGreaterThanOrEqual(2)
    expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
  })
})
