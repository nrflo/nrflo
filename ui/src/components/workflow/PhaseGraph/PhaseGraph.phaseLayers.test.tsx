import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act } from '@testing-library/react'
import { PhaseGraph } from './PhaseGraph'
import type { PhaseGraphProps } from './types'
import type { PhaseState, ActiveAgentV4 } from '@/types/workflow'
import type { Node } from '@xyflow/react'

// Capture nodes passed to layout so we can assert phaseIndex values
const capturedNodes: Node[][] = []
const mockGetLayoutedElements = vi.fn((nodes: Node[], edges: any[]) => {
  capturedNodes.push([...nodes])
  return Promise.resolve({ nodes, edges })
})

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

vi.mock('@/hooks/useElapsedTime', () => ({
  useTickingClock: vi.fn(),
}))

function makePhaseState(overrides: Partial<PhaseState> = {}): PhaseState {
  return { status: 'pending', ...overrides }
}

function makeAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_id: 'a1',
    agent_type: 'implementor',
    phase: 'test-be',
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
      'test-be': makePhaseState(),
      'test-fe': makePhaseState(),
    },
    phaseOrder: ['test-be', 'test-fe'],
    activeAgents: {},
    agentHistory: [],
    sessions: [],
    ...overrides,
  }
}

async function flushLayout() {
  await act(async () => {})
}

describe('PhaseGraph phaseLayers', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    capturedNodes.length = 0
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('assigns same phaseIndex to nodes in the same layer', async () => {
    // Both test-be and test-fe are layer 1
    render(<PhaseGraph {...makeProps({
      phaseLayers: { 'test-be': 1, 'test-fe': 1 },
    })} />)
    await flushLayout()

    expect(mockGetLayoutedElements).toHaveBeenCalled()
    const nodes = capturedNodes[0]
    expect(nodes).toHaveLength(2)

    const beNode = nodes.find(n => n.data.phaseName === 'test-be')
    const feNode = nodes.find(n => n.data.phaseName === 'test-fe')
    expect(beNode).toBeDefined()
    expect(feNode).toBeDefined()
    expect(beNode!.data.phaseIndex).toBe(1)
    expect(feNode!.data.phaseIndex).toBe(1)
  })

  it('assigns different phaseIndex to nodes in different layers', async () => {
    render(<PhaseGraph {...makeProps({
      phaseLayers: { 'test-be': 0, 'test-fe': 1 },
    })} />)
    await flushLayout()

    const nodes = capturedNodes[0]
    const beNode = nodes.find(n => n.data.phaseName === 'test-be')
    const feNode = nodes.find(n => n.data.phaseName === 'test-fe')
    expect(beNode!.data.phaseIndex).toBe(0)
    expect(feNode!.data.phaseIndex).toBe(1)
  })

  it('falls back to forEach index when phaseLayers is undefined', async () => {
    // No phaseLayers — sequential order: test-be=0, test-fe=1
    render(<PhaseGraph {...makeProps({ phaseLayers: undefined })} />)
    await flushLayout()

    const nodes = capturedNodes[0]
    const beNode = nodes.find(n => n.data.phaseName === 'test-be')
    const feNode = nodes.find(n => n.data.phaseName === 'test-fe')
    expect(beNode!.data.phaseIndex).toBe(0)
    expect(feNode!.data.phaseIndex).toBe(1)
  })

  it('falls back to forEach index for phases missing from phaseLayers', async () => {
    // test-fe is missing from phaseLayers — should fall back to idx=1
    render(<PhaseGraph {...makeProps({
      phaseLayers: { 'test-be': 0 },
    })} />)
    await flushLayout()

    const nodes = capturedNodes[0]
    const feNode = nodes.find(n => n.data.phaseName === 'test-fe')
    expect(feNode!.data.phaseIndex).toBe(1) // idx=1 (second in forEach)
  })

  it('edges connect nodes across consecutive phaseIndex layers, not within same layer', async () => {
    // Layer 0: setup, Layer 1: test-be + test-fe (parallel)
    render(<PhaseGraph {...makeProps({
      phases: {
        setup: makePhaseState({ status: 'completed' }),
        'test-be': makePhaseState({ status: 'pending' }),
        'test-fe': makePhaseState({ status: 'pending' }),
      },
      phaseOrder: ['setup', 'test-be', 'test-fe'],
      agentHistory: [],
      phaseLayers: { setup: 0, 'test-be': 1, 'test-fe': 1 },
    })} />)
    await flushLayout()

    const [passedNodes, passedEdges] = mockGetLayoutedElements.mock.calls[0] as [Node[], any[]]

    const setupNode = passedNodes.find(n => n.data.phaseName === 'setup')
    const beNode = passedNodes.find(n => n.data.phaseName === 'test-be')
    const feNode = passedNodes.find(n => n.data.phaseName === 'test-fe')

    expect(setupNode).toBeDefined()
    expect(beNode).toBeDefined()
    expect(feNode).toBeDefined()

    // setup → test-be and setup → test-fe edges should exist
    const setupToBeEdge = passedEdges.find((e: any) => e.source === setupNode!.id && e.target === beNode!.id)
    const setupToFeEdge = passedEdges.find((e: any) => e.source === setupNode!.id && e.target === feNode!.id)
    expect(setupToBeEdge).toBeDefined()
    expect(setupToFeEdge).toBeDefined()

    // No edge between test-be and test-fe (same phaseIndex = 1)
    const beToFeEdge = passedEdges.find((e: any) =>
      (e.source === beNode!.id && e.target === feNode!.id) ||
      (e.source === feNode!.id && e.target === beNode!.id)
    )
    expect(beToFeEdge).toBeUndefined()
  })

  it('uses correct phaseIndex for running parallel agents', async () => {
    // Two agents running in parallel at layer 2
    render(<PhaseGraph {...makeProps({
      phases: {
        'test-be': makePhaseState({ status: 'in_progress' }),
        'test-fe': makePhaseState({ status: 'in_progress' }),
      },
      phaseOrder: ['test-be', 'test-fe'],
      activeAgents: {
        'test-be-agent:claude:sonnet': makeAgent({ agent_type: 'test-be-agent', phase: 'test-be' }),
        'test-fe-agent:claude:sonnet': makeAgent({ agent_type: 'test-fe-agent', phase: 'test-fe' }),
      },
      phaseLayers: { 'test-be': 2, 'test-fe': 2 },
    })} />)
    await flushLayout()

    const nodes = capturedNodes[0]
    nodes.forEach(n => {
      expect(n.data.phaseIndex).toBe(2)
    })
  })
})
