import { describe, it, expect } from 'vitest'
import { getLayoutedElements } from './layout'
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

/**
 * Y normalization tests — verifies the fix for nrworkflow-335e37.
 *
 * ELK assigns non-zero Y offsets to the topmost node in multi-agent graphs.
 * After layout, the topmost node should always start at Y=0 regardless of
 * how many agents are present.
 */
describe('Y normalization: topmost node starts at Y=0', () => {
  it('topmost node Y is 0 for single-agent graph', async () => {
    const nodes = [makeNode('agent1', 0)]
    const { nodes: layouted } = await getLayoutedElements(nodes, [], null)
    expect(layouted[0].position.y).toBe(0)
  })

  it('topmost node Y is 0 for multi-agent single-layer graph', async () => {
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
    const minY = Math.min(...layouted.map(n => n.position.y))
    expect(minY).toBe(0)
  })

  it('topmost node Y is 0 for full feature workflow (1→2→1 pattern)', async () => {
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
    const minY = Math.min(...layouted.map(n => n.position.y))
    expect(minY).toBe(0)
  })

  it('topmost node Y is 0 for mobile multi-agent layout', async () => {
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
    const { nodes: layouted } = await getLayoutedElements(nodes, edges, null, true)
    const minY = Math.min(...layouted.map(n => n.position.y))
    expect(minY).toBe(0)
  })

  it('relative Y spacing between layers is preserved after normalization', async () => {
    const nodes = [
      makeNode('setup', 0),
      makeNode('impl', 1),
      makeNode('qa', 2),
    ]
    const edges: Edge[] = [
      { id: 'e1', source: 'setup', target: 'impl' },
      { id: 'e2', source: 'impl', target: 'qa' },
    ]
    const { nodes: layouted } = await getLayoutedElements(nodes, edges, null)

    const byLayer = layouted.sort((a, b) => a.data.phaseIndex - b.data.phaseIndex)
    const gap01 = byLayer[1].position.y - byLayer[0].position.y
    const gap12 = byLayer[2].position.y - byLayer[1].position.y

    // Gaps between layers should be at least BASE_HEIGHT (110) + between-layers spacing (120)
    expect(gap01).toBeGreaterThanOrEqual(110 + 120)
    expect(gap12).toBeGreaterThanOrEqual(110 + 120)
  })
})
