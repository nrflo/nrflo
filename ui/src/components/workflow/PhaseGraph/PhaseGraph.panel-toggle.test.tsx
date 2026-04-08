import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act } from '@testing-library/react'
import { PhaseGraph } from './PhaseGraph'
import type { PhaseGraphProps } from './types'
import type { PhaseState, ActiveAgentV4 } from '@/types/workflow'

const mockFitView = vi.fn()
let mockStoreState = { width: 800, height: 600 }

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
    useStore: (selector: (s: Record<string, unknown>) => unknown) => selector(mockStoreState),
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

function propsWithAgent(): PhaseGraphProps {
  return makeProps({
    phases: { investigation: makePhaseState({ status: 'in_progress' }) },
    phaseOrder: ['investigation'],
    activeAgents: {
      'setup-analyzer:claude:sonnet': makeAgent({
        agent_type: 'setup-analyzer',
        phase: 'investigation',
      }),
    },
  })
}

/** Flush pending microtasks (async layout Promise) */
async function flushLayout() {
  await act(async () => {})
}

describe('PhaseGraph - Container Resize', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    mockStoreState = { width: 800, height: 600 }
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('calls fitView 150ms after container dimensions change', async () => {
    const { rerender } = render(<PhaseGraph {...propsWithAgent()} />)
    await flushLayout()

    // Flush mount timers (nodeKey@100ms, container@150ms)
    act(() => { vi.advanceTimersByTime(150) })
    vi.clearAllMocks()

    // Simulate container resize (e.g., panel collapse)
    mockStoreState = { width: 600, height: 600 }
    rerender(<PhaseGraph {...propsWithAgent()} />)
    await flushLayout()

    // Not called before 150ms
    act(() => { vi.advanceTimersByTime(149) })
    expect(mockFitView).not.toHaveBeenCalled()

    // Called at 150ms
    act(() => { vi.advanceTimersByTime(1) })
    expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3 })
    expect(mockFitView).toHaveBeenCalledTimes(1)
  })

  it('calls fitView after container width expands', async () => {
    const { rerender } = render(<PhaseGraph {...propsWithAgent()} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(150) })
    vi.clearAllMocks()

    // Simulate expand
    mockStoreState = { width: 1000, height: 600 }
    rerender(<PhaseGraph {...propsWithAgent()} />)
    await flushLayout()

    act(() => { vi.advanceTimersByTime(150) })
    expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3 })
  })

  it('debounces rapid dimension changes to single fitView call', async () => {
    const { rerender } = render(<PhaseGraph {...propsWithAgent()} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(150) })
    vi.clearAllMocks()

    // Rapid resizes (simulating CSS transition frames)
    mockStoreState = { width: 750, height: 600 }
    rerender(<PhaseGraph {...propsWithAgent()} />)
    await flushLayout()

    mockStoreState = { width: 700, height: 600 }
    rerender(<PhaseGraph {...propsWithAgent()} />)
    await flushLayout()

    mockStoreState = { width: 650, height: 600 }
    rerender(<PhaseGraph {...propsWithAgent()} />)
    await flushLayout()

    // Only one fitView call after 150ms from the last change
    act(() => { vi.advanceTimersByTime(150) })
    expect(mockFitView).toHaveBeenCalledTimes(1)
    expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3 })
  })

  it('does not fire fitView on container resize when no nodes exist', async () => {
    const props = makeProps({ phases: {}, phaseOrder: [] })
    const { rerender } = render(<PhaseGraph {...props} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(150) })
    vi.clearAllMocks()

    mockStoreState = { width: 600, height: 600 }
    rerender(<PhaseGraph {...props} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(200) })

    expect(mockFitView).not.toHaveBeenCalled()
  })

  it('fires fitView independently for node changes and container resize', async () => {
    const { rerender } = render(<PhaseGraph {...propsWithAgent()} />)
    await flushLayout()
    act(() => { vi.advanceTimersByTime(150) })
    vi.clearAllMocks()

    // Change nodes AND container dimensions simultaneously
    mockStoreState = { width: 600, height: 600 }
    rerender(<PhaseGraph {...makeProps({
      phases: { investigation: makePhaseState({ status: 'in_progress' }) },
      phaseOrder: ['investigation'],
      activeAgents: {
        'setup-analyzer:claude:sonnet': makeAgent({ agent_type: 'setup-analyzer', phase: 'investigation' }),
        'setup-analyzer:claude:haiku': makeAgent({ agent_type: 'setup-analyzer', phase: 'investigation', model_id: 'claude-haiku-4-5' }),
      },
    })} />)
    await flushLayout()

    // Node change fires at 100ms
    act(() => { vi.advanceTimersByTime(100) })
    expect(mockFitView).toHaveBeenCalledTimes(1)

    // Container resize fires at 150ms
    act(() => { vi.advanceTimersByTime(50) })
    expect(mockFitView).toHaveBeenCalledTimes(2)
  })
})
