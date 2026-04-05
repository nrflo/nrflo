import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act } from '@testing-library/react'
import { PhaseGraph } from './PhaseGraph'
import type { PhaseGraphProps } from './types'
import type { PhaseState, ActiveAgentV4, AgentHistoryEntry, CallbackInfo } from '@/types/workflow'
import type { Node } from '@xyflow/react'

const mockGetLayoutedElements = vi.fn((nodes: Node[], edges: any[]) =>
  Promise.resolve({ nodes, edges })
)

vi.mock('@xyflow/react', async () => {
  const actual = await vi.importActual('@xyflow/react')
  return {
    ...actual,
    ReactFlow: ({ children }: { children: React.ReactNode }) => (
      <div data-testid="react-flow">{children}</div>
    ),
    Background: () => <div />,
    Controls: () => <div />,
    useReactFlow: () => ({ fitView: vi.fn() }),
  }
})

vi.mock('./layout', () => ({
  getLayoutedElements: (nodes: Node[], edges: any[]) => mockGetLayoutedElements(nodes, edges),
  BASE_HEIGHT: 110,
}))

vi.mock('./AgentFlowNode', () => ({
  AgentFlowNode: ({ data }: { data: any }) => (
    <div data-testid={`agent-node-${data.agentKey}`}>{data.phaseName}</div>
  ),
}))

vi.mock('@/hooks/useIsMobile', () => ({
  useIsMobile: () => false,
}))

vi.mock('@/hooks/useElapsedTime', () => ({
  useTickingClock: vi.fn(),
}))

function makePhaseState(overrides: Partial<PhaseState> = {}): PhaseState {
  return { status: 'pending', ...overrides }
}

function makeHistoryEntry(overrides: Partial<AgentHistoryEntry> = {}): AgentHistoryEntry {
  return {
    agent_id: 'h1',
    agent_type: 'implementor',
    phase: 'implement',
    result: 'pass',
    ...overrides,
  }
}

function makeAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_id: 'a1',
    agent_type: 'qa-verifier',
    phase: 'qa',
    model_id: 'claude-sonnet-4-5',
    cli: 'claude',
    model: 'sonnet',
    pid: 12345,
    session_id: 's1',
    started_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

/** Base props: implement at layer 1 (completed), qa at layer 2 (running) */
function makeCallbackProps(overrides: Partial<PhaseGraphProps> = {}): PhaseGraphProps {
  return {
    phases: {
      implement: makePhaseState({ status: 'completed' }),
      qa: makePhaseState({ status: 'in_progress' }),
    },
    phaseOrder: ['implement', 'qa'],
    phaseLayers: { implement: 1, qa: 2 },
    activeAgents: {
      'qa-verifier:claude:sonnet': makeAgent(),
    },
    agentHistory: [makeHistoryEntry()],
    sessions: [],
    ...overrides,
  }
}

async function flushLayout() {
  await act(async () => {})
}

describe('PhaseGraph callback edge', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('adds a callback-edge when callbackInfo is provided', async () => {
    const callbackInfo: CallbackInfo = {
      level: 1,
      from_layer: 2,
      from_agent: 'qa-verifier',
      instructions: 'Fix the login bug',
    }
    render(<PhaseGraph {...makeCallbackProps({ callbackInfo })} />)
    await flushLayout()

    const edges = mockGetLayoutedElements.mock.calls[0][1] as any[]
    const callbackEdge = edges.find((e: any) => e.id === 'callback-edge')
    expect(callbackEdge).toBeDefined()
  })

  it('callback edge source is the from_agent node and target is at the target layer', async () => {
    const callbackInfo: CallbackInfo = {
      level: 1,
      from_layer: 2,
      from_agent: 'qa-verifier',
      instructions: 'Fix the login bug',
    }
    render(<PhaseGraph {...makeCallbackProps({ callbackInfo })} />)
    await flushLayout()

    const nodes = mockGetLayoutedElements.mock.calls[0][0] as Node[]
    const edges = mockGetLayoutedElements.mock.calls[0][1] as any[]

    const callbackEdge = edges.find((e: any) => e.id === 'callback-edge')
    expect(callbackEdge).toBeDefined()

    // Source node should be the qa-verifier running at layer 2
    const sourceNode = nodes.find((n: Node) => n.id === callbackEdge.source)
    expect(sourceNode).toBeDefined()
    expect(sourceNode!.data.phaseIndex).toBe(2)
    expect((sourceNode!.data as any).agent?.agent_type).toBe('qa-verifier')

    // Target node should be at layer 1 (implement phase)
    const targetNode = nodes.find((n: Node) => n.id === callbackEdge.target)
    expect(targetNode).toBeDefined()
    expect(targetNode!.data.phaseIndex).toBe(1)
  })

  it('callback edge has blue stroke, smoothstep type, and animated', async () => {
    const callbackInfo: CallbackInfo = {
      level: 1,
      from_layer: 2,
      from_agent: 'qa-verifier',
      instructions: 'Fix the login bug',
    }
    render(<PhaseGraph {...makeCallbackProps({ callbackInfo })} />)
    await flushLayout()

    const edges = mockGetLayoutedElements.mock.calls[0][1] as any[]
    const callbackEdge = edges.find((e: any) => e.id === 'callback-edge')

    expect(callbackEdge.type).toBe('smoothstep')
    expect(callbackEdge.animated).toBe(true)
    expect(callbackEdge.style.stroke).toBe('#3b82f6')
  })

  it('callback edge label shows instructions when short', async () => {
    const callbackInfo: CallbackInfo = {
      level: 1,
      from_layer: 2,
      from_agent: 'qa-verifier',
      instructions: 'Fix the login bug',
    }
    render(<PhaseGraph {...makeCallbackProps({ callbackInfo })} />)
    await flushLayout()

    const edges = mockGetLayoutedElements.mock.calls[0][1] as any[]
    const callbackEdge = edges.find((e: any) => e.id === 'callback-edge')
    expect(callbackEdge.label).toBe('Fix the login bug')
  })

  it('callback edge label is truncated to 60 chars with ellipsis', async () => {
    const long = 'A'.repeat(70)
    const callbackInfo: CallbackInfo = {
      level: 1,
      from_layer: 2,
      from_agent: 'qa-verifier',
      instructions: long,
    }
    render(<PhaseGraph {...makeCallbackProps({ callbackInfo })} />)
    await flushLayout()

    const edges = mockGetLayoutedElements.mock.calls[0][1] as any[]
    const callbackEdge = edges.find((e: any) => e.id === 'callback-edge')
    expect(callbackEdge.label).toBe('A'.repeat(57) + '...')
    expect(callbackEdge.label).toHaveLength(60)
  })

  it('no callback edge when callbackInfo is absent', async () => {
    render(<PhaseGraph {...makeCallbackProps()} />)
    await flushLayout()

    const edges = mockGetLayoutedElements.mock.calls[0][1] as any[]
    const callbackEdge = edges.find((e: any) => e.id === 'callback-edge')
    expect(callbackEdge).toBeUndefined()
  })

  it('falls back to any node at from_layer when from_agent has no exact match', async () => {
    // from_agent doesn't match any active/history agent type
    const callbackInfo: CallbackInfo = {
      level: 1,
      from_layer: 2,
      from_agent: 'unknown-agent',
      instructions: 'Retry',
    }
    render(<PhaseGraph {...makeCallbackProps({ callbackInfo })} />)
    await flushLayout()

    const edges = mockGetLayoutedElements.mock.calls[0][1] as any[]
    const callbackEdge = edges.find((e: any) => e.id === 'callback-edge')
    // Fallback: should still produce an edge with a node at from_layer=2
    expect(callbackEdge).toBeDefined()
    const nodes = mockGetLayoutedElements.mock.calls[0][0] as Node[]
    const sourceNode = nodes.find((n: Node) => n.id === callbackEdge.source)
    expect(sourceNode!.data.phaseIndex).toBe(2)
  })
})
