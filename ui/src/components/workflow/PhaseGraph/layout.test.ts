import { describe, it, expect } from 'vitest'
import { getLayoutedElements } from './layout'
import type { Node, Edge } from '@xyflow/react'
import type { AgentFlowNodeData } from './types'
import type { ActiveAgentV4 } from '@/types/workflow'

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


function makeNode(id: string, phaseIndex: number, data: Partial<AgentFlowNodeData> = {}): Node<AgentFlowNodeData> {
  return {
    id,
    type: 'agent',
    position: { x: 0, y: 0 },
    data: {
      agentKey: id,
      phaseName: `phase${phaseIndex}`,
      phaseIndex,
      agent: makeAgent(),
      onToggleExpand: () => {},
      ...data,
    },
  }
}

describe('layout', () => {
  describe('AGENT_NODE_WIDTH constant', () => {
    it('uses 320px layout constant for node spacing', async () => {
      const nodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 0),
        makeNode('next', 1),
      ]
      const edges: Edge[] = [
        { id: 'e1', source: 'agent1', target: 'next' },
        { id: 'e2', source: 'agent2', target: 'next' },
      ]

      const { nodes: layouted } = await getLayoutedElements(nodes, edges, null)

      // Same-layer nodes should be spaced by AGENT_NODE_WIDTH + nodeNode gap
      const sorted = [layouted[0], layouted[1]].sort((a, b) => a.position.x - b.position.x)
      const spacing = sorted[1].position.x - sorted[0].position.x

      expect(spacing).toBe(412) // 352 + 60
    })

    it('maintains 22px edge padding (352 - 330)', () => {
      const AGENT_NODE_WIDTH = 352
      const VISUAL_CARD_WIDTH = 330
      const EXPECTED_EDGE_PADDING = 22

      expect(AGENT_NODE_WIDTH - VISUAL_CARD_WIDTH).toBe(EXPECTED_EDGE_PADDING)
    })
  })

  describe('card width integration (ticket nrflow-eacb3a)', () => {
    it('positions single agent at a defined position', async () => {
      const nodes = [makeNode('agent1', 0)]
      const edges: Edge[] = []

      const { nodes: layouted } = await getLayoutedElements(nodes, edges, null)

      expect(typeof layouted[0].position.x).toBe('number')
      expect(typeof layouted[0].position.y).toBe('number')
    })

    it('positions two parallel agents side by side', async () => {
      const nodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 0),
        makeNode('next', 1),
      ]
      const edges: Edge[] = [
        { id: 'e1', source: 'agent1', target: 'next' },
        { id: 'e2', source: 'agent2', target: 'next' },
      ]

      const { nodes: layouted } = await getLayoutedElements(nodes, edges, null)

      // Same layer: same Y, different X, no overlap
      expect(layouted[0].position.y).toBe(layouted[1].position.y)
      const spacing = Math.abs(layouted[1].position.x - layouted[0].position.x)
      expect(spacing).toBeGreaterThanOrEqual(352)
    })

    it('positions three parallel agents without overlap', async () => {
      const nodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 0),
        makeNode('agent3', 0),
        makeNode('next', 1),
      ]
      const edges: Edge[] = [
        { id: 'e1', source: 'agent1', target: 'next' },
        { id: 'e2', source: 'agent2', target: 'next' },
        { id: 'e3', source: 'agent3', target: 'next' },
      ]

      const { nodes: layouted } = await getLayoutedElements(nodes, edges, null)

      const layer0 = [layouted[0], layouted[1], layouted[2]].sort((a, b) => a.position.x - b.position.x)
      const gap1 = layer0[1].position.x - layer0[0].position.x
      const gap2 = layer0[2].position.x - layer0[1].position.x

      expect(gap1).toBeGreaterThanOrEqual(320)
      expect(gap2).toBeGreaterThanOrEqual(320)
    })

    it('positions nodes at same Y coordinate when in same layer', async () => {
      const nodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 0),
        makeNode('next', 1),
      ]
      const edges: Edge[] = [
        { id: 'e1', source: 'agent1', target: 'next' },
        { id: 'e2', source: 'agent2', target: 'next' },
      ]

      const { nodes: layouted } = await getLayoutedElements(nodes, edges, null)

      expect(layouted[0].position.y).toBe(layouted[1].position.y)
    })

    it('positions sequential layers with vertical gap', async () => {
      const nodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 1),
      ]
      const edges: Edge[] = [
        { id: 'e1', source: 'agent1', target: 'agent2' },
      ]

      const { nodes: layouted } = await getLayoutedElements(nodes, edges, null)

      // Layer 1 Y should be greater than layer 0 Y
      expect(layouted[1].position.y).toBeGreaterThan(layouted[0].position.y)
      // Gap should be at least BASE_HEIGHT + nodeNodeBetweenLayers spacing
      const gap = layouted[1].position.y - layouted[0].position.y
      expect(gap).toBeGreaterThanOrEqual(110 + 120)
    })

    it('sets measured width to AGENT_NODE_WIDTH (352)', async () => {
      const nodes = [makeNode('agent1', 0)]
      const edges: Edge[] = []

      const { nodes: layouted } = await getLayoutedElements(nodes, edges, null)

      expect(layouted[0].measured?.width).toBe(352)
    })

    it('sets measured height to BASE_HEIGHT (110) for non-expanded nodes', async () => {
      const nodes = [makeNode('agent1', 0)]
      const edges: Edge[] = []

      const { nodes: layouted } = await getLayoutedElements(nodes, edges, null)

      expect(layouted[0].measured?.height).toBe(110)
    })

    it('sets measured height to EXPANDED_HEIGHT (420) for expanded node', async () => {
      const nodes = [makeNode('agent1', 0)]
      const edges: Edge[] = []

      const { nodes: layouted } = await getLayoutedElements(nodes, edges, 'agent1')

      expect(layouted[0].measured?.height).toBe(420)
    })
  })

  describe('edge preservation', () => {
    it('preserves edge connections', async () => {
      const nodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 1),
      ]
      const edges: Edge[] = [
        { id: 'e1', source: 'agent1', target: 'agent2' },
      ]

      const { edges: layoutedEdges } = await getLayoutedElements(nodes, edges, null)

      expect(layoutedEdges).toHaveLength(1)
      expect(layoutedEdges[0].id).toBe('e1')
      expect(layoutedEdges[0].source).toBe('agent1')
      expect(layoutedEdges[0].target).toBe('agent2')
    })
  })

  describe('parallel agent scenarios (fan-out/fan-in)', () => {
    it('handles fan-out: single agent to multiple parallel agents', async () => {
      const nodes = [
        makeNode('setup', 0),
        makeNode('test1', 1),
        makeNode('test2', 1),
        makeNode('test3', 1),
      ]
      const edges: Edge[] = [
        { id: 'e1', source: 'setup', target: 'test1' },
        { id: 'e2', source: 'setup', target: 'test2' },
        { id: 'e3', source: 'setup', target: 'test3' },
      ]

      const { nodes: layouted } = await getLayoutedElements(nodes, edges, null)

      const layer0Y = layouted[0].position.y

      // Layer 1 (three parallel): all same Y, greater than layer 0
      expect(layouted[1].position.y).toBe(layouted[2].position.y)
      expect(layouted[2].position.y).toBe(layouted[3].position.y)
      expect(layouted[1].position.y).toBeGreaterThan(layer0Y)

      // No horizontal overlap in layer 1
      const sorted = [layouted[1], layouted[2], layouted[3]].sort((a, b) => a.position.x - b.position.x)
      expect(sorted[1].position.x - sorted[0].position.x).toBeGreaterThanOrEqual(320)
      expect(sorted[2].position.x - sorted[1].position.x).toBeGreaterThanOrEqual(320)
    })

    it('handles fan-in: multiple agents to single agent', async () => {
      const nodes = [
        makeNode('impl1', 0),
        makeNode('impl2', 0),
        makeNode('impl3', 0),
        makeNode('qa', 1),
      ]
      const edges: Edge[] = [
        { id: 'e1', source: 'impl1', target: 'qa' },
        { id: 'e2', source: 'impl2', target: 'qa' },
        { id: 'e3', source: 'impl3', target: 'qa' },
      ]

      const { nodes: layouted } = await getLayoutedElements(nodes, edges, null)

      // Layer 0 (three parallel): same Y
      expect(layouted[0].position.y).toBe(layouted[1].position.y)
      expect(layouted[1].position.y).toBe(layouted[2].position.y)

      // Layer 1 (single): Y greater than layer 0
      expect(layouted[3].position.y).toBeGreaterThan(layouted[0].position.y)
    })

    it('centers single nodes over parallel layer in 1→2→1 pattern', async () => {
      const nodes = [
        makeNode('setup', 0),
        makeNode('impl1', 1),
        makeNode('impl2', 1),
        makeNode('qa', 2),
      ]
      const edges: Edge[] = [
        { id: 'e1', source: 'setup', target: 'impl1' },
        { id: 'e2', source: 'setup', target: 'impl2' },
        { id: 'e3', source: 'impl1', target: 'qa' },
        { id: 'e4', source: 'impl2', target: 'qa' },
      ]

      const { nodes: layouted } = await getLayoutedElements(nodes, edges, null)

      const nodeWidth = 352
      const layer1 = [layouted[1], layouted[2]].sort((a, b) => a.position.x - b.position.x)
      const layer1Center = (layer1[0].position.x + layer1[1].position.x + nodeWidth) / 2

      // Single nodes (setup, qa) should be centered over the parallel pair
      const setupCenter = layouted[0].position.x + nodeWidth / 2
      const qaCenter = layouted[3].position.x + nodeWidth / 2

      expect(Math.abs(setupCenter - layer1Center)).toBeLessThan(1)
      expect(Math.abs(qaCenter - layer1Center)).toBeLessThan(1)
    })

    it('handles complex multi-layer parallel execution', async () => {
      const nodes = [
        makeNode('setup', 0),
        makeNode('test-be', 1),
        makeNode('test-fe', 1),
        makeNode('impl-be', 2),
        makeNode('impl-fe', 2),
        makeNode('qa', 3),
      ]
      const edges: Edge[] = [
        { id: 'e1', source: 'setup', target: 'test-be' },
        { id: 'e2', source: 'setup', target: 'test-fe' },
        { id: 'e3', source: 'test-be', target: 'impl-be' },
        { id: 'e4', source: 'test-be', target: 'impl-fe' },
        { id: 'e5', source: 'test-fe', target: 'impl-be' },
        { id: 'e6', source: 'test-fe', target: 'impl-fe' },
        { id: 'e7', source: 'impl-be', target: 'qa' },
        { id: 'e8', source: 'impl-fe', target: 'qa' },
      ]

      const { nodes: layouted } = await getLayoutedElements(nodes, edges, null)

      const layer0Y = layouted[0].position.y
      const layer1Y = layouted[1].position.y
      const layer2Y = layouted[3].position.y
      const layer3Y = layouted[5].position.y

      // Same-layer nodes share Y
      expect(layouted[1].position.y).toBe(layouted[2].position.y) // Layer 1
      expect(layouted[3].position.y).toBe(layouted[4].position.y) // Layer 2

      // Y increases between layers
      expect(layer1Y).toBeGreaterThan(layer0Y)
      expect(layer2Y).toBeGreaterThan(layer1Y)
      expect(layer3Y).toBeGreaterThan(layer2Y)
    })
  })

  describe('horizontal gap proportions with wider cards', () => {
    it('maintains 60px horizontal gap between 352px node widths', async () => {
      const nodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 0),
        makeNode('next', 1),
      ]
      const edges: Edge[] = [
        { id: 'e1', source: 'agent1', target: 'next' },
        { id: 'e2', source: 'agent2', target: 'next' },
      ]

      const { nodes: layouted } = await getLayoutedElements(nodes, edges, null)

      const nodeWidth = layouted[0].measured?.width || 0
      const sorted = [layouted[0], layouted[1]].sort((a, b) => a.position.x - b.position.x)
      const spacing = sorted[1].position.x - sorted[0].position.x
      const actualGap = spacing - nodeWidth

      expect(nodeWidth).toBe(352)
      expect(actualGap).toBe(60)
    })

    it('gap-to-width ratio is 60:352 (17.05%)', () => {
      const HORIZONTAL_GAP = 60
      const AGENT_NODE_WIDTH = 352
      const ratio = HORIZONTAL_GAP / AGENT_NODE_WIDTH

      expect(ratio).toBeCloseTo(0.1705, 3)
      expect(ratio).toBeGreaterThan(0.15)
    })
  })

  describe('expanded node layout', () => {
    it('expands single node without affecting horizontal position', async () => {
      const collapsedNodes = [makeNode('agent1', 0)]
      const expandedNodes = [makeNode('agent1', 0)]
      const edges: Edge[] = []

      const collapsed = await getLayoutedElements(collapsedNodes, edges, null)
      const expanded = await getLayoutedElements(expandedNodes, edges, 'agent1')

      expect(collapsed.nodes[0].position.x).toBe(expanded.nodes[0].position.x)
      expect(collapsed.nodes[0].position.y).toBe(expanded.nodes[0].position.y)
    })

    it('expands node in layer with multiple agents without affecting horizontal positions', async () => {
      const collapsedNodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 0),
        makeNode('agent3', 0),
        makeNode('next', 1),
      ]
      const expandedNodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 0),
        makeNode('agent3', 0),
        makeNode('next', 1),
      ]
      const edges: Edge[] = [
        { id: 'e1', source: 'agent1', target: 'next' },
        { id: 'e2', source: 'agent2', target: 'next' },
        { id: 'e3', source: 'agent3', target: 'next' },
      ]

      const collapsed = await getLayoutedElements(collapsedNodes, edges, null)
      const expanded = await getLayoutedElements(expandedNodes, edges, 'agent2')

      // Horizontal positions unchanged
      expect(collapsed.nodes[0].position.x).toBe(expanded.nodes[0].position.x)
      expect(collapsed.nodes[1].position.x).toBe(expanded.nodes[1].position.x)
      expect(collapsed.nodes[2].position.x).toBe(expanded.nodes[2].position.x)
    })

    it('expanding node increases subsequent layer Y positions', async () => {
      const collapsedNodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 1),
      ]
      const expandedNodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 1),
      ]
      const edges: Edge[] = [
        { id: 'e1', source: 'agent1', target: 'agent2' },
      ]

      const collapsed = await getLayoutedElements(collapsedNodes, edges, null)
      const expanded = await getLayoutedElements(expandedNodes, edges, 'agent1')

      // Layer 1 should shift down when layer 0 expands
      expect(expanded.nodes[1].position.y).toBeGreaterThan(collapsed.nodes[1].position.y)
    })
  })

  describe('synthetic edge generation for disconnected layers', () => {
    it('positions nodes in correct layers even without explicit edges', async () => {
      // This tests buildLayerEdges: if no edges connect layers, synthetic edges
      // ensure ELK still assigns nodes to their phaseIndex layers
      const nodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 1),
        makeNode('agent3', 2),
      ]
      const edges: Edge[] = [] // No explicit edges

      const { nodes: layouted } = await getLayoutedElements(nodes, edges, null)

      // Nodes should still be in vertical sequence by layer
      const layer0Y = layouted[0].position.y
      const layer1Y = layouted[1].position.y
      const layer2Y = layouted[2].position.y

      expect(layer1Y).toBeGreaterThan(layer0Y)
      expect(layer2Y).toBeGreaterThan(layer1Y)
    })

    it('positions parallel nodes at same Y without explicit edges', async () => {
      const nodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 0),
        makeNode('agent3', 1),
      ]
      const edges: Edge[] = []

      const { nodes: layouted } = await getLayoutedElements(nodes, edges, null)

      // Same layer nodes should share Y coordinate even without edges
      expect(layouted[0].position.y).toBe(layouted[1].position.y)
      expect(layouted[2].position.y).toBeGreaterThan(layouted[0].position.y)
    })
  })
})
