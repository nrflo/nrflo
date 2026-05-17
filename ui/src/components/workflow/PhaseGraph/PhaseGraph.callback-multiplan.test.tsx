import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act, screen } from '@testing-library/react'
import { PhaseGraph } from './PhaseGraph'
import type { PhaseGraphProps } from './types'
import type { PhaseState, ActiveAgentV4, AgentHistoryEntry, CallbackInfo } from '@/types/workflow'
import type { Node } from '@xyflow/react'

const mockGetLayoutedElements = vi.fn((nodes: Node[], edges: any[]) =>
  Promise.resolve({ nodes, edges })
)

// Extended ReactFlow mock that renders mergedFromBadge nodes into the DOM so tests can query them
vi.mock('@xyflow/react', async () => {
  const actual = await vi.importActual('@xyflow/react')
  return {
    ...actual,
    ReactFlow: ({ children, nodes }: { children: React.ReactNode; nodes?: any[] }) => (
      <div data-testid="react-flow">
        {children}
        {nodes?.filter((n: any) => n.type === 'mergedFromBadge').map((n: any) => (
          <div key={n.id} data-testid="merged-from-badge">{(n.data?.agentIds ?? []).join(', ')}</div>
        ))}
      </div>
    ),
    Background: () => <div />,
    Controls: () => <div />,
    useReactFlow: () => ({ fitView: vi.fn() }),
    useStore: (selector: (s: Record<string, unknown>) => unknown) => selector({ width: 800, height: 600 }),
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

vi.mock('@/hooks/useIsMobile', () => ({ useIsMobile: () => false }))
vi.mock('@/hooks/useElapsedTime', () => ({ useTickingClock: vi.fn() }))

// --- Factories ---

function makePhaseState(overrides: Partial<PhaseState> = {}): PhaseState {
  return { status: 'pending', ...overrides }
}

function makeHistoryEntry(overrides: Partial<AgentHistoryEntry> = {}): AgentHistoryEntry {
  return { agent_id: 'h1', agent_type: 'implementor', phase: 'implement', result: 'pass', ...overrides }
}

function makeAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_id: 'a1', agent_type: 'qa-verifier', phase: 'qa',
    model_id: 'claude-sonnet-4-5', cli: 'claude', model: 'sonnet',
    pid: 12345, session_id: 's1', started_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

/** 4 phases at layers 1-4; reviewer running at layer 4, rest completed with history. */
function makeFourLayerProps(callbackInfo: CallbackInfo): PhaseGraphProps {
  return {
    phases: {
      setup: makePhaseState({ status: 'completed' }),
      implement: makePhaseState({ status: 'completed' }),
      qa: makePhaseState({ status: 'completed' }),
      review: makePhaseState({ status: 'in_progress' }),
    },
    phaseOrder: ['setup', 'implement', 'qa', 'review'],
    phaseLayers: { setup: 1, implement: 2, qa: 3, review: 4 },
    activeAgents: {
      'reviewer:claude:sonnet': makeAgent({ agent_type: 'reviewer', phase: 'review', agent_id: 'a2', session_id: 's2' }),
    },
    agentHistory: [
      makeHistoryEntry({ agent_type: 'setup-analyzer', phase: 'setup', agent_id: 'h1' }),
      makeHistoryEntry({ agent_type: 'implementor', phase: 'implement', agent_id: 'h2' }),
      makeHistoryEntry({ agent_type: 'qa-verifier', phase: 'qa', agent_id: 'h3' }),
    ],
    sessions: [],
    callbackInfo,
  }
}

/**
 * Two phases (unit, lint) share layer 1; qa at layer 2.
 * phaseOrder places lint last so in whole_layer mode the last node at layer 1 is the linter,
 * making it possible to verify agents-mode targets unit-tester specifically.
 */
function makeTwoAtSameLayerProps(callbackInfo: CallbackInfo): PhaseGraphProps {
  return {
    phases: {
      unit: makePhaseState({ status: 'completed' }),
      lint: makePhaseState({ status: 'completed' }),
      qa: makePhaseState({ status: 'in_progress' }),
    },
    phaseOrder: ['unit', 'lint', 'qa'],
    phaseLayers: { unit: 1, lint: 1, qa: 2 },
    activeAgents: { 'qa-verifier:claude:sonnet': makeAgent() },
    agentHistory: [
      makeHistoryEntry({ agent_type: 'unit-tester', phase: 'unit', agent_id: 'h1' }),
      makeHistoryEntry({ agent_type: 'linter', phase: 'lint', agent_id: 'h2' }),
    ],
    sessions: [],
    callbackInfo,
  }
}

async function flushLayout() {
  await act(async () => {})
}

// --- Tests ---

describe('PhaseGraph multi-step callback edges', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('3-step agents plan: 3 edges with ascending target layers and [agents] label prefix', async () => {
    const callbackInfo: CallbackInfo = {
      from_layer: 4,
      from_agent: 'reviewer',
      plan: [
        { layer: 1, mode: 'agents', agents: ['setup-analyzer'], instructions: 'Redo setup' },
        { layer: 2, mode: 'agents', agents: ['implementor'], instructions: 'Fix impl' },
        { layer: 3, mode: 'agents', agents: ['qa-verifier'], instructions: 'Rerun QA' },
      ],
    }
    render(<PhaseGraph {...makeFourLayerProps(callbackInfo)} />)
    await flushLayout()

    const edges = mockGetLayoutedElements.mock.calls[0][1] as any[]
    const cbEdges = edges.filter((e: any) => e.id.startsWith('callback-edge-'))
    expect(cbEdges).toHaveLength(3)
    expect(cbEdges.map((e: any) => e.id)).toEqual([
      'callback-edge-0-0', 'callback-edge-1-0', 'callback-edge-2-0',
    ])

    cbEdges.forEach((e: any) => expect(e.label).toMatch(/^\[agents\]/))

    const nodes = mockGetLayoutedElements.mock.calls[0][0] as Node[]
    const targetLayers = cbEdges.map((e: any) => {
      return nodes.find((n: Node) => n.id === e.target)!.data.phaseIndex
    })
    expect(targetLayers).toEqual([1, 2, 3])
  })

  it('1-step whole_layer plan: single edge with id=callback-edge and no prefix', async () => {
    const callbackInfo: CallbackInfo = {
      from_layer: 2,
      from_agent: 'qa-verifier',
      plan: [{ layer: 1, mode: 'whole_layer', instructions: 'Fix everything' }],
    }
    render(<PhaseGraph {...{
      phases: {
        implement: makePhaseState({ status: 'completed' }),
        qa: makePhaseState({ status: 'in_progress' }),
      },
      phaseOrder: ['implement', 'qa'],
      phaseLayers: { implement: 1, qa: 2 },
      activeAgents: { 'qa-verifier:claude:sonnet': makeAgent() },
      agentHistory: [makeHistoryEntry()],
      sessions: [],
      callbackInfo,
    }} />)
    await flushLayout()

    const edges = mockGetLayoutedElements.mock.calls[0][1] as any[]
    const legacyEdge = edges.find((e: any) => e.id === 'callback-edge')
    expect(legacyEdge).toBeDefined()
    expect(edges.filter((e: any) => e.id.startsWith('callback-edge-'))).toHaveLength(0)
    expect(legacyEdge.label).toBe('Fix everything')
  })

  it('agents-mode targets specific agent node, not the rightmost node at the layer', async () => {
    const callbackInfo: CallbackInfo = {
      from_layer: 2,
      from_agent: 'qa-verifier',
      plan: [{ layer: 1, mode: 'agents', agents: ['unit-tester'], instructions: 'Rerun units' }],
    }
    render(<PhaseGraph {...makeTwoAtSameLayerProps(callbackInfo)} />)
    await flushLayout()

    const nodes = mockGetLayoutedElements.mock.calls[0][0] as Node[]
    const edges = mockGetLayoutedElements.mock.calls[0][1] as any[]
    const cbEdge = edges.find((e: any) => e.id === 'callback-edge')
    expect(cbEdge).toBeDefined()

    const targetNode = nodes.find((n: Node) => n.id === cbEdge.target)
    // agents mode must hit unit-tester, not linter (which is last at layer 1 in iteration order)
    expect((targetNode!.data as any).historyEntry?.agent_type).toBe('unit-tester')
  })

  it('merged-from-badge node appears in DOM when requests.length >= 2', async () => {
    const callbackInfo: CallbackInfo = {
      from_layer: 2,
      from_agent: 'qa-verifier',
      plan: [{ layer: 1, mode: 'whole_layer', instructions: 'Fix' }],
      requests: [
        { from_agent: 'qa-verifier', mode: 'agent' },
        { from_agent: 'linter', mode: 'agent' },
      ],
    }
    render(<PhaseGraph {...{
      phases: {
        implement: makePhaseState({ status: 'completed' }),
        qa: makePhaseState({ status: 'in_progress' }),
      },
      phaseOrder: ['implement', 'qa'],
      phaseLayers: { implement: 1, qa: 2 },
      activeAgents: { 'qa-verifier:claude:sonnet': makeAgent() },
      agentHistory: [makeHistoryEntry()],
      sessions: [],
      callbackInfo,
    }} />)
    await flushLayout()

    const badge = screen.getByTestId('merged-from-badge')
    expect(badge).toBeInTheDocument()
    expect(badge).toHaveTextContent('qa-verifier')
    expect(badge).toHaveTextContent('linter')
  })
})
