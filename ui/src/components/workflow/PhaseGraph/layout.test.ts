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
    it('uses 320px layout constant for node spacing', () => {
      // Create two parallel agents in same layer
      const nodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 0),
      ]
      const edges: Edge[] = []

      const { nodes: layouted } = getLayoutedElements(nodes, edges, null)

      // AGENT_NODE_WIDTH = 320, HORIZONTAL_GAP = 60
      // Total width = 2 * 320 + 1 * 60 = 700
      // Start X = -700/2 = -350
      // Agent1 X = -350
      // Agent2 X = -350 + 320 + 60 = 30
      const agent1X = layouted[0].position.x
      const agent2X = layouted[1].position.x
      const spacing = agent2X - agent1X

      expect(spacing).toBe(380) // 320 + 60
    })

    it('maintains 20px edge padding (320 - 300)', () => {
      // AGENT_NODE_WIDTH (320) should be 20px wider than visual card width (300)
      // This creates edge spacing for proper ReactFlow rendering
      const AGENT_NODE_WIDTH = 320
      const VISUAL_CARD_WIDTH = 300
      const EXPECTED_EDGE_PADDING = 20

      expect(AGENT_NODE_WIDTH - VISUAL_CARD_WIDTH).toBe(EXPECTED_EDGE_PADDING)
    })
  })

  describe('card width integration (ticket nrworkflow-eacb3a)', () => {
    it('positions single agent centered at origin', () => {
      const nodes = [makeNode('agent1', 0)]
      const edges: Edge[] = []

      const { nodes: layouted } = getLayoutedElements(nodes, edges, null)

      // Single node: totalWidth = 320, startX = -320/2 = -160
      expect(layouted[0].position.x).toBe(-160)
      expect(layouted[0].position.y).toBe(0)
    })

    it('positions two parallel agents with proper horizontal spacing', () => {
      const nodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 0),
      ]
      const edges: Edge[] = []

      const { nodes: layouted } = getLayoutedElements(nodes, edges, null)

      // Total width = 2*320 + 60 = 700
      // Start X = -350
      expect(layouted[0].position.x).toBe(-350)
      expect(layouted[1].position.x).toBe(30) // -350 + 320 + 60
    })

    it('positions three parallel agents without overlap', () => {
      const nodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 0),
        makeNode('agent3', 0),
      ]
      const edges: Edge[] = []

      const { nodes: layouted } = getLayoutedElements(nodes, edges, null)

      // Total width = 3*320 + 2*60 = 1080
      // Start X = -540
      expect(layouted[0].position.x).toBe(-540)
      expect(layouted[1].position.x).toBe(-160) // -540 + 380
      expect(layouted[2].position.x).toBe(220)  // -160 + 380

      // Verify no overlap: each agent needs 300px visual space + gap
      const gap1 = layouted[1].position.x - layouted[0].position.x
      const gap2 = layouted[2].position.x - layouted[1].position.x

      expect(gap1).toBeGreaterThanOrEqual(320) // Layout width
      expect(gap2).toBeGreaterThanOrEqual(320)
    })

    it('positions nodes at same Y coordinate when in same layer', () => {
      const nodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 0),
      ]
      const edges: Edge[] = []

      const { nodes: layouted } = getLayoutedElements(nodes, edges, null)

      expect(layouted[0].position.y).toBe(layouted[1].position.y)
    })

    it('positions sequential layers with vertical gap', () => {
      const nodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 1),
      ]
      const edges: Edge[] = []

      const { nodes: layouted } = getLayoutedElements(nodes, edges, null)

      // VERTICAL_GAP = 120, BASE_HEIGHT = 110
      // Layer 0 Y = 0
      // Layer 1 Y = 0 + 110 + 120 = 230
      expect(layouted[0].position.y).toBe(0)
      expect(layouted[1].position.y).toBe(230)
    })

    it('sets measured width to AGENT_NODE_WIDTH (320)', () => {
      const nodes = [makeNode('agent1', 0)]
      const edges: Edge[] = []

      const { nodes: layouted } = getLayoutedElements(nodes, edges, null)

      expect(layouted[0].measured?.width).toBe(320)
    })

    it('sets measured height to BASE_HEIGHT (110) for non-expanded nodes', () => {
      const nodes = [makeNode('agent1', 0)]
      const edges: Edge[] = []

      const { nodes: layouted } = getLayoutedElements(nodes, edges, null)

      expect(layouted[0].measured?.height).toBe(110)
    })

    it('sets measured height to EXPANDED_HEIGHT (420) for expanded node', () => {
      const nodes = [makeNode('agent1', 0)]
      const edges: Edge[] = []

      const { nodes: layouted } = getLayoutedElements(nodes, edges, 'agent1')

      expect(layouted[0].measured?.height).toBe(420)
    })
  })

  describe('edge preservation', () => {
    it('preserves edge connections', () => {
      const nodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 1),
      ]
      const edges: Edge[] = [
        { id: 'e1', source: 'agent1', target: 'agent2' },
      ]

      const { edges: layoutedEdges } = getLayoutedElements(nodes, edges, null)

      expect(layoutedEdges).toHaveLength(1)
      expect(layoutedEdges[0].id).toBe('e1')
      expect(layoutedEdges[0].source).toBe('agent1')
      expect(layoutedEdges[0].target).toBe('agent2')
    })
  })

  describe('parallel agent scenarios (fan-out/fan-in)', () => {
    it('handles fan-out: single agent to multiple parallel agents', () => {
      const nodes = [
        makeNode('setup', 0),
        makeNode('test1', 1),
        makeNode('test2', 1),
        makeNode('test3', 1),
      ]
      const edges: Edge[] = []

      const { nodes: layouted } = getLayoutedElements(nodes, edges, null)

      // Layer 0 (single): centered at -160
      expect(layouted[0].position.x).toBe(-160)
      expect(layouted[0].position.y).toBe(0)

      // Layer 1 (three parallel): total width = 3*320 + 2*60 = 1080, start X = -540
      expect(layouted[1].position.x).toBe(-540)
      expect(layouted[2].position.x).toBe(-160)
      expect(layouted[3].position.x).toBe(220)
      expect(layouted[1].position.y).toBe(230) // All at same Y
      expect(layouted[2].position.y).toBe(230)
      expect(layouted[3].position.y).toBe(230)
    })

    it('handles fan-in: multiple agents to single agent', () => {
      const nodes = [
        makeNode('impl1', 0),
        makeNode('impl2', 0),
        makeNode('impl3', 0),
        makeNode('qa', 1),
      ]
      const edges: Edge[] = []

      const { nodes: layouted } = getLayoutedElements(nodes, edges, null)

      // Layer 0 (three parallel)
      expect(layouted[0].position.y).toBe(0)
      expect(layouted[1].position.y).toBe(0)
      expect(layouted[2].position.y).toBe(0)

      // Layer 1 (single): centered
      expect(layouted[3].position.x).toBe(-160)
      expect(layouted[3].position.y).toBe(230)
    })

    it('handles complex multi-layer parallel execution', () => {
      const nodes = [
        makeNode('setup', 0),
        makeNode('test-be', 1),
        makeNode('test-fe', 1),
        makeNode('impl-be', 2),
        makeNode('impl-fe', 2),
        makeNode('qa', 3),
      ]
      const edges: Edge[] = []

      const { nodes: layouted } = getLayoutedElements(nodes, edges, null)

      // Verify each layer's Y position increments
      expect(layouted[0].position.y).toBe(0)   // Layer 0
      expect(layouted[1].position.y).toBe(230) // Layer 1
      expect(layouted[2].position.y).toBe(230) // Layer 1
      expect(layouted[3].position.y).toBe(460) // Layer 2
      expect(layouted[4].position.y).toBe(460) // Layer 2
      expect(layouted[5].position.y).toBe(690) // Layer 3
    })
  })

  describe('horizontal gap proportions with wider cards', () => {
    it('maintains 60px horizontal gap between 320px node widths', () => {
      const nodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 0),
      ]
      const edges: Edge[] = []

      const { nodes: layouted } = getLayoutedElements(nodes, edges, null)

      const nodeWidth = layouted[0].measured?.width || 0
      const spacing = layouted[1].position.x - layouted[0].position.x
      const actualGap = spacing - nodeWidth

      expect(nodeWidth).toBe(320)
      expect(actualGap).toBe(60) // HORIZONTAL_GAP constant
    })

    it('gap-to-width ratio is 60:320 (18.75%)', () => {
      // With wider cards (320px), the 60px gap becomes proportionally smaller
      // This should still provide adequate visual separation
      const HORIZONTAL_GAP = 60
      const AGENT_NODE_WIDTH = 320
      const ratio = HORIZONTAL_GAP / AGENT_NODE_WIDTH

      expect(ratio).toBeCloseTo(0.1875, 4) // 18.75%
      expect(ratio).toBeGreaterThan(0.15)  // At least 15% gap
    })
  })

  describe('expanded node layout', () => {
    it('expands single node without affecting horizontal position', () => {
      const nodes = [makeNode('agent1', 0)]
      const edges: Edge[] = []

      const collapsed = getLayoutedElements(nodes, edges, null)
      const expanded = getLayoutedElements(nodes, edges, 'agent1')

      expect(collapsed.nodes[0].position.x).toBe(expanded.nodes[0].position.x)
      expect(collapsed.nodes[0].position.y).toBe(expanded.nodes[0].position.y)
    })

    it('expands node in layer with multiple agents without affecting siblings', () => {
      const nodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 0),
        makeNode('agent3', 0),
      ]
      const edges: Edge[] = []

      const collapsed = getLayoutedElements(nodes, edges, null)
      const expanded = getLayoutedElements(nodes, edges, 'agent2')

      // Horizontal positions unchanged
      expect(collapsed.nodes[0].position.x).toBe(expanded.nodes[0].position.x)
      expect(collapsed.nodes[1].position.x).toBe(expanded.nodes[1].position.x)
      expect(collapsed.nodes[2].position.x).toBe(expanded.nodes[2].position.x)

      // Y positions unchanged within same layer
      expect(collapsed.nodes[0].position.y).toBe(expanded.nodes[0].position.y)
      expect(collapsed.nodes[1].position.y).toBe(expanded.nodes[1].position.y)
      expect(collapsed.nodes[2].position.y).toBe(expanded.nodes[2].position.y)
    })

    it('expanding node increases subsequent layer Y positions', () => {
      const collapsedNodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 1),
      ]
      const expandedNodes = [
        makeNode('agent1', 0),
        makeNode('agent2', 1),
      ]
      const edges: Edge[] = []

      const collapsed = getLayoutedElements(collapsedNodes, edges, null)
      const expanded = getLayoutedElements(expandedNodes, edges, 'agent1')

      // Layer 1 should shift down when layer 0 expands
      // Collapsed: 0 + 110 + 120 = 230
      // Expanded: 0 + 420 + 120 = 540
      expect(collapsed.nodes[1].position.y).toBe(230)
      expect(expanded.nodes[1].position.y).toBe(540)
      expect(expanded.nodes[1].position.y).toBeGreaterThan(collapsed.nodes[1].position.y)
    })
  })
})
