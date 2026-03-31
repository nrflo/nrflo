import { describe, it, expect } from 'vitest'
import { getLayoutedElements, MOBILE_NODE_WIDTH, AGENT_NODE_WIDTH, BASE_HEIGHT } from './layout'
import type { Node, Edge } from '@xyflow/react'
import type { AgentFlowNodeData } from './types'
import type { ActiveAgentV4 } from '@/types/workflow'

function makeAgent(): ActiveAgentV4 {
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
  }
}

function makeNode(id: string, phaseIndex: number): Node<AgentFlowNodeData> {
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
    },
  }
}

describe('layout mobile (isMobile=true)', () => {
  it('MOBILE_NODE_WIDTH constant is 264', () => {
    expect(MOBILE_NODE_WIDTH).toBe(264)
  })

  it('AGENT_NODE_WIDTH constant is 352 (desktop)', () => {
    expect(AGENT_NODE_WIDTH).toBe(352)
  })

  it('uses MOBILE_NODE_WIDTH=264 for measured.width on mobile', async () => {
    const nodes = [makeNode('agent1', 0)]
    const { nodes: layouted } = await getLayoutedElements(nodes, [], null, true)
    expect(layouted[0].measured?.width).toBe(264)
  })

  it('uses desktop AGENT_NODE_WIDTH=352 for measured.width when isMobile=false', async () => {
    const nodes = [makeNode('agent1', 0)]
    const { nodes: layouted } = await getLayoutedElements(nodes, [], null, false)
    expect(layouted[0].measured?.width).toBe(352)
  })

  it('horizontal spacing between same-layer mobile nodes is 264+30=294', async () => {
    const nodes = [
      makeNode('agent1', 0),
      makeNode('agent2', 0),
      makeNode('next', 1),
    ]
    const edges: Edge[] = [
      { id: 'e1', source: 'agent1', target: 'next' },
      { id: 'e2', source: 'agent2', target: 'next' },
    ]

    const { nodes: layouted } = await getLayoutedElements(nodes, edges, null, true)

    const layer0 = [layouted[0], layouted[1]].sort((a, b) => a.position.x - b.position.x)
    const spacing = layer0[1].position.x - layer0[0].position.x

    expect(spacing).toBe(294) // MOBILE_NODE_WIDTH(264) + nodeNode(30)
  })

  it('horizontal spacing between same-layer desktop nodes is 320+60=380', async () => {
    const nodes = [
      makeNode('agent1', 0),
      makeNode('agent2', 0),
      makeNode('next', 1),
    ]
    const edges: Edge[] = [
      { id: 'e1', source: 'agent1', target: 'next' },
      { id: 'e2', source: 'agent2', target: 'next' },
    ]

    const { nodes: layouted } = await getLayoutedElements(nodes, edges, null, false)

    const layer0 = [layouted[0], layouted[1]].sort((a, b) => a.position.x - b.position.x)
    expect(layer0[1].position.x - layer0[0].position.x).toBe(412) // 352+60
  })

  it('between-layers vertical gap is smaller on mobile (60 vs 120)', async () => {
    const nodes = [makeNode('agent1', 0), makeNode('agent2', 1)]
    const edges: Edge[] = [{ id: 'e1', source: 'agent1', target: 'agent2' }]

    const [mobileResult, desktopResult] = await Promise.all([
      getLayoutedElements([makeNode('agent1', 0), makeNode('agent2', 1)], edges, null, true),
      getLayoutedElements([makeNode('agent1', 0), makeNode('agent2', 1)], edges, null, false),
    ])

    const mobileGap = mobileResult.nodes[1].position.y - mobileResult.nodes[0].position.y
    const desktopGap = desktopResult.nodes[1].position.y - desktopResult.nodes[0].position.y

    // Mobile gap should be at least BASE_HEIGHT + 60 (mobile spacing)
    expect(mobileGap).toBeGreaterThanOrEqual(BASE_HEIGHT + 60)
    // Mobile gap should be less than desktop gap (which uses 120 spacing)
    expect(mobileGap).toBeLessThan(desktopGap)
  })

  it('mobile same-layer nodes share Y coordinate (no overlap)', async () => {
    const nodes = [
      makeNode('agent1', 0),
      makeNode('agent2', 0),
      makeNode('next', 1),
    ]
    const edges: Edge[] = [
      { id: 'e1', source: 'agent1', target: 'next' },
      { id: 'e2', source: 'agent2', target: 'next' },
    ]

    const { nodes: layouted } = await getLayoutedElements(nodes, edges, null, true)

    expect(layouted[0].position.y).toBe(layouted[1].position.y)
  })

  it('mobile layers maintain correct vertical order', async () => {
    const nodes = [
      makeNode('agent1', 0),
      makeNode('agent2', 1),
      makeNode('agent3', 2),
    ]
    const edges: Edge[] = [
      { id: 'e1', source: 'agent1', target: 'agent2' },
      { id: 'e2', source: 'agent2', target: 'agent3' },
    ]

    const { nodes: layouted } = await getLayoutedElements(nodes, edges, null, true)

    expect(layouted[1].position.y).toBeGreaterThan(layouted[0].position.y)
    expect(layouted[2].position.y).toBeGreaterThan(layouted[1].position.y)
  })

  it('mobile 22px edge padding: MOBILE_NODE_WIDTH(264) - visual card(242) = 22', () => {
    const VISUAL_CARD_WIDTH = 242
    expect(MOBILE_NODE_WIDTH - VISUAL_CARD_WIDTH).toBe(22)
  })
})
