import ELK from 'elkjs/lib/elk.bundled.js'
import type { Node, Edge } from '@xyflow/react'
import type { SessionFlowResponse, SessionFlowNode } from '@/types/session'

export const FLOW_NODE_WIDTH = 260
export const FLOW_NODE_HEIGHT = 88

const elk = new ELK()

export interface SessionFlowNodeData extends Record<string, unknown> {
  node: SessionFlowNode
  isRoot: boolean
}

/** Turns a SessionFlowResponse into layered ReactFlow nodes/edges via elkjs, deduping any child shared by multiple parents. */
export async function getSessionFlowLayout(
  flow: SessionFlowResponse | undefined
): Promise<{ nodes: Node<SessionFlowNodeData>[]; edges: Edge[] }> {
  if (!flow || flow.nodes.length === 0) return { nodes: [], edges: [] }

  const seenNodeIds = new Set<string>()
  const rfNodes: Node<SessionFlowNodeData>[] = []
  for (const n of flow.nodes) {
    if (seenNodeIds.has(n.session_id)) continue
    seenNodeIds.add(n.session_id)
    rfNodes.push({
      id: n.session_id,
      type: 'sessionFlow',
      position: { x: 0, y: 0 },
      data: { node: n, isRoot: n.session_id === flow.root_session_id && n.depth === 0 },
    })
  }

  const seenEdgeIds = new Set<string>()
  const rfEdges: Edge[] = []
  for (const e of flow.edges) {
    if (!seenNodeIds.has(e.from_session_id) || !seenNodeIds.has(e.to_session_id)) continue
    const id = `${e.from_session_id}-${e.to_session_id}-${e.kind}`
    if (seenEdgeIds.has(id)) continue
    seenEdgeIds.add(id)
    rfEdges.push({ id, source: e.from_session_id, target: e.to_session_id, label: e.kind })
  }

  const elkGraph = {
    id: 'root',
    layoutOptions: {
      'elk.algorithm': 'layered',
      'elk.direction': 'DOWN',
      'elk.layered.spacing.nodeNodeBetweenLayers': '80',
      'elk.spacing.nodeNode': '40',
    },
    children: rfNodes.map((n) => ({ id: n.id, width: FLOW_NODE_WIDTH, height: FLOW_NODE_HEIGHT })),
    edges: rfEdges.map((e) => ({ id: e.id, sources: [e.source], targets: [e.target] })),
  }

  const layout = await elk.layout(elkGraph)
  const positions = new Map((layout.children ?? []).map((c) => [c.id, c]))

  for (const node of rfNodes) {
    const elkNode = positions.get(node.id)
    if (elkNode) {
      node.position = { x: elkNode.x ?? 0, y: elkNode.y ?? 0 }
      node.measured = { width: FLOW_NODE_WIDTH, height: FLOW_NODE_HEIGHT }
    }
  }

  return { nodes: rfNodes, edges: rfEdges }
}
