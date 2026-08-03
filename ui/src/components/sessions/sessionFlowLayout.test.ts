import { describe, it, expect } from 'vitest'
import { getSessionFlowLayout, FLOW_NODE_WIDTH, FLOW_NODE_HEIGHT } from './sessionFlowLayout'
import type { SessionFlowResponse, SessionFlowNode, SessionFlowEdge } from '@/types/session'

function makeNode(overrides: Partial<SessionFlowNode> = {}): SessionFlowNode {
  return {
    session_id: 's1',
    kind: 'delegate',
    depth: 1,
    status: 'completed',
    ...overrides,
  }
}

function makeEdge(overrides: Partial<SessionFlowEdge> = {}): SessionFlowEdge {
  return {
    from_session_id: 'root',
    to_session_id: 'child',
    kind: 'delegate',
    ...overrides,
  }
}

describe('getSessionFlowLayout', () => {
  it('returns empty nodes/edges for an undefined flow', async () => {
    const result = await getSessionFlowLayout(undefined)
    expect(result).toEqual({ nodes: [], edges: [] })
  })

  it('returns empty nodes/edges for a flow with no nodes', async () => {
    const flow: SessionFlowResponse = { root_session_id: 'root', nodes: [], edges: [], truncated: false }
    const result = await getSessionFlowLayout(flow)
    expect(result).toEqual({ nodes: [], edges: [] })
  })

  it('maps each flow node to a ReactFlow node, marking the depth-0 root session', async () => {
    const flow: SessionFlowResponse = {
      root_session_id: 'root',
      nodes: [
        makeNode({ session_id: 'root', depth: 0, kind: 'delegate' }),
        makeNode({ session_id: 'child', depth: 1 }),
      ],
      edges: [makeEdge({ from_session_id: 'root', to_session_id: 'child' })],
      truncated: false,
    }

    const { nodes, edges } = await getSessionFlowLayout(flow)

    expect(nodes).toHaveLength(2)
    expect(edges).toHaveLength(1)

    const rootNode = nodes.find((n) => n.id === 'root')!
    const childNode = nodes.find((n) => n.id === 'child')!
    expect(rootNode.data.isRoot).toBe(true)
    expect(childNode.data.isRoot).toBe(false)
    expect(rootNode.type).toBe('sessionFlow')
    expect(rootNode.measured).toEqual({ width: FLOW_NODE_WIDTH, height: FLOW_NODE_HEIGHT })
  })

  it('does not mark a depth>0 node as root even if its session_id matches root_session_id', async () => {
    const flow: SessionFlowResponse = {
      root_session_id: 'root',
      nodes: [makeNode({ session_id: 'root', depth: 2 })],
      edges: [],
      truncated: false,
    }
    const { nodes } = await getSessionFlowLayout(flow)
    expect(nodes[0].data.isRoot).toBe(false)
  })

  it('dedupes a session_id shared by two nodes, keeping a single node', async () => {
    const flow: SessionFlowResponse = {
      root_session_id: 'root',
      nodes: [
        makeNode({ session_id: 'root', depth: 0 }),
        makeNode({ session_id: 'p2', depth: 1 }),
        makeNode({ session_id: 'shared', depth: 2 }),
        makeNode({ session_id: 'shared', depth: 2 }),
      ],
      edges: [
        makeEdge({ from_session_id: 'root', to_session_id: 'shared' }),
        makeEdge({ from_session_id: 'p2', to_session_id: 'shared' }),
      ],
      truncated: false,
    }

    const { nodes, edges } = await getSessionFlowLayout(flow)

    expect(nodes.filter((n) => n.id === 'shared')).toHaveLength(1)
    expect(nodes).toHaveLength(3)
    expect(edges).toHaveLength(2)
  })

  it('drops edges referencing a session id absent from the node list', async () => {
    const flow: SessionFlowResponse = {
      root_session_id: 'root',
      nodes: [makeNode({ session_id: 'root', depth: 0 })],
      edges: [makeEdge({ from_session_id: 'root', to_session_id: 'missing' })],
      truncated: false,
    }
    const { nodes, edges } = await getSessionFlowLayout(flow)
    expect(nodes).toHaveLength(1)
    expect(edges).toHaveLength(0)
  })

  it('dedupes a repeated edge', async () => {
    const flow: SessionFlowResponse = {
      root_session_id: 'root',
      nodes: [makeNode({ session_id: 'root', depth: 0 }), makeNode({ session_id: 'child', depth: 1 })],
      edges: [
        makeEdge({ from_session_id: 'root', to_session_id: 'child' }),
        makeEdge({ from_session_id: 'root', to_session_id: 'child' }),
      ],
      truncated: false,
    }
    const { edges } = await getSessionFlowLayout(flow)
    expect(edges).toHaveLength(1)
  })

  it('assigns computed positions from the elk layout to every node', async () => {
    const flow: SessionFlowResponse = {
      root_session_id: 'root',
      nodes: [makeNode({ session_id: 'root', depth: 0 }), makeNode({ session_id: 'child', depth: 1 })],
      edges: [makeEdge({ from_session_id: 'root', to_session_id: 'child' })],
      truncated: false,
    }
    const { nodes } = await getSessionFlowLayout(flow)
    for (const n of nodes) {
      expect(typeof n.position.x).toBe('number')
      expect(typeof n.position.y).toBe('number')
    }
    // Layered top-down layout: child sits below root.
    const root = nodes.find((n) => n.id === 'root')!
    const child = nodes.find((n) => n.id === 'child')!
    expect(child.position.y).toBeGreaterThan(root.position.y)
  })
})
