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
      investigation: makePhaseState(),
      implementation: makePhaseState(),
    },
    phaseOrder: ['investigation', 'implementation'],
    activeAgents: {},
    agentHistory: [],
    sessions: [],
    ...overrides,
  }
}

function propsWithAgent(logPanelCollapsed?: boolean): PhaseGraphProps {
  return makeProps({
    phases: { investigation: makePhaseState({ status: 'in_progress' }) },
    phaseOrder: ['investigation'],
    activeAgents: {
      'setup-analyzer:claude:sonnet': makeAgent({
        agent_type: 'setup-analyzer',
        phase: 'investigation',
      }),
    },
    logPanelCollapsed,
  })
}

/** Flush pending microtasks (async layout Promise) */
async function flushLayout() {
  await act(async () => {})
}

describe('PhaseGraph - Panel Toggle', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('calls fitView with correct options after 350ms delay on collapse', async () => {
    const { rerender } = render(<PhaseGraph {...propsWithAgent(false)} />)
    await flushLayout()

    // Initial mount fitView (50ms nodeKey timer)
    act(() => { vi.advanceTimersByTime(50) })
    vi.clearAllMocks()

    // Collapse panel
    rerender(<PhaseGraph {...propsWithAgent(true)} />)
    await flushLayout()

    // Not called before 350ms
    act(() => { vi.advanceTimersByTime(349) })
    expect(mockFitView).not.toHaveBeenCalled()

    // Called at 350ms with correct options
    act(() => { vi.advanceTimersByTime(1) })
    expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
    expect(mockFitView).toHaveBeenCalledTimes(1)
  })

  it('calls fitView after 350ms delay on expand', async () => {
    const { rerender } = render(<PhaseGraph {...propsWithAgent(true)} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(50) })
    vi.clearAllMocks()

    rerender(<PhaseGraph {...propsWithAgent(false)} />)
    await flushLayout()

    act(() => { vi.advanceTimersByTime(350) })
    expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
  })

  it('handles multiple rapid toggles (each creates a timer)', async () => {
    const { rerender } = render(<PhaseGraph {...propsWithAgent(false)} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(50) })
    vi.clearAllMocks()

    rerender(<PhaseGraph {...propsWithAgent(true)} />)
    await flushLayout()
    rerender(<PhaseGraph {...propsWithAgent(false)} />)
    await flushLayout()
    rerender(<PhaseGraph {...propsWithAgent(true)} />)
    await flushLayout()

    act(() => { vi.advanceTimersByTime(350) })
    expect(mockFitView.mock.calls.length).toBeGreaterThanOrEqual(1)
  })

  it('fires fitView independently for node changes and panel toggle', async () => {
    const { rerender } = render(<PhaseGraph {...propsWithAgent(false)} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(50) })
    vi.clearAllMocks()

    // Change nodes AND toggle panel simultaneously
    rerender(<PhaseGraph {...makeProps({
      phases: { investigation: makePhaseState({ status: 'in_progress' }) },
      phaseOrder: ['investigation'],
      activeAgents: {
        'setup-analyzer:claude:sonnet': makeAgent({ agent_type: 'setup-analyzer', phase: 'investigation' }),
        'setup-analyzer:claude:haiku': makeAgent({ agent_type: 'setup-analyzer', phase: 'investigation', model_id: 'claude-haiku-4-5' }),
      },
      logPanelCollapsed: true,
    })} />)
    await flushLayout()

    // Node change fires at 50ms
    act(() => { vi.advanceTimersByTime(50) })
    expect(mockFitView).toHaveBeenCalledTimes(1)

    // Panel toggle fires at 350ms
    act(() => { vi.advanceTimersByTime(300) })
    expect(mockFitView).toHaveBeenCalledTimes(2)
  })

  it('does not fire fitView on panel toggle when no nodes exist', async () => {
    const props = makeProps({ phases: {}, phaseOrder: [], logPanelCollapsed: false })
    const { rerender } = render(<PhaseGraph {...props} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(50) })
    vi.clearAllMocks()

    rerender(<PhaseGraph {...props} logPanelCollapsed={true} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(400) })

    expect(mockFitView).not.toHaveBeenCalled()
  })
})
