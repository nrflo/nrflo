import ELK from 'elkjs/lib/elk.bundled.js'
import type { Node, Edge } from '@xyflow/react'
import type { AgentFlowNodeData } from './types'

export const AGENT_NODE_WIDTH = 320
export const BASE_HEIGHT = 110
export const EXPANDED_HEIGHT = 420  // Base + messages area

const elk = new ELK()

/** Build synthetic edges between adjacent layers so ELK places same-layer nodes in the same row. */
function buildLayerEdges(nodes: Node<AgentFlowNodeData>[], existingEdges: Edge[]) {
  const existingSources = new Set(existingEdges.map(e => e.source))
  const existingTargets = new Set(existingEdges.map(e => e.target))

  // Group node IDs by phaseIndex
  const byPhase: Record<number, string[]> = {}
  for (const node of nodes) {
    const idx = node.data.phaseIndex
    if (!byPhase[idx]) byPhase[idx] = []
    byPhase[idx].push(node.id)
  }

  const phases = Object.keys(byPhase).map(Number).sort((a, b) => a - b)
  const syntheticEdges: { id: string; sources: string[]; targets: string[] }[] = []

  for (let i = 0; i < phases.length - 1; i++) {
    const currentIds = byPhase[phases[i]]
    const nextIds = byPhase[phases[i + 1]]
    for (const src of currentIds) {
      for (const tgt of nextIds) {
        // Only add if no existing edge already connects these
        if (!(existingSources.has(src) && existingTargets.has(tgt))) {
          syntheticEdges.push({
            id: `_synth_${src}_${tgt}`,
            sources: [src],
            targets: [tgt],
          })
        }
      }
    }
  }

  return syntheticEdges
}

export async function getLayoutedElements(
  nodes: Node<AgentFlowNodeData>[],
  edges: Edge[],
  expandedAgentKey: string | null
): Promise<{ nodes: Node<AgentFlowNodeData>[]; edges: Edge[] }> {
  if (nodes.length === 0) return { nodes, edges }

  const elkEdges = edges.map(edge => ({
    id: edge.id,
    sources: [edge.source],
    targets: [edge.target],
  }))

  // Add synthetic edges for layer connectivity so ELK assigns correct layers
  const syntheticEdges = buildLayerEdges(nodes, edges)

  const elkGraph = {
    id: 'root',
    layoutOptions: {
      'elk.algorithm': 'layered',
      'elk.direction': 'DOWN',
      'elk.layered.spacing.nodeNodeBetweenLayers': '120',
      'elk.spacing.nodeNode': '60',
      'elk.layered.nodePlacement.strategy': 'NETWORK_SIMPLEX',
      'elk.alignment': 'CENTER',
      'elk.partitioning.activate': 'true',
    },
    children: nodes.map(node => {
      const isExpanded = expandedAgentKey === node.id
      const height = isExpanded ? EXPANDED_HEIGHT : BASE_HEIGHT
      return {
        id: node.id,
        width: AGENT_NODE_WIDTH,
        height,
        layoutOptions: {
          'elk.partitioning.partition': String(node.data.phaseIndex),
        },
      }
    }),
    edges: [...elkEdges, ...syntheticEdges],
  }

  const layout = await elk.layout(elkGraph)

  const nodeMap = new Map(
    (layout.children ?? []).map(child => [child.id, child])
  )

  for (const node of nodes) {
    const elkNode = nodeMap.get(node.id)
    if (elkNode) {
      node.position = { x: elkNode.x ?? 0, y: elkNode.y ?? 0 }
      const isExpanded = expandedAgentKey === node.id
      node.measured = {
        width: AGENT_NODE_WIDTH,
        height: isExpanded ? EXPANDED_HEIGHT : BASE_HEIGHT,
      }
    }
  }

  return { nodes, edges }
}
